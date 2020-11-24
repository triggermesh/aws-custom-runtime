package events

import "net/http"

type Mapper interface {
	Request(r *http.Request)
	Response(w http.ResponseWriter, statusCode int, data []byte) (int, error)
}
