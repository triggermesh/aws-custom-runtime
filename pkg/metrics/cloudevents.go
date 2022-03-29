/*
Copyright 2022 Triggermesh Inc.

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

package metrics

import (
	"encoding/json"

	"github.com/triggermesh/aws-custom-runtime/pkg/converter/cloudevents"
	"go.opencensus.io/tag"
)

const (
	typeAttr   = "type"
	sourceAttr = "source"
)

// Default metric tags for raw requests.
var (
	DefaultRequestType   = tag.Insert(tagKeyEventType, "plain-http")
	DefaultRequestSource = tag.Insert(tagKeyEventSource, "unknown")
)

// CETagsFromContext parses Lambda context and returns CloudEvents-specific
// type and source tags.
func CETagsFromContext(context map[string]string) (tag.Mutator, tag.Mutator) {
	if context == nil {
		return DefaultRequestType, DefaultRequestSource
	}
	ceContext, exists := context[cloudevents.ContextKey]
	if !exists {
		return DefaultRequestType, DefaultRequestSource
	}
	var attributes map[string]string
	if err := json.Unmarshal([]byte(ceContext), &attributes); err != nil {
		return DefaultRequestType, DefaultRequestSource
	}
	return tag.Insert(tagKeyEventType, attributes[typeAttr]),
		tag.Insert(tagKeyEventSource, attributes[sourceAttr])
}
