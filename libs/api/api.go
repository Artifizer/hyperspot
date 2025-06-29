package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hypernetix/hyperspot/libs/config"
)

var humaConfig = huma.DefaultConfig("HyperSpot Server API", "0.1.0")

type responseRecorder struct {
	middleware.WrapResponseWriter
	body *bytes.Buffer
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.WrapResponseWriter.Write(b)
}

func (r *responseRecorder) Flush() {
	if f, ok := r.WrapResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logAPIRequest(r)
		start := time.Now()

		// Wrap the response writer
		recorder := &responseRecorder{
			WrapResponseWriter: middleware.NewWrapResponseWriter(w, r.ProtoMajor),
			body:               bytes.NewBuffer(nil),
		}

		next.ServeHTTP(recorder, r)
		duration := time.Since(start).Seconds()
		logAPIResponse(r, recorder, duration)
	})
}

var apiHumaOperations = []huma.Operation{}

func RegisterEndpoint[I, O any](api huma.API, op huma.Operation, handler func(context.Context, *I) (*O, error)) {
	for _, o := range apiHumaOperations {
		if o.OperationID == op.OperationID {
			panic(fmt.Sprintf("Operation ID %s already registered", op.OperationID))
		}
		if o.Method == op.Method && o.Path == op.Path {
			panic(fmt.Sprintf("Operation %s %s already registered", op.Method, op.Path))
		}
		if o.Summary == op.Summary {
			panic(fmt.Sprintf("Operation summary %s already registered", op.Summary))
		}
	}
	op.BodyReadTimeout = config.GetServerTimeout()
	apiHumaOperations = append(apiHumaOperations, op)
	huma.Register(api, op, handler)
}

type RegisterAPIRoutesFunc func(api huma.API)

var registeredAPIRoutes []RegisterAPIRoutesFunc

func RegisterAPIRoutes(registerFunc RegisterAPIRoutesFunc) {
	registeredAPIRoutes = append(registeredAPIRoutes, registerFunc)
}

// SetupAPI initializes and configures the API
func SetupAPI(router *chi.Mux) huma.API {
	serverTimeout := config.GetServerTimeout() // Pass minimum timeout in seconds

	// Add other middleware
	// router.Use(middleware.Logger)
	router.Use(loggerMiddleware)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(serverTimeout))

	// Create Huma API
	api := humachi.New(router, humaConfig)

	for _, registerApiRoutesFunc := range registeredAPIRoutes {
		registerApiRoutesFunc(api)
	}

	// Add common middleware
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		// Add authentication, logging, etc.
		next(ctx)
	})

	return api
}
