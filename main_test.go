package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSetupEnv(t *testing.T) {
	if err := setupEnv(); err != nil {
		t.Errorf("Setup Env error %s\n", err)
	}

	for k, v := range environment {
		if value, _ := os.LookupEnv(k); value != v {
			t.Errorf("Env variable mismath: got %s, expected %s\n", value, v)
		}
	}
}

func TestNewTask(t *testing.T) {
	payload := []byte(`{"payload": "test"}`)

	tasks = make(chan message, 100)
	results = make(map[string]chan message)
	defer close(tasks)

	recorder := httptest.NewRecorder()
	handler := http.HandlerFunc(newTask)

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatal(err)
	}

	go handler.ServeHTTP(recorder, req)
	task := <-tasks
	results[task.id] <- task
	time.Sleep(time.Millisecond * 100)

	if status := recorder.Code; status != http.StatusOK {
		t.Errorf("Got %d status code, expecting %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != string(payload) {
		t.Errorf("Got \"%s\" body, expecting \"%s\"", recorder.Body.String(), payload)
	}
}

func TestGetTask(t *testing.T) {
	payload := message{id: "123", deadline: 000000000000000000, data: []byte(`{"payload": "test"}`)}

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
	deadline := recorder.Header().Get("Lambda-Runtime-Deadline-Ms")
	if id != payload.id {
		t.Errorf("Got \"%s\" id, expecting \"%s\"", id, payload.id)
	}
	if deadline != strconv.Itoa(int(payload.deadline)) {
		t.Errorf("Got \"%s\" deadline, expecting \"%d\"", deadline, payload.deadline)
	}
	if recorder.Body.String() != string(payload.data) {
		t.Errorf("Got \"%s\" body, expecting \"%s\"", recorder.Body.String(), payload.data)
	}
}

func TestInitError(t *testing.T) {
	payload := []byte(`Init error`)

	if os.Getenv("CRASH") == "1" {

		recorder := httptest.NewRecorder()
		handler := http.HandlerFunc(initError)
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
		t.Errorf("Got \"%s\" exit message, expecting \"%s\"", got, expected)
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
			t.Errorf("Got \"%s\" value, expecting \"%s\"", first, v.result[0])
		}
		if second != v.result[1] {
			t.Errorf("Got \"%s\" value, expecting \"%s\"", second, v.result[1])
		}
	}
}

func TestResponseHandler(t *testing.T) {
	cases := []struct {
		path     string
		data     string
		response string
	}{
		{"foo/response", "{payload}", ""},
		{"foo/error", "{payload}", "! Error: {payload}"},
		{"foo/bar", "", "Unknown endpoint: bar"},
	}

	tasks = make(chan message, 100)
	results = make(map[string]chan message, 5)
	defer close(tasks)

	results["foo"] = make(chan message, 5)
	defer close(results["foo"])

	recorder := httptest.NewRecorder()
	handler := http.HandlerFunc(responseHandler)

	for _, v := range cases {
		req, err := http.NewRequest("POST", awsEndpoint+"/invocation/"+v.path, bytes.NewBuffer([]byte(v.data)))
		if err != nil {
			t.Fatal(err)
		}
		handler.ServeHTTP(recorder, req)

		// fmt.Println(req, recorder.Body.String())
		// if recorder.Body.String() != v.response {
		// t.Errorf("Got \"%s\" response, expecting \"%s\"", recorder.Body.String(), v.response)
		// }
	}
}
