package sender

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

type Handler struct {
	Target      string
	ContentType string
}

func New(target, contentType string) *Handler {
	return &Handler{
		Target:      target,
		ContentType: contentType,
	}
}

func (h *Handler) Send(data []byte, statusCode int, writer http.ResponseWriter) error {
	ctx := context.Background()

	if h.Target != "" {
		_, err := h.request(ctx, data)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return fmt.Errorf("failed to send the data: %w", err)
		}
		writer.WriteHeader(statusCode)
		return nil
	}

	return h.reply(ctx, data, statusCode, writer)
}

func (h *Handler) request(ctx context.Context, data []byte) (*http.Response, error) {
	return http.Post(h.Target, h.ContentType, bytes.NewBuffer(data))
}

func (h *Handler) reply(ctx context.Context, data []byte, statusCode int, writer http.ResponseWriter) error {
	writer.WriteHeader(statusCode)
	_, err := writer.Write(data)
	return err
}
