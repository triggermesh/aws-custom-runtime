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
	"time"

	"github.com/gorilla/mux"
)

const (
	requestSizeLimit = 67108864
	functionTTL      = 5e+9 // Funtions deadline, 5 seconds
)

type message struct {
	id       string
	deadline int64
	data     []byte
}

var (
	tasks   chan message
	results map[string]chan message

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
		id:       fmt.Sprintf("%d", now),
		deadline: now + functionTTL,
		data:     body,
	}
	fmt.Printf("<- %s %s\n", task.id, task.data)

	results[task.id] = make(chan message)
	defer close(results[task.id])

	tasks <- task

	select {
	case <-time.After(time.Duration(functionTTL)):
		fmt.Printf("-> ! %s Deadline is reached\n", task.id)
		w.WriteHeader(http.StatusRequestTimeout)
		w.Write([]byte("Function deadline is reached"))
	case result := <-results[task.id]:
		fmt.Printf("Response in queue %s\n", result.id)
		fmt.Printf("-> %s %s\n", result.id, result.data)
		w.WriteHeader(http.StatusOK)
		w.Write(result.data)
	}

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

func postResult(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["AwsRequestId"]
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("! %s\n", err)
		return
	}
	defer r.Body.Close()

	results[id] <- message{
		id:   id,
		data: data,
	}
	w.WriteHeader(http.StatusAccepted)
	return
}

func taskError(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["AwsRequestId"]
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("! %s\n", err)
		return
	}

	results[id] <- message{
		id:   id,
		data: data,
	}
	w.WriteHeader(http.StatusAccepted)
	return
}

func api() {
	router := mux.NewRouter()
	router.HandleFunc(awsEndpoint+"/init/error", initError).Methods("POST")
	router.HandleFunc(awsEndpoint+"/invocation/next", getTask).Methods("GET")
	router.HandleFunc(awsEndpoint+"/invocation/{AwsRequestId}/response", postResult).Methods("POST")
	router.HandleFunc(awsEndpoint+"/invocation/{AwsRequestId}/error", taskError).Methods("POST")
	log.Fatal(http.ListenAndServe(":80", router))
}

type maxBytesHandler struct {
	h http.Handler
	n int64
}

func (h *maxBytesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.n)
	h.h.ServeHTTP(w, r)
}

func main() {
	tasks = make(chan message, 100)
	results = make(map[string]chan message)
	defer close(tasks)

	fmt.Println("Setup env")
	if err := setupEnv(); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Run API")
	go api()

	fmt.Println("Run bootstrap")
	go func() {
		if err := exec.Command("sh", "-c", environment["LAMBDA_TASK_ROOT"]+"/bootstrap").Run(); err != nil {
			log.Fatalln(err)
		}
	}()

	fmt.Println("Starting listener")
	http.HandleFunc("/", newTask)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
