package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/harikishoreadabala/go-retry-mechanism/retry"
)

// RetryableHTTPClient wraps http.Client with retry logic
type RetryableHTTPClient struct {
	client      *http.Client
	retryConfig retry.Config
}

// NewRetryableHTTPClient creates a new HTTP client with retry capabilities
func NewRetryableHTTPClient() *RetryableHTTPClient {
	return &RetryableHTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		retryConfig: retry.Config{
			MaxRetries:     3,
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     5 * time.Second,
			BackoffFactor:  2.0,
			JitterFactor:   0.1,
		},
	}
}

// Do executes HTTP request with retry logic
func (c *RetryableHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var body []byte

	// Read body if present (to allow retry)
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body.Close()
	}

	err := retry.Do(ctx, c.retryConfig, func() error {
		// Reset body for each attempt
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		var err error
		resp, err = c.client.Do(req)
		if err != nil {
			return retry.RetryableError{Err: err}
		}

		// Check HTTP status
		if retry.IsRetryableHTTPStatus(resp.StatusCode) {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return retry.RetryableError{
				Err: fmt.Errorf("retryable HTTP status %d: %s", resp.StatusCode, string(bodyBytes)),
			}
		}

		return nil
	})

	return resp, err
}

// Example usage
func ExampleHTTPClient() {
	client := NewRetryableHTTPClient()
	ctx := context.Background()

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.example.com/payment",
		bytes.NewBufferString(`{"amount": 100}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(ctx, req)
	if err != nil {
		fmt.Printf("Request failed after retries: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Request succeeded with status: %d\n", resp.StatusCode)
}
