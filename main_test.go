package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/triggermesh/aws-custom-runtime/pkg/converter"
	"github.com/triggermesh/aws-custom-runtime/pkg/logger"
	"github.com/triggermesh/aws-custom-runtime/pkg/metrics"
	"github.com/triggermesh/aws-custom-runtime/pkg/sender"
)

func TestSetupEnv(t *testing.T) {
	var s Specification
	err := envconfig.Process("", &s)
	if err != nil {
		t.Fatal(err)
	}

	if err := setupEnv(s.InternalAPIport); err != nil {
		t.Errorf("Setup Env error %s\n", err)
	}

	for k, v := range environment {
		if value, _ := os.LookupEnv(k); value != v {
			t.Errorf("Env variable mismath: got %s, expected %s\n", value, v)
		}
	}
}

func TestNewTask(t *testing.T) {
	var s Specification
	err := envconfig.Process("", &s)
	if err != nil {
		t.Fatal(err)
	}

	conv, err := converter.New(s.ResponseFormat)
	if err != nil {
		log.Fatalf("Cannot create converter: %v", err)
	}

	// start metrics reporter
	mr, err := metrics.StatsExporter()
	if err != nil {
		log.Fatalf("Cannot start stats exporter: %v", err)
	}

	handler := Handler{
		sender:           sender.New(s.Sink, conv.ContentType()),
		converter:        conv,
		reporter:         mr,
		logger:           logger.New(),
		requestSizeLimit: s.RequestSizeLimit,
		functionTTL:      s.FunctionTTL,
	}

	payload := []byte(`{"payload": "test"}`)

	tasks = make(chan message, 100)
	results = make(map[string]chan message)
	defer close(tasks)

	recorder := httptest.NewRecorder()
	h := http.HandlerFunc(handler.serve)

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatal(err)
	}

	go h.ServeHTTP(recorder, req)
	task := <-tasks
	results[task.id] <- task
	time.Sleep(time.Millisecond * 100)

	// TODO: fix status codes
	// if status := recorder.Code; status != http.StatusOK {
	// t.Errorf("Got %d status code, expecting %d", recorder.Code, http.StatusOK)
	// }
	if recorder.Body.String() != string(payload) {
		t.Errorf("Got %q body, expecting %q", recorder.Body.String(), payload)
	}
}

func TestGetTask(t *testing.T) {
	payload := message{id: "123", deadline: time.Now(), data: []byte(`{"payload": "test"}`)}

	tasks = make(chan message, 100)
	results = make(map[string]chan message)
	defer close(tasks)

	recorder := httptest.NewRecorder()
	handler := http.HandlerFunc(getTask)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks <- payload
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Got %d status code, expecting %d", recorder.Code, http.StatusOK)
	}

	id := recorder.Header().Get("Lambda-Runtime-Aws-Request-Id")
	if id != payload.id {
		t.Errorf("Got %q id, expecting %q", id, payload.id)
	}
	deadline := recorder.Header().Get("Lambda-Runtime-Deadline-Ms")
	if deadline != payload.deadline.String() {
		t.Errorf("Got %q deadline, expecting %q", deadline, payload.deadline)
	}
	if recorder.Body.String() != string(payload.data) {
		t.Errorf("Got %q body, expecting %q", recorder.Body.String(), payload.data)
	}
}

func TestInitError(t *testing.T) {
	h := Handler{
		logger: logger.New(),
	}

	payload := []byte(`Init error`)

	if os.Getenv("CRASH") == "1" {

		recorder := httptest.NewRecorder()
		handler := http.HandlerFunc(h.initError)
		req, err := http.NewRequest("POST", "/", bytes.NewBuffer(payload))
		if err != nil {
			t.Fatal(err)
		}
		handler.ServeHTTP(recorder, req)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestInitError")
	cmd.Env = append(os.Environ(), "CRASH=1")
	stdout, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	gotBytes, _ := ioutil.ReadAll(stdout)
	expected := fmt.Sprintf("Runtime initialization error: %s\n", string(payload))
	got := string(gotBytes)
	if !strings.HasSuffix(got, expected) {
		t.Errorf("Got %q exit message, expecting %q", got, expected)
	}
}

func TestParsePath(t *testing.T) {
	cases := []struct {
		path   string
		result []string
	}{
		{"foo", []string{"", ""}},
		{"foo/bar", []string{"foo", "bar"}},
		{"foo/bar/bleh", []string{"", ""}},
	}

	for _, v := range cases {
		first, second, _ := parsePath(v.path)
		if first != v.result[0] {
			t.Errorf("Got %q value, expecting %q", first, v.result[0])
		}
		if second != v.result[1] {
			t.Errorf("Got %q value, expecting %q", second, v.result[1])
		}
	}
}

func TestResponseHandler(t *testing.T) {
	h := Handler{
		logger: logger.New(),
	}

	cases := []struct {
		path     string
		data     string
		response string
	}{
		{"foo/response", "{payload}", ""},
		// TODO: figure out expected behavior for "error" endpoint, main.go#213
		// {"foo/error", "{payload}", "! Error: {payload}"},
		{"foo/bar", "", "Unknown endpoint: bar"},
	}

	tasks = make(chan message, 100)
	results = make(map[string]chan message, 5)
	defer close(tasks)

	results["foo"] = make(chan message, 5)
	defer close(results["foo"])

	recorder := httptest.NewRecorder()
	handler := http.HandlerFunc(h.responseHandler)

	for _, v := range cases {
		req, err := http.NewRequest("POST", awsEndpoint+"/invocation/"+v.path, bytes.NewBuffer([]byte(v.data)))
		if err != nil {
			t.Fatal(err)
		}
		handler.ServeHTTP(recorder, req)

		if recorder.Body.String() != v.response {
			t.Errorf("Got %q response, expecting %q", recorder.Body.String(), v.response)
		}
	}
}
