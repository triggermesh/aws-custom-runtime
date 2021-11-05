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

package cloudevents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
)

const contentType = "application/cloudevents+json"

type ceBinaryStructure struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Source      string      `json:"source"`
	Specversion string      `json:"specversion"`
	Time        string      `json:"time,omitempty"`
	Contenttype string      `json:"datacontenttype,omitempty"`
	Data        interface{} `json:"data,omitempty"`
}

// CloudEvent is a data structure required to map KLR responses to cloudevents
type CloudEvent struct {
	// FunctionResponseMode describes what data is returned from the function:
	// only data payload or full event in binary format
	FunctionResponseMode string `envconfig:"function_response_mode" default:"data"`

	Overrides Overrides `envconfig:"overrides"`
}

type Overrides struct {
	EventType string `envconfig:"type" default:"ce.klr.triggermesh.io"`
	Source    string `envconfig:"source" default:"knative-lambda-runtime"`
	Subject   string `envconfig:"subject" default:"klr-response"`
}

func New() (*CloudEvent, error) {
	var ce CloudEvent
	if err := envconfig.Process("ce", &ce); err != nil {
		return nil, fmt.Errorf("cannot process CloudEvent env variables: %v", err)
	}
	return &ce, nil
}

func (ce *CloudEvent) Response(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	if ce.FunctionResponseMode == "event" {
		return ce.fillInContext(data)
	}

	// If response format is set to CloudEvents
	// and CE_TYPE is empty,
	// then reply with the empty response
	if ce.Overrides.EventType == "" {
		return nil, nil
	}

	var body interface{}
	contentType := "text/plain"

	switch {
	case json.Valid(data) &&
		(bytes.TrimSpace(data)[0] == '{' ||
			bytes.TrimSpace(data)[0] == '['):
		contentType = "application/json"
		body = json.RawMessage(data)
	default:
		data = bytes.TrimSpace(data)
		data = bytes.Trim(data, "\"")
		body = string(data)
	}

	b := ceBinaryStructure{
		ID:          uuid.NewString(),
		Type:        ce.Overrides.EventType,
		Time:        time.Now().Format(time.RFC3339),
		Source:      ce.Overrides.Source,
		Specversion: "1.0",
		Contenttype: contentType,
		Data:        body,
	}
	return json.Marshal(b)
}

func (ce *CloudEvent) fillInContext(data []byte) ([]byte, error) {
	var response ceBinaryStructure
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("cannot unmarshal function response into binary CE: %w", err)
	}

	if response.ID == "" {
		response.ID = uuid.NewString()
	}
	if response.Type == "" {
		response.Type = ce.Overrides.EventType
	}
	if response.Source == "" {
		response.Source = ce.Overrides.Source
	}
	if response.Specversion == "" {
		response.Specversion = "1.0"
	}

	return json.Marshal(response)
}

func (ce *CloudEvent) Request(request []byte, headers http.Header) ([]byte, map[string]string, error) {
	var context map[string]string
	var body []byte
	var err error

	switch headers.Get("Content-Type") {
	case "application/cloudevents+json":
		if body, context, err = parseStructuredCE(request); err != nil {
			return nil, nil, fmt.Errorf("structured CloudEvent parse error: %w", err)
		}
	case "application/json":
		body = request
		context = parseBinaryCE(headers)
	default:
		return request, nil, nil
	}

	ceContext, err := json.Marshal(context)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot encode request context: %w", err)
	}

	runtimeContext := map[string]string{
		"Lambda-Runtime-Cloudevents-Context": string(ceContext),
	}

	return body, runtimeContext, nil
}

func parseStructuredCE(body []byte) ([]byte, map[string]string, error) {
	var event map[string]interface{}
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, nil, fmt.Errorf("cannot unmarshal body: %w", err)
	}

	data, err := json.Marshal(event["data"])
	if err != nil {
		return nil, nil, fmt.Errorf("cannot marshal body: %w", err)
	}

	delete(event, "data")
	headers := make(map[string]string, len(event))
	for k, v := range event {
		headers[k] = fmt.Sprintf("%v", v)
	}

	return data, headers, nil
}

func parseBinaryCE(headers http.Header) map[string]string {
	h := make(map[string]string)
	for k, v := range headers {
		if strings.HasPrefix(k, "Ce-") {
			h[strings.ToLower(k[3:])] = strings.Join(v, ",")
		}
	}
	return h
}

func (ce *CloudEvent) ContentType() string {
	return contentType
}
