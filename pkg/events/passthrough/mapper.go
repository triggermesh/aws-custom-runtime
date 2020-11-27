package passthrough

import (
	"net/http"
)

type Passthrough struct{}

func NewMapper() *Passthrough {
	return &Passthrough{}
}

func (c *Passthrough) Request(r *http.Request) {}

func (c *Passthrough) Response(w http.ResponseWriter, statusCode int, data []byte) (int, error) {
	w.WriteHeader(statusCode)
	return w.Write(data)
}
