// Copyright 2018 TriggerMesh, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	numberOfinvokers = 8    // Number of bootstrap processes
	requestSizeLimit = 1e+7 // Request bosy size limit, 10Mb
	functionTTL      = 3e+9 // Funtions deadline, 3 seconds
)

type message struct {
	ID              string
	Deadline        int64
	StatusCode      int
	Headers         map[string]interface{}
	IsBase64Encoded bool
	Body            string
}

var (
	tasks   chan message
	results map[string]chan message

	mutex sync.RWMutex

	awsEndpoint = "/2018-06-01/runtime"
	environment = map[string]string{
		"PATH":                   "/usr/local/bin:/usr/bin/:/bin:/opt/bin",
		"LD_LIBRARY_PATH":        "/lib64:/usr/lib64:$LAMBDA_RUNTIME_DIR:$LAMBDA_RUNTIME_DIR/lib:$LAMBDA_TASK_ROOT:$LAMBDA_TASK_ROOT/lib:/opt/lib",
		"AWS_LAMBDA_RUNTIME_API": "127.0.0.1",

		// Some dummy values
		"AWS_LAMBDA_FUNCTION_NAME":        "foo",
		"AWS_LAMBDA_FUNCTION_MEMORY_SIZE": "128",
		"AWS_LAMBDA_FUNCTION_VERSION":     "0.0.1",
		"AWS_LAMBDA_LOG_GROUP_NAME":       "foo-group",
		"AWS_LAMBDA_LOG_STREAM_NAME":      "foo-stream",
	}
)

func setupEnv() error {
	environment["_HANDLER"], _ = os.LookupEnv("_HANDLER")
	environment["LAMBDA_TASK_ROOT"], _ = os.LookupEnv("LAMBDA_TASK_ROOT")

	for k, v := range environment {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

func newTask(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, requestSizeLimit))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	now := time.Now().UnixNano()
	task := message{
		ID:       fmt.Sprintf("%d", now),
		Deadline: now + functionTTL,
		Body:     string(body),
	}
	fmt.Printf("<- %s %s\n", task.ID, task.Body)

	resultsChannel := make(chan message)
	mutex.Lock()
	results[task.ID] = resultsChannel
	mutex.Unlock()
	defer close(resultsChannel)

	tasks <- task

	select {
	case <-time.After(time.Duration(functionTTL)):
		fmt.Printf("-> ! %s Deadline is reached\n", task.ID)
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(fmt.Sprintf("Deadline is reached, data %s", task.Body)))
	case result := <-resultsChannel:
		fmt.Printf("-> %s %d %s\n", result.ID, result.StatusCode, result.Body)
		for k, v := range result.Headers {
			if value, ok := v.(string); ok {
				w.Header().Add(k, value)
			}
		}
		w.WriteHeader(result.StatusCode)
		w.Write([]byte(result.Body))
	}
	mutex.Lock()
	delete(results, task.ID)
	mutex.Unlock()
	return
}

func getTask(w http.ResponseWriter, r *http.Request) {
	task := <-tasks

	// Dummy headers required by Rust client. Replace with something meaningful
	w.Header().Set("Lambda-Runtime-Aws-Request-Id", task.ID)
	w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.Itoa(int(task.Deadline)))
	w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", "arn:aws:lambda:us-east-1:123456789012:function:custom-runtime")
	w.Header().Set("Lambda-Runtime-Trace-Id", "0")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(task.Body))
	return
}

func initError(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatalln(err)
	}
	defer r.Body.Close()

	log.Fatalf("Runtime initialization error: %s\n", data)
	return
}

func parsePath(query string) (string, string, error) {
	path := strings.TrimPrefix(query, awsEndpoint+"/invocation/")
	request := strings.Split(path, "/")
	if len(request) != 2 {
		return "", "", fmt.Errorf("Incorrect URL query size")
	}
	return request[0], request[1], nil
}

func responseHandler(w http.ResponseWriter, r *http.Request) {
	id, kind, err := parsePath(r.URL.Path)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("! %s\n", err)
		return
	}
	defer r.Body.Close()

	mutex.RLock()
	resultsChannel, ok := results[id]
	mutex.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusGone)
		w.Write([]byte("Function deadline is reached"))
		return
	}

	result := message{
		Body:       string(body),
		StatusCode: 200,
	}
	switch kind {
	case "response":
		if js, ok := parseJSON(body); ok {
			result = js
		}
	case "error":
		result.StatusCode = 500
		fmt.Printf("! Error: %s\n", body)
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("Unknown endpoint: %s", kind)))
		return
	}
	result.ID = id
	resultsChannel <- result
	w.WriteHeader(http.StatusAccepted)
	return
}

func parseJSON(s []byte) (message, bool) {
	var js message
	if err := json.Unmarshal(s, &js); err != nil {
		fmt.Println(s, err)
		return js, false
	}
	if js.StatusCode == 0 {
		return js, false
	}
	return js, true
}

func api() error {
	apiRouter := http.NewServeMux()
	apiRouter.HandleFunc(awsEndpoint+"/init/error", initError)
	apiRouter.HandleFunc(awsEndpoint+"/invocation/next", getTask)
	apiRouter.HandleFunc(awsEndpoint+"/invocation/", responseHandler)
	return http.ListenAndServe(":80", apiRouter)
}

func main() {
	tasks = make(chan message, 100)
	results = make(map[string]chan message)
	defer close(tasks)

	fmt.Println("Setup env")
	if err := setupEnv(); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Starting API")
	go func() {
		log.Fatalln(api())
	}()

	fmt.Println("Starting invokers")
	for i := 0; i < numberOfinvokers; i++ {
		go func() {
			if err := exec.Command("sh", "-c", environment["LAMBDA_TASK_ROOT"]+"/bootstrap").Run(); err != nil {
				log.Fatalln(err)
			}
		}()
	}

	taskRouter := http.NewServeMux()
	taskRouter.HandleFunc("/", newTask)
	fmt.Println("Listening...")
	log.Fatalln(http.ListenAndServe(":8080", taskRouter))
}
