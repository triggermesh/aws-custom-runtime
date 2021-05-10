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
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
)

type ceBody struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Time        string      `json:"time"`
	Source      string      `json:"source"`
	Specversion string      `json:"specversion"`
	Data        interface{} `json:"data"`
}

const contentType = "application/cloudevents+json"

// CloudEvent is a data structure required to map KLR responses to cloudevents
type CloudEvent struct {
	EventType string `envconfig:"type" default:"ce.klr.triggermesh.io"`
	Source    string `envconfig:"source" default:"knative-lambda-runtime"`
	Subject   string `envconfig:"subject" default:"klr-response"`
}

func New() (*CloudEvent, error) {
	var ce CloudEvent
	if err := envconfig.Process("ce", &ce); err != nil {
		return nil, fmt.Errorf("Cannot process CloudEvent env variables: %v", err)
	}
	return &ce, nil
}

func (ce *CloudEvent) Convert(data []byte) ([]byte, error) {
	// If response format is set to CloudEvents
	// and CE_TYPE is empty,
	// then reply with the empty response
	if ce.EventType == "" {
		return nil, nil
	}

	var body interface{}
	body = string(data)

	// try to decode function's response into JSON
	if json.Valid(data) {
		body = json.RawMessage(data)
	}

	b := ceBody{
		ID:          uuid.NewString(),
		Type:        ce.EventType,
		Time:        time.Now().Format(time.RFC3339),
		Source:      ce.Source,
		Specversion: "1.0",
		Data:        body,
	}
	return json.Marshal(b)
}

func (ce *CloudEvent) ContentType() string {
	return contentType
}
