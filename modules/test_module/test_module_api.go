package test_module

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

// waitResponse is the response payload for the wait endpoint.
type waitResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

// WaitHandler sleeps for the specified number of seconds.
// It returns early if the request context is canceled.
func waitHandler(ctx context.Context, input *struct {
	Seconds int `query:"seconds"`
}) (*waitResponse, error) {
	secs := input.Seconds
	if secs <= 0 {
		return nil, huma.Error400BadRequest("Invalid seconds value; must be > 0")
	}

	waitDuration := time.Duration(secs) * time.Second

	// Check if the context has already been canceled before starting the wait
	if err := ctx.Err(); err != nil {
		logging.Debug("Context already canceled before wait started")
		return nil, huma.NewError(http.StatusRequestTimeout, fmt.Sprintf("Request context error: %s", err))
	}

	select {
	case <-time.After(waitDuration):
		logging.Debug("Waited for %d seconds", secs)
		return &waitResponse{Body: struct {
			Message string `json:"message"`
		}{Message: fmt.Sprintf("Waited for %d seconds", secs)}}, nil
	case <-ctx.Done():
		logging.Debug("Request canceled")
		// Return the context cancellation error.
		return nil, huma.NewError(http.StatusRequestTimeout, "Request canceled")
	}
}

func waitStreamHandler(ctx context.Context, input *struct {
	Body    map[string]interface{} `query:"body"`
	Seconds int                    `query:"seconds"`
}) (*huma.StreamResponse, error) {
	secs := input.Seconds
	if secs <= 0 {
		return nil, huma.Error400BadRequest("Invalid seconds value; must be > 0")
	}

	b := input.Body
	logging.Debug("input body: %+v", b)

	waitDuration := time.Duration(secs) * time.Second

	select {
	case <-time.After(waitDuration):
		logging.Debug("Waited for %d seconds (stream)", secs)
		return &huma.StreamResponse{Body: func(ctx huma.Context) {
			ctx.SetHeader("Content-Type", "application/json")
			fmt.Fprintf(ctx.BodyWriter(), `{"message": "Waited for %d seconds (stream)"}`, secs)
		}}, nil
	case <-ctx.Done():
		logging.Debug("Request canceled (stream)")
		return nil, huma.NewError(http.StatusRequestTimeout, "Request canceled (stream)")
	}
}

// RegisterWaitRoutes registers the wait endpoint with the provided Huma API.
func registerTestModuleAPIRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:     "wait",
		Method:          http.MethodGet,
		BodyReadTimeout: config.GetServerTimeout(),
		Path:            "/test/wait",
		Summary:         "Wait for a specified number of seconds",
		Tags:            []string{"Test Module"},
	}, waitHandler)

	huma.Register(api, huma.Operation{
		OperationID:     "wait",
		Method:          http.MethodPost,
		BodyReadTimeout: config.GetServerTimeout(),
		Path:            "/test/wait",
		Summary:         "Wait for a specified number of seconds",
		Tags:            []string{"Test Module"},
	}, waitHandler)

	huma.Register(api, huma.Operation{
		OperationID:     "wait-stream",
		BodyReadTimeout: config.GetServerTimeout(),
		Method:          http.MethodPost,
		Path:            "/test/wait-stream",
		Summary:         "Wait for a specified number of seconds and stream the response",
		Tags:            []string{"Test Module"},
	}, waitStreamHandler)
}
