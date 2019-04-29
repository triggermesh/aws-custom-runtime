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

	"github.com/triggermesh/aws-custom-runtime/pkg/events/apiGateway"
)

type message struct {
	id         string
	deadline   int64
	data       []byte
	statusCode int
}

type responseWrapper struct {
	http.ResponseWriter
	StatusCode int
	Body []byte
}

var (
	numberOfinvokers       = 8  // Number of bootstrap processes
	requestSizeLimit int64 = 5  // Request body size limit, Mb
	functionTTL      int64 = 10 // Funtions deadline, seconds

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

func (rw *responseWrapper) Write(data []byte) (int, error) {
	rw.Body = data
	return len(data), nil
}

func (rw *responseWrapper) WriteHeader(statusCode int) {
	rw.StatusCode = statusCode;
}

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
	requestSizeLimitInBytes := requestSizeLimit * 1e+6
	functionTTLInNanoSeconds := functionTTL * 1e+9
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, requestSizeLimitInBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	now := time.Now().UnixNano()
	task := message{
		id:       fmt.Sprintf("%d", now),
		deadline: now + functionTTLInNanoSeconds,
		data:     body,
	}
	fmt.Printf("<- %s %s\n", task.id, task.data)

	resultsChannel := make(chan message)
	mutex.Lock()
	results[task.id] = resultsChannel
	mutex.Unlock()
	defer close(resultsChannel)

	tasks <- task

	select {
	case <-time.After(time.Duration(functionTTLInNanoSeconds)):
		fmt.Printf("-> ! %s Deadline is reached\n", task.id)
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(fmt.Sprintf("Deadline is reached, data %s", task.data)))
	case result := <-resultsChannel:
		fmt.Printf("-> %s %d %s\n", result.id, result.statusCode, result.data)
		w.WriteHeader(result.statusCode)
		w.Write(result.data)
	}
	mutex.Lock()
	delete(results, task.id)
	mutex.Unlock()
	return
}

func getTask(w http.ResponseWriter, r *http.Request) {
	task := <-tasks

	// Dummy headers required by Rust client. Replace with something meaningful
	w.Header().Set("Lambda-Runtime-Aws-Request-Id", task.id)
	w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.Itoa(int(task.deadline)))
	w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", "arn:aws:lambda:us-east-1:123456789012:function:custom-runtime")
	w.Header().Set("Lambda-Runtime-Trace-Id", "0")

	w.WriteHeader(http.StatusOK)
	w.Write(task.data)
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

	data, err := ioutil.ReadAll(r.Body)
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

	statusCode := 200

	switch kind {
	case "response":
	case "error":
		statusCode = 500
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("Unknown endpoint: %s", kind)))
		return
	}
	resultsChannel <- message{
		id:         id,
		data:       data,
		statusCode: statusCode,
	}
	w.WriteHeader(http.StatusAccepted)
	return
}

func mapEvent(h http.Handler) http.Handler {
	eventType, _ := os.LookupEnv("EVENT")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := responseWrapper{w, 200, []byte{}}
		switch eventType {
		case "API_GATEWAY":
			apiGateway.Request(r)
			h.ServeHTTP(&rw, r)
			apiGateway.Response(w, rw.StatusCode, rw.Body)
		default:
			h.ServeHTTP(w, r)
		}
	})
}

func setLimits() {
	if v, ok := os.LookupEnv("INVOKER_COUNT"); ok {
		if vv, err := strconv.Atoi(v); err != nil {
			fmt.Printf("can't set invokers limit, using default value %d\n", numberOfinvokers)
		} else {
			numberOfinvokers = vv
		}
	}
	if v, ok := os.LookupEnv("REQUEST_SIZE_LIMIT"); ok {
		if vv, err := strconv.Atoi(v); err != nil {
			fmt.Printf("can't set request size limit, using default value %d\n", requestSizeLimit)
		} else {
			requestSizeLimit = int64(vv)
		}
	}
	if v, ok := os.LookupEnv("FUNCTION_TTL"); ok {
		if vv, err := strconv.Atoi(v); err != nil {
			fmt.Printf("can't set function ttl, using default value %d\n", functionTTL)
		} else {
			functionTTL = int64(vv)
		}
	}
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

	fmt.Println("Setting limits")
	setLimits()

	fmt.Println("Setup env")
	if err := setupEnv(); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Starting API")
	go func() {
		log.Fatalln(api())
	}()

	for i := 0; i < numberOfinvokers; i++ {
		fmt.Println("Starting bootstrap", i+1)
		go func(i int) {
			cmd := exec.Command("sh", "-c", environment["LAMBDA_TASK_ROOT"]+"/bootstrap")
			cmd.Env = append(os.Environ(), fmt.Sprintf("BOOTSTRAP_INDEX=%d", i))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Fatalln(err)
			}
		}(i)
	}

	taskRouter := http.NewServeMux()
	taskHandler := http.HandlerFunc(newTask)
	taskRouter.Handle("/", mapEvent(taskHandler))
	fmt.Println("Listening...")
	log.Fatalln(http.ListenAndServe(":8080", taskRouter))
}
