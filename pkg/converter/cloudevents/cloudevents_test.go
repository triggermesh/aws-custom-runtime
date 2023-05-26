package cloudevents

import (
	"net/http"
	"reflect"
	"testing"
)

func TestCloudEvent_Request(t *testing.T) {
	tests := []struct {
		name                   string
		request                string
		headers                http.Header
		expectedBody           string
		expectedRuntimeContext map[string]string
		wantErr                bool
	}{
		{
			name:    "Event of type application/cloudevents+json",
			request: `{"source":"test","data":{"foo":"bar"}}`,
			headers: http.Header{
				"Content-Type": {"application/cloudevents+json"},
			},
			expectedBody: `{"foo":"bar"}`,
			expectedRuntimeContext: map[string]string{
				CeContextKey:     `{"source":"test"}`,
				ClientContextKey: `{"custom":{"source":"test"}}`,
			},
		},
		{
			name:    "Event of type application/json",
			request: `{"foo":"bar"}`,
			headers: http.Header{
				"Content-Type": {"application/json"},
				"ce-source":    {"test"},
			},
			expectedBody: `{"foo":"bar"}`,
			expectedRuntimeContext: map[string]string{
				CeContextKey:     `{"source":"test"}`,
				ClientContextKey: `{"custom":{"source":"test"}}`,
			},
		},
		{
			name:    "Event of other type",
			request: `hello world`,
			headers: http.Header{
				"Content-Type": {"text/plain"},
			},
			expectedBody:           `hello world`,
			expectedRuntimeContext: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := &CloudEvent{}
			body, runtimeContext, err := ce.Request([]byte(tt.request), tt.headers)

			if (err != nil) != tt.wantErr {
				t.Errorf("Request() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(string(body), tt.expectedBody) {
				t.Errorf("Request() got = %v, want %v", string(body), tt.expectedBody)
			}

			if !reflect.DeepEqual(runtimeContext, tt.expectedRuntimeContext) {
				t.Errorf("Request() got1 = %v, want %v", runtimeContext, tt.expectedRuntimeContext)
			}
		})
	}
}
