/*
Copyright 2021 Triggermesh Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
	"go.uber.org/zap"

	"github.com/triggermesh/aws-custom-runtime/pkg/converter"
	"github.com/triggermesh/aws-custom-runtime/pkg/logger"
	"github.com/triggermesh/aws-custom-runtime/pkg/metrics"
	"github.com/triggermesh/aws-custom-runtime/pkg/sender"
)

var (
	tasks   chan message
	results map[string]chan message

	mutex sync.RWMutex

	awsEndpoint = "/2018-06-01/runtime"
	environment = map[string]string{
		"LD_LIBRARY_PATH":        "/lib64:/usr/lib64:$LAMBDA_RUNTIME_DIR:$LAMBDA_RUNTIME_DIR/lib:$LAMBDA_TASK_ROOT:$LAMBDA_TASK_ROOT/lib:/opt/lib:$LD_LIBRARY_PATH",
		"AWS_LAMBDA_RUNTIME_API": "127.0.0.1",

		// Some dummy values
		"AWS_LAMBDA_FUNCTION_NAME":        "foo",
		"AWS_LAMBDA_FUNCTION_MEMORY_SIZE": "128",
		"AWS_LAMBDA_FUNCTION_VERSION":     "0.0.1",
		"AWS_LAMBDA_LOG_GROUP_NAME":       "foo-group",
		"AWS_LAMBDA_LOG_STREAM_NAME":      "foo-stream",
	}
)

// Specification is a set of env variables that can be used to configure runtime API
type Specification struct {
	// Number of bootstrap processes
	NumberOfinvokers int `envconfig:"invoker_count" default:"4"`
	// Request body size limit, Mb
	RequestSizeLimit int64 `envconfig:"request_size_limit" default:"5"`
	// Funtions deadline, seconds
	FunctionTTL time.Duration `envconfig:"function_ttl" default:"10s"`
	// Lambda runtime API port for functions
	InternalAPIport string `envconfig:"internal_api_port" default:"80"`
	// Lambda API port to put function requests and get results
	// Note that this uses the same environment variable Knative uses to communicate expected port.
	ExternalAPIport string `envconfig:"port" default:"8080"`

	Sink           string `envconfig:"k_sink"`
	ResponseFormat string `envconfig:"response_format"`
}

type Handler struct {
	sender    *sender.Sender
	converter converter.Converter
	reporter  *metrics.EventProcessingStatsReporter
	logger    *zap.SugaredLogger

	requestSizeLimit int64
	functionTTL      time.Duration
}

type message struct {
	id         string
	deadline   time.Time
	data       []byte
	context    map[string]string
	statusCode int
}

func setupEnv(internalAPIport string) error {
	environment["_HANDLER"], _ = os.LookupEnv("_HANDLER")
	environment["LAMBDA_TASK_ROOT"], _ = os.LookupEnv("LAMBDA_TASK_ROOT")
	environment["AWS_LAMBDA_RUNTIME_API"] += ":" + internalAPIport

	for k, v := range environment {
		if err := os.Setenv(k, os.ExpandEnv(v)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	eventTypeTag, eventSrcTag := metrics.DefaultRequestType, metrics.DefaultRequestSource
	start := time.Now()
	defer func() {
		h.reporter.ReportProcessingLatency(time.Since(start), eventTypeTag, eventSrcTag)
	}()

	requestSizeLimitInBytes := h.requestSizeLimit * 1e+6
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, requestSizeLimitInBytes))
	if err != nil {
		h.reporter.ReportProcessingError(false, eventTypeTag, eventSrcTag)
		h.logger.Error("Request exceeds allowed size limit, rejecting")
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	defer r.Body.Close()

	req, context, err := h.converter.Request(body, r.Header)
	if err != nil {
		h.reporter.ReportProcessingError(false, eventTypeTag, eventSrcTag)
		h.logger.Errorf("Cannot convert request: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	eventTypeTag, eventSrcTag = metrics.CETagsFromContext(context)

	h.logger.Debugf("Enqueuing request: %+v, %s", context, string(req))
	result := enqueue(req, context, h.functionTTL)
	h.logger.Debugf("Result: %+v, %s", result.context, string(result.data))

	result.data, err = h.converter.Response(result.data)
	if err != nil {
		result.data = []byte(fmt.Sprintf("Response conversion error: %v", err))
		h.logger.Errorf("Cannot convert response: %v", err)
	}
	if err := h.sender.Send(result.data, result.statusCode, w); err != nil {
		h.reporter.ReportProcessingError(false, eventTypeTag, eventSrcTag)
		h.logger.Errorf("Cannot send response: %v", err)
		return
	}
	h.reporter.ReportProcessingSuccess(eventTypeTag, eventSrcTag)
}

func enqueue(request []byte, context map[string]string, ttl time.Duration) message {
	task := message{
		id:       uuid.New().String(),
		deadline: time.Now().Add(ttl),
		data:     request,
		context:  context,
	}

	resultsChannel := make(chan message)
	mutex.Lock()
	results[task.id] = resultsChannel
	mutex.Unlock()
	defer close(resultsChannel)

	tasks <- task

	var resp message
	select {
	case <-time.After(ttl):
		resp = message{
			id:         task.id,
			data:       []byte(fmt.Sprintf("Deadline is reached, data %s", task.data)),
			statusCode: http.StatusGone,
		}
	case result := <-resultsChannel:
		resp = result
	}
	mutex.Lock()
	delete(results, task.id)
	mutex.Unlock()
	return resp
}

func getTask(w http.ResponseWriter, r *http.Request) {
	task := <-tasks

	// Dummy headers required by Rust client. Replace with something meaningful
	w.Header().Set("Lambda-Runtime-Aws-Request-Id", task.id)
	w.Header().Set("Lambda-Runtime-Deadline-Ms", strconv.Itoa(int(task.deadline.UnixMilli())))
	w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", "arn:aws:lambda:us-east-1:123456789012:function:custom-runtime")
	w.Header().Set("Lambda-Runtime-Trace-Id", "0")
	for k, v := range task.context {
		w.Header().Set(k, v)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(task.data)
}

func (h *Handler) initError(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.logger.Fatalf("Cannot read initialization error data: %v", err)
	}
	defer r.Body.Close()

	h.logger.Fatalf("Runtime initialization error: %s", data)
}

func parsePath(query string) (string, string, error) {
	path := strings.TrimPrefix(query, awsEndpoint+"/invocation/")
	request := strings.Split(path, "/")
	if len(request) != 2 {
		return "", "", fmt.Errorf("incorrect URL path")
	}
	return request[0], request[1], nil
}

func (h *Handler) responseHandler(w http.ResponseWriter, r *http.Request) {
	id, kind, err := parsePath(r.URL.Path)
	if err != nil {
		h.logger.Errorf("Runtime response error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorf("Cannot read response data: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
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
}

func ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func (h *Handler) internalAPI() error {
	internalSocket, _ := os.LookupEnv("AWS_LAMBDA_RUNTIME_API")
	if internalSocket == "" {
		return fmt.Errorf("AWS_LAMBDA_RUNTIME_API is not set")
	}

	apiRouter := http.NewServeMux()
	apiRouter.HandleFunc(awsEndpoint+"/init/error", h.initError)
	apiRouter.HandleFunc(awsEndpoint+"/invocation/next", getTask)
	apiRouter.HandleFunc(awsEndpoint+"/invocation/", h.responseHandler)
	apiRouter.HandleFunc("/2018-06-01/ping", ping)

	err := http.ListenAndServe(internalSocket, apiRouter)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func main() {
	logger := logger.New()

	// parse env
	var spec Specification
	if err := envconfig.Process("", &spec); err != nil {
		logger.Fatalf("Cannot process env variables: %v", err)
	}
	logger.Debugf("Runtime specification: %+v", spec)
	logger.Debug("Setting up runtime env")
	if err := setupEnv(spec.InternalAPIport); err != nil {
		logger.Fatalf("Cannot setup runime env: %v", err)
	}

	// create converter
	conv, err := converter.New(spec.ResponseFormat)
	if err != nil {
		logger.Fatalf("Cannot create converter: %v", err)
	}

	// start metrics reporter
	mr, err := metrics.StatsExporter()
	if err != nil {
		logger.Fatalf("Cannot start stats exporter: %v", err)
	}

	// setup sender
	handler := Handler{
		sender:           sender.New(spec.Sink, conv.ContentType()),
		converter:        conv,
		reporter:         mr,
		logger:           logger,
		requestSizeLimit: spec.RequestSizeLimit,
		functionTTL:      spec.FunctionTTL,
	}

	// setup channels
	tasks = make(chan message, 100)
	results = make(map[string]chan message)
	defer close(tasks)

	// start Lambda API
	logger.Debug("Starting API")
	go func() {
		if err := handler.internalAPI(); err != nil {
			logger.Fatalf("Runtime internal API error: %v", err)
		}
	}()

	// start invokers
	for i := 0; i < spec.NumberOfinvokers; i++ {
		logger.Debug("Starting bootstrap", i+1)
		go func(i int) {
			cmd := exec.Command("sh", "-c", environment["LAMBDA_TASK_ROOT"]+"/bootstrap")
			cmd.Env = append(os.Environ(), fmt.Sprintf("BOOTSTRAP_INDEX=%d", i))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				logger.Fatalf("Cannot start bootstrap process: %v", err)
			}
		}(i)
	}

	// start external API
	taskRouter := http.NewServeMux()
	taskRouter.Handle("/", http.HandlerFunc(handler.serve))
	logger.Info("Runtime initialized")
	err = http.ListenAndServe(":"+spec.ExternalAPIport, taskRouter)
	if err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Runtime external API error: %v", err)
	}
}
