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
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type queue map[string]string

var (
	tasks   = queue{}
	results = queue{}
	lock    = sync.RWMutex{}

	environment = map[string]string{
		"_HANDLER":                        "",
		"LAMBDA_TASK_ROOT":                "",
		"AWS_REGION":                      "",
		"AWS_EXECUTION_ENV":               "",
		"AWS_LAMBDA_FUNCTION_NAME":        "",
		"AWS_LAMBDA_FUNCTION_MEMORY_SIZE": "",
		"AWS_LAMBDA_FUNCTION_VERSION":     "",
		"AWS_LAMBDA_LOG_GROUP_NAME":       "",
		"AWS_LAMBDA_LOG_STREAM_NAME":      "",
		"AWS_ACCESS_KEY_ID":               "",
		"AWS_SECRET_ACCESS_KEY":           "",
		"AWS_SESSION_TOKEN":               "",
		"LANG":                            "",
		"TZ":                              "",
		"LAMBDA_RUNTIME_DIR":              "",
		"PATH":                            "/usr/local/bin:/usr/bin/:/bin:/opt/bin",
		"LD_LIBRARY_PATH":                 "/lib64:/usr/lib64:$LAMBDA_RUNTIME_DIR:$LAMBDA_RUNTIME_DIR/lib:$LAMBDA_TASK_ROOT:$LAMBDA_TASK_ROOT/lib:/opt/lib",
		"AWS_LAMBDA_RUNTIME_API":          "localhost",
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

func (t queue) read(key string) (string, bool) {
	lock.RLock()
	defer lock.RUnlock()
	data, ok := t[key]
	return data, ok
}

func (t queue) write(key, value string) {
	lock.Lock()
	defer lock.Unlock()
	t[key] = value
}

func putTask(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer r.Body.Close()

	id := strconv.Itoa(int(time.Now().UnixNano()))
	fmt.Printf("<- %s %s\n", id, body)
	tasks.write(id, string(body))

	response, ok := results.read(id)
	for !ok {
		response, ok = results.read(id)
	}
	fmt.Printf("-> %s %s\n", id, response)
	w.Write([]byte(response + "\n"))
	w.WriteHeader(200)
	return
}

func getTask(w http.ResponseWriter, r *http.Request) {
	for len(tasks) == 0 {
		time.Sleep(time.Millisecond * 100)
	}

	for id, data := range tasks {
		delete(tasks, id)
		w.Header().Set("Lambda-Runtime-Aws-Request-Id", id)
		w.Write([]byte(data))
		w.WriteHeader(200)
		break
	}
	return
}

func initError(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Init error")
	return
}

func taskResult(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatalln(err)
	}
	defer r.Body.Close()

	vars := mux.Vars(r)
	results.write(vars["AwsRequestId"], string(data))
	return
}

func taskError(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Response error")
	return
}

func api() {
	router := mux.NewRouter()
	router.HandleFunc("/2018-06-01/runtime/init/error", initError).Methods("POST")
	router.HandleFunc("/2018-06-01/runtime/invocation/next", getTask).Methods("GET")
	router.HandleFunc("/2018-06-01/runtime/invocation/{AwsRequestId}/response", taskResult).Methods("POST")
	router.HandleFunc("/2018-06-01/runtime/invocation/{AwsRequestId}/error", taskError).Methods("POST")
	log.Fatal(http.ListenAndServe(":80", router))
}

func main() {
	if err := setupEnv(); err != nil {
		fmt.Println("Setup env")
		log.Fatalln(err)
	}

	fmt.Println("Run API")
	go api()

	go func() {
		fmt.Println("Run bootstrap")
		if err := exec.Command("sh", "-c", environment["LAMBDA_TASK_ROOT"]+"/bootstrap").Run(); err != nil {
			log.Fatalln(err)
		}
	}()

	fmt.Println("Starting listener")
	http.HandleFunc("/", putTask)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
