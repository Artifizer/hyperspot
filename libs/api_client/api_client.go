package api_client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"errors"

	"github.com/hypernetix/hyperspot/libs/core"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
)

// Options represents client configuration options
type Options struct {
	Verbose     int
	LLMService  string
	LLMURL      string
	Model       string
	Temperature float64
}

// APIError represents an error from the API
type APIError struct {
	StatusCode int
	Message    string
	RawBody    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}

// BaseAPIClient provides common functionality for API clients
type BaseAPIClient struct {
	name             string
	baseUrl          string
	options          Options
	likelyIsOnline   bool
	lastAlive        time.Time
	shortTimeoutSec  int
	longTimeoutSec   int
	httpShortTimeout *http.Client
	httpLongTimeout  *http.Client
	mutex            utils.DebugMutex
	// background discovery feature
	enableDiscovery   bool
	discoveryIsActive bool
	// retry configuration
	retryTimeout time.Duration // 0 disables retries, default 500ms
}

// NewBaseAPIClient creates a new base API client with default 500ms retry policy
func NewBaseAPIClient(
	name string,
	baseURL string,
	shortTimeoutSec int,
	longTimeoutSec int,
	insecureSkipVerify bool,
	enableDiscovery bool,
) *BaseAPIClient {
	return &BaseAPIClient{
		name:            name,
		baseUrl:         baseURL,
		likelyIsOnline:  false,
		shortTimeoutSec: shortTimeoutSec,
		longTimeoutSec:  longTimeoutSec,
		enableDiscovery: enableDiscovery,
		retryTimeout:    500 * time.Millisecond,
		httpShortTimeout: &http.Client{
			Timeout: time.Duration(shortTimeoutSec) * time.Second,
			Transport: &http.Transport{
				ResponseHeaderTimeout: time.Duration(shortTimeoutSec) * time.Second,
				ExpectContinueTimeout: time.Duration(shortTimeoutSec) * time.Second,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: insecureSkipVerify,
				},
			},
		},
		httpLongTimeout: &http.Client{
			Timeout: time.Duration(longTimeoutSec) * time.Second,
			Transport: &http.Transport{
				ResponseHeaderTimeout: time.Duration(longTimeoutSec) * time.Second,
				ExpectContinueTimeout: time.Duration(longTimeoutSec) * time.Second,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: insecureSkipVerify,
				},
			},
		},
	}
}

type HTTPResponse struct {
	UpstreamResponse *http.Response
	BodyBytes        []byte
	Duration         time.Duration
}

func (c *BaseAPIClient) doRequest(ctx context.Context, method string, path string, api_key string, body []byte, logRequest bool, longTimeout bool) (*http.Response, error) {
	// Create request with the original body
	if !strings.HasPrefix(path, c.baseUrl) {
		path = c.baseUrl + path
	}

	req, err := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set headers
	if api_key != "" {
		req.Header.Set("Authorization", "Bearer "+api_key)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.name)
	req.Header.Set("Accept", "application/json")

	// Log request using the body copy
	if logRequest {
		c.logUpstreamRequest(req)
	}

	// Execute request with retry logic
	return c.doRequestWithRetry(ctx, req, body, longTimeout)
}

// SetRetryPolicy sets the retry timeout duration. Set to 0 to disable retries.
func (c *BaseAPIClient) SetRetryTimeout(retryTimeout time.Duration) {
	c.retryTimeout = retryTimeout
}

// doRequestWithRetry executes HTTP request with exponential backoff retry logic
func (c *BaseAPIClient) doRequestWithRetry(ctx context.Context, req *http.Request, body []byte, longTimeout bool) (*http.Response, error) {
	// If retry policy is 0, disable retries
	if c.retryTimeout == 0 {
		// Single attempt without retries
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		if longTimeout {
			return c.httpLongTimeout.Do(req)
		}
		return c.httpShortTimeout.Do(req)
	}

	const baseDelay = 10 * time.Millisecond
	const multiplier = 4

	startTime := time.Now()
	var lastErr error
	attempt := 0

	for {
		// Check if context is cancelled before retry
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Create a new request body reader for each attempt
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		// Send request
		var resp *http.Response
		var err error
		if longTimeout {
			resp, err = c.httpLongTimeout.Do(req)
		} else {
			resp, err = c.httpShortTimeout.Do(req)
		}

		// If successful, return immediately
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable (connection errors)
		if !isRetryableError(err) {
			// Non-retryable error, return immediately
			return nil, err
		}

		// Check if we have time left for another retry
		elapsed := time.Since(startTime)
		if elapsed >= c.retryTimeout {
			break // Time exhausted
		}

		// Calculate progressive delay: 10ms * 4^attempt
		delay := baseDelay
		for i := 0; i < attempt; i++ {
			delay *= multiplier
		}

		// Check if we have enough time left for this delay
		remainingTime := c.retryTimeout - elapsed
		if delay > remainingTime {
			// Use remaining time or skip if too little time left
			if remainingTime < time.Millisecond {
				break // Not enough time for meaningful retry
			}
			delay = remainingTime
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}

		attempt++
	}

	// All retries exhausted within time window
	totalTime := time.Since(startTime)
	return nil, fmt.Errorf("request failed after %d attempts in %v: %w", attempt+1, totalTime, lastErr)
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Connection errors that are typically transient
	retryableErrors := []string{
		"connection refused",
		"connection reset by peer",
		"no such host",
		"network is unreachable",
		"timeout",
		"temporary failure",
		"service unavailable",
		"i/o timeout",
		"context deadline exceeded",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryable) {
			return true
		}
	}

	// Check for specific network error types
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.Temporary()
	}

	// Check for syscall errors (like ECONNREFUSED)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true // Most network operation errors are retryable
	}

	return false
}

func (c *BaseAPIClient) Request(ctx context.Context, method string, path string, api_key string, body []byte, logRequest bool) (*http.Response, error) {
	return c.doRequest(ctx, method, path, api_key, body, logRequest, false)
}

func (c *BaseAPIClient) RequestWithLongTimeout(ctx context.Context, method string, path string, api_key string, body []byte, logRequest bool) (*http.Response, error) {
	return c.doRequest(ctx, method, path, api_key, body, logRequest, true)
}

func (c *BaseAPIClient) doRequestAndParse(ctx context.Context, method string, path string, api_key string, body []byte, longTimeout bool) (*HTTPResponse, error) {
	timeStart := time.Now()
	ret := &HTTPResponse{}

	var upstreamResp *http.Response
	var err error

	if longTimeout {
		upstreamResp, err = c.RequestWithLongTimeout(ctx, method, path, api_key, body, true)
	} else {
		upstreamResp, err = c.Request(ctx, method, path, api_key, body, true)
	}
	if upstreamResp == nil {
		logging.Debug("Upstream request '%s %s%s' failed: %s", method, c.baseUrl, path, err)
		c.SetUpstreamLikelyIsOffline(ctx)
		return nil, err
	}
	if upstreamResp.Body == nil {
		c.SetUpstreamLikelyIsOffline(ctx)
		return nil, fmt.Errorf("%s: %s %s failed to get response body", c.GetName(), method, path)
	}
	defer upstreamResp.Body.Close()

	var response_body []byte
	response_body, err = io.ReadAll(upstreamResp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %s %s failed to read response body: %w", c.GetName(), method, path, err)
	}

	if upstreamResp.StatusCode == http.StatusOK {
		c.SetUpstreamLikelyIsOnline()
	}
	ret.UpstreamResponse = upstreamResp
	ret.Duration = time.Since(timeStart)

	var data interface{}
	if err := json.Unmarshal(response_body, &data); err != nil {
		return ret, fmt.Errorf("failed to unmarshal response body: %w, data: %v", err, data)
	}

	c.logUpstreamResponse(method, path, upstreamResp.StatusCode, string(response_body), ret.Duration)
	ret.BodyBytes = response_body

	if upstreamResp.StatusCode >= http.StatusInternalServerError { // 5xx
		c.SetUpstreamLikelyIsOffline(ctx)
	}

	err = nil
	if upstreamResp.StatusCode >= http.StatusBadRequest { // 4xx
		err = fmt.Errorf("upstream HTTP %s request failed: %s%s\n%s", method, c.baseUrl, path, string(response_body))
	}

	return ret, err
}

func (c *BaseAPIClient) RequestAndParse(ctx context.Context, method string, path string, api_key string, body []byte) (*HTTPResponse, error) {
	return c.doRequestAndParse(ctx, method, path, api_key, body, false)
}

func (c *BaseAPIClient) RequestAndParseWithLongTimeout(ctx context.Context, method string, path string, api_key string, body []byte) (*HTTPResponse, error) {
	return c.doRequestAndParse(ctx, method, path, api_key, body, true)
}

func (c *BaseAPIClient) Get(ctx context.Context, path string, api_key string) (*HTTPResponse, error) {
	return c.RequestAndParse(ctx, "GET", path, api_key, nil)
}

func (c *BaseAPIClient) GetWithLongTimeout(ctx context.Context, path string, api_key string) (*HTTPResponse, error) {
	return c.RequestAndParseWithLongTimeout(ctx, "GET", path, api_key, nil)
}

func (c *BaseAPIClient) Post(ctx context.Context, path string, api_key string, body []byte) (*HTTPResponse, error) {
	return c.RequestAndParse(ctx, "POST", path, api_key, body)
}

func (c *BaseAPIClient) PostWithLongTimeout(ctx context.Context, path string, api_key string, body []byte) (*HTTPResponse, error) {
	return c.RequestAndParseWithLongTimeout(ctx, "POST", path, api_key, body)
}

// GetName returns the name of the API client
func (c *BaseAPIClient) GetName() string {
	return c.baseUrl
}

func (c *BaseAPIClient) LikelyIsAlive() bool {
	return c.likelyIsOnline
}

func (c *BaseAPIClient) SetUpstreamLikelyIsOnline() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.likelyIsOnline {
		if c.lastAlive.IsZero() {
			logging.Info("The %s has been discovered", c.GetFullName())
		} else {
			logging.Info("The %s is back online", c.GetFullName())
		}
		c.likelyIsOnline = true
	}
	c.lastAlive = time.Now()
}

func (c *BaseAPIClient) SetUpstreamLikelyIsOffline(ctx context.Context) {
	if ctx.Err() != nil {
		// ctx.Err() can return only context.Canceled or context.DeadlineExceeded
		// in both cases we consider it as client initiated termination and so
		// we don't set the server as offline
		return
	}

	if c.likelyIsOnline {
		c.mutex.Lock()
		c.likelyIsOnline = false
		c.mutex.Unlock()

		logging.Warn("The %s service is likely offline", c.GetFullName())
	}

	c.StartOnlineWatchdog(ctx)
}

// The StartOnlineWatchdog() function runs in a separate goroutine until the API client becomes online
// If the API client is offline, it will set the likelyIsOnline flag to false and start the discovery process
func (c *BaseAPIClient) StartOnlineWatchdog(ctx context.Context) {
	c.mutex.Lock()
	if !c.enableDiscovery || c.discoveryIsActive {
		c.mutex.Unlock()
		return
	}

	c.discoveryIsActive = true
	c.mutex.Unlock()

	go func() {
		defer func() {
			c.mutex.Lock()
			c.discoveryIsActive = false
			c.mutex.Unlock()
		}()

		for {
			var sleep time.Duration
			c.mutex.Lock()
			last := c.lastAlive
			c.mutex.Unlock()

			switch {
			case last.IsZero():
				// If the last alive time is zero, we assume the service is not yet discovered
				sleep = time.Second / 2
			case time.Since(last) > time.Minute:
				sleep = time.Second * 2
			case time.Since(last) > 10*time.Second:
				sleep = time.Second
			default:
				sleep = time.Second / 2
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(sleep):
				// continue to check the service status
			}

			resp, err := c.Request(context.Background(), "GET", c.baseUrl, "", nil, false)
			if err != nil {
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				continue
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				continue
			}
			var data any
			if err := json.Unmarshal(body, &data); err != nil {
				continue
			}

			// fmt.Printf("The service is alive: %s\n", c.GetFullName())

			c.mutex.Lock()
			c.lastAlive = time.Now()
			c.mutex.Unlock()

			c.SetUpstreamLikelyIsOnline()
			return
		}
	}()
}

// GetEndpoint returns the API endpoint
func (c *BaseAPIClient) GetBaseURL() string {
	return c.baseUrl
}

func (c *BaseAPIClient) GetFullName() string {
	if c.name == "" {
		return c.baseUrl
	}
	return fmt.Sprintf("%s (%s)", c.baseUrl, c.name)
}

func init() {
	core.RegisterModule(&core.Module{
		Name: "api_client",
	})
}
