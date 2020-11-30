package cloudevents

import (
	"fmt"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
)

// CloudEvent is a data structure required to map KLR responses to cloudevents
type CloudEvent struct {
	EventType string `envconfig:"type" default:"ce.klr.triggermesh.io"`
	Source    string `envconfig:"k_service" default:"knative-lambda-runtime"`
	Subject   string `envconfig:"subject" default:"klr-response"`
}

// NewMapper reurns an empty instance of CloudEvent structure
func NewMapper() *CloudEvent {
	return &CloudEvent{}
}

// Request method can be used to customize incoming requests before passing them
// to the KLR function
func (c *CloudEvent) Request(r *http.Request) {}

// Response method converts generic KLR response to the Cloudevent format
func (c *CloudEvent) Response(w http.ResponseWriter, statusCode int, data []byte) (int, error) {
	response := cloudevents.NewEvent()
	response.SetType(c.EventType)
	response.SetSource(c.Source)
	response.SetSubject(c.Subject)
	response.SetID(uuid.New().String())
	if err := response.SetData(cloudevents.ApplicationJSON, data); err != nil {
		return 0, fmt.Errorf("failed to set event data: %w", err)
	}
	return w.Write([]byte(response.String()))
}
