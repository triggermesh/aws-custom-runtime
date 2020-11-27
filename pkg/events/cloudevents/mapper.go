package cloudevents

import (
	"fmt"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
)

type CloudEvents struct {
	Service string
}

func NewMapper(service string) *CloudEvents {
	return &CloudEvents{
		Service: service,
	}
}

func (c *CloudEvents) Request(r *http.Request) {}

func (c *CloudEvents) Response(w http.ResponseWriter, statusCode int, data []byte) (int, error) {
	response := cloudevents.NewEvent()
	response.SetType("ce.klr.triggermesh.io")
	response.SetSource(c.Service)
	response.SetID(uuid.New().String())
	if err := response.SetData(cloudevents.ApplicationJSON, data); err != nil {
		return 0, fmt.Errorf("failed to set event data: %w", err)
	}
	return w.Write([]byte(response.String()))
}
