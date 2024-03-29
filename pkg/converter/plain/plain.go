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

package plain

import "net/http"

type Plain struct{}

const contentType = "plain/text"

func New() (*Plain, error) {
	return &Plain{}, nil
}

func (p *Plain) Response(data []byte) ([]byte, error) {
	return data, nil
}

func (p *Plain) Request(request []byte, headers http.Header) ([]byte, map[string]string, error) {
	return request, nil, nil
}

func (p *Plain) ContentType() string {
	return contentType
}
