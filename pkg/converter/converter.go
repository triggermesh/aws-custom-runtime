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

package converter

import (
	"net/http"

	"github.com/triggermesh/aws-custom-runtime/pkg/converter/cloudevents"
	"github.com/triggermesh/aws-custom-runtime/pkg/converter/plain"
)

type Converter interface {
	Response([]byte) ([]byte, error)
	Request([]byte, http.Header) ([]byte, map[string]string, error)
	ContentType() string
}

func New(format string) (Converter, error) {
	switch format {
	case "API_GATEWAY":
	case "CLOUDEVENTS":
		return cloudevents.New()
	}
	return plain.New()
}
