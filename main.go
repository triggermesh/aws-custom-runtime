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
	"context"
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

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/kelseyhightower/envconfig"
)

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

	ce ceClient
)

// Specification is a set of env variables that can be used to configure runtime API
type Specification struct {
	// Number of bootstrap processes
	NumberOfinvokers int `envconfig:"invoker_count" default:"4"`
	// Request body size limit, Mb
	RequestSizeLimit int64 `envconfig:"request_size_limit" default:"5"`
	// Funtions deadline, seconds
	FunctionTTL int64 `envconfig:"function_ttl" default:"10"`
	// Lambda runtime API port for functions
	InternalAPIport string `envconfig:"internal_api_port" default:"80"`
	// Lambda API port to put function requests and get results
	ExternalAPIport string `envconfig:"external_api_port" default:"8080"`
	// Optional sink reference
	Sink string `envconfig:"k_sink"`
}

type ceClient struct {
	env    cloudEventSpec
	client cloudevents.Client
}

// cloudEventSpec is a data structure required to map KLR responses to cloudevents
type cloudEventSpec struct {
	EventType string `envconfig:"type" default:"ce.klr.triggermesh.io"`
	Source    string `envconfig:"source" default:"knative-lambda-runtime"`
	Subject   string `envconfig:"subject" default:"klr-response"`
}

type message struct {
	id         string
	deadline   int64
	data       []byte
	statusCode int
}

func (s *Specification) setupEnv() error {
	environment["_HANDLER"], _ = os.LookupEnv("_HANDLER")
	environment["LAMBDA_TASK_ROOT"], _ = os.LookupEnv("LAMBDA_TASK_ROOT")
	environment["AWS_LAMBDA_RUNTIME_API"] += ":" + s.InternalAPIport

	for k, v := range environment {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Specification) newTask(w http.ResponseWriter, r *http.Request) {
	requestSizeLimitInBytes := s.RequestSizeLimit * 1e+6
	functionTTLInNanoSeconds := s.FunctionTTL * 1e+9
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
	log.Printf("<- %s %s\n", task.id, task.data)

	resultsChannel := make(chan message)
	mutex.Lock()
	results[task.id] = resultsChannel
	mutex.Unlock()
	defer close(resultsChannel)

	tasks <- task

	select {
	case <-time.After(time.Duration(functionTTLInNanoSeconds)):
		log.Printf("-> ! %s Deadline is reached\n", task.id)
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(fmt.Sprintf("Deadline is reached, data %s", task.data)))
	case result := <-resultsChannel:
		log.Printf("-> %s %d %s\n", result.id, result.statusCode, result.data)
		if s.Sink != "" {
			if err := sendCE(s.Sink, result.data); err != nil {
				log.Printf("-> ! CE %s is not sent: %d\n", result.id, result.statusCode)
				result.statusCode = http.StatusBadGateway
				result.data = []byte(err.Error())
			} else {
				result.statusCode = http.StatusAccepted
				result.data = []byte("ok")
			}
		}
		w.WriteHeader(result.statusCode)
		w.Write(result.data)
	}
	mutex.Lock()
	delete(results, task.id)
	mutex.Unlock()
	return
}

func sendCE(sink string, data interface{}) error {
	e := cloudevents.NewEvent()
	e.SetType(ce.env.EventType)
	e.SetSource(ce.env.Source)
	e.SetSubject(ce.env.Subject)
	if err := e.SetData(cloudevents.ApplicationJSON, data); err != nil {
		return fmt.Errorf("cannot set CE data: %w", err)
	}
	ctx := cloudevents.ContextWithTarget(context.Background(), sink)
	res := ce.client.Send(ctx, e)
	if cloudevents.IsUndelivered(res) {
		return fmt.Errorf("failed to send CE: %w", res)
	}
	return nil
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
		log.Printf("! %s\n", err)
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

func ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
	return
}

func api() error {
	internalSocket, _ := os.LookupEnv("AWS_LAMBDA_RUNTIME_API")
	if internalSocket == "" {
		return fmt.Errorf("AWS_LAMBDA_RUNTIME_API is not set")
	}

	apiRouter := http.NewServeMux()
	apiRouter.HandleFunc(awsEndpoint+"/init/error", initError)
	apiRouter.HandleFunc(awsEndpoint+"/invocation/next", getTask)
	apiRouter.HandleFunc(awsEndpoint+"/invocation/", responseHandler)
	apiRouter.HandleFunc("/2018-06-01/ping", ping)

	err := http.ListenAndServe(internalSocket, apiRouter)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func main() {
	var spec Specification
	if err := envconfig.Process("", &spec); err != nil {
		log.Fatalf("Cannot process env variables: %v", err)
	}

	if spec.Sink != "" {
		p, err := cloudevents.NewHTTP()
		if err != nil {
			log.Fatalf("failed to create protocol: %s", err.Error())
		}

		ce.client, err = cloudevents.NewClient(p, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
		if err != nil {
			log.Fatalf("failed to create CE client, %v", err)
		}

		if err := envconfig.Process("CE", &ce.env); err != nil {
			log.Fatalf("Cannot process env variables: %v", err)
		}

		log.Printf("%+v\n", ce.env)
	}

	log.Printf("%+v\n", spec)

	log.Println("Setting up runtime env")
	if err := spec.setupEnv(); err != nil {
		log.Fatalf("Cannot setup runime env: %v", err)
	}

	tasks = make(chan message, 100)
	results = make(map[string]chan message)
	defer close(tasks)

	log.Println("Starting API")
	go func() {
		if err := api(); err != nil {
			log.Fatalf("Runtime internal API error: %v", err)
		}
	}()

	for i := 0; i < spec.NumberOfinvokers; i++ {
		log.Println("Starting bootstrap", i+1)
		go func(i int) {
			cmd := exec.Command("sh", "-c", environment["LAMBDA_TASK_ROOT"]+"/bootstrap")
			cmd.Env = append(os.Environ(), fmt.Sprintf("BOOTSTRAP_INDEX=%d", i))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Fatalf("Cannot start bootstrap process: %v", err)
			}
		}(i)
	}

	taskRouter := http.NewServeMux()
	taskHandler := http.HandlerFunc(spec.newTask)
	taskRouter.Handle("/", taskHandler)
	log.Println("Listening...")
	err := http.ListenAndServe(":"+spec.ExternalAPIport, taskRouter)
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("Runtime external API error: %v", err)
	}
}
