package apigateway

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

type APIGateway struct{}

func NewMapper() *APIGateway {
	return &APIGateway{}
}

func (a *APIGateway) Request(r *http.Request) {
	event := events.APIGatewayProxyRequest{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Fatalln(err)
	}

	event.Body = string(body)
	event.Headers = make(map[string]string)
	for k, v := range r.Header {
		event.Headers[k] = strings.Join(v, ",")
	}
	event.HTTPMethod = r.Method
	event.Path = r.URL.Path
	event.QueryStringParameters = make(map[string]string)
	for k, v := range r.URL.Query() {
		event.QueryStringParameters[k] = strings.Join(v, ",")
	}
	event.RequestContext = events.APIGatewayProxyRequestContext{}
	// event.Resource = ""
	// event.QueryStringParameters = make(map[string]string)
	// event.IsBase64Encoded = false
	js, err := json.Marshal(event)
	if err != nil {
		log.Fatalln(err)
	}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(js))
}

func (a *APIGateway) Response(w http.ResponseWriter, statusCode int, data []byte) (int, error) {
	var js events.APIGatewayProxyResponse
	if err := json.Unmarshal(data, &js); err != nil {
		return 0, err
	}
	for k, v := range js.Headers {
		w.Header().Set(k, v)
	}

	if js.StatusCode >= 200 {
		statusCode = js.StatusCode
	}
	w.WriteHeader(statusCode)

	return w.Write([]byte(js.Body))
}
