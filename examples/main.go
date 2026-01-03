package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/harikishoreadabala/go-retry-mechanism/retry"
)

func main() {
	fmt.Println("=== Go Retry Mechanism Examples ===\n")

	// Example 1: Simple retry with simulated failures
	fmt.Println("1. Simple Retry Example:")
	simpleRetryExample()

	// Example 2: HTTP client with retry
	fmt.Println("\n2. HTTP Client Retry Example:")
	httpRetryExample()

	// Example 3: Concurrent operations with retry
	fmt.Println("\n3. Concurrent Retry Example:")
	concurrentRetryExample()

	// Example 4: Custom backoff strategy
	fmt.Println("\n4. Custom Backoff Example:")
	customBackoffExample()
}

func simpleRetryExample() {
	var attempts int32

	err := retry.Do(context.Background(), retry.DefaultConfig(), func() error {
		current := atomic.AddInt32(&attempts, 1)
		fmt.Printf("  Attempt %d...\n", current)

		// Fail first 2 attempts
		if current < 3 {
			return retry.RetryableError{Err: errors.New("simulated failure")}
		}

		fmt.Println("  Success!")
		return nil
	})

	if err != nil {
		log.Printf("Failed after retries: %v", err)
	}
}

func httpRetryExample() {
	// Create a test server that fails first 2 requests
	var serverAttempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts := atomic.AddInt32(&serverAttempts, 1)
		fmt.Printf("  Server received request %d\n", attempts)

		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Service temporarily unavailable"))
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Payment processed",
		})
	}))
	defer server.Close()

	// Use our retryable HTTP client
	client := NewRetryableHTTPClient()
	req, _ := http.NewRequest("POST", server.URL+"/payment", nil)

	resp, err := client.Do(context.Background(), req)
	if err != nil {
		log.Printf("Request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	fmt.Printf("  Final result: %+v\n", result)
}

func concurrentRetryExample() {
	ctx := context.Background()
	results := make(chan string, 3)

	// Launch 3 concurrent operations
	for i := 1; i <= 3; i++ {
		go func(id int) {
			err := retry.Do(ctx, retry.Config{
				MaxRetries:     2,
				InitialBackoff: 50 * time.Millisecond,
				MaxBackoff:     500 * time.Millisecond,
				BackoffFactor:  2.0,
				JitterFactor:   0.2, // Higher jitter for concurrent ops
			}, func() error {
				// Simulate work
				fmt.Printf("  Worker %d attempting...\n", id)
				time.Sleep(10 * time.Millisecond)

				// Random failure
				if time.Now().UnixNano()%2 == 0 {
					return retry.RetryableError{Err: fmt.Errorf("worker %d failed", id)}
				}

				results <- fmt.Sprintf("Worker %d succeeded", id)
				return nil
			})

			if err != nil {
				results <- fmt.Sprintf("Worker %d failed after retries", id)
			}
		}(i)
	}

	// Collect results
	for i := 0; i < 3; i++ {
		fmt.Printf("  Result: %s\n", <-results)
	}
}

func customBackoffExample() {
	// Fast retry for local services
	fastConfig := retry.Config{
		MaxRetries:     5,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  1.5,
		JitterFactor:   0.1,
	}

	// Slow retry for external APIs
	slowConfig := retry.Config{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  3.0,
		JitterFactor:   0.2,
	}

	fmt.Println("  Fast retry config:", fastConfig)
	fmt.Println("  Slow retry config:", slowConfig)

	// Example with notification
	err := retry.DoWithNotify(context.Background(), slowConfig, func() error {
		fmt.Println("  Attempting external API call...")
		return retry.RetryableError{Err: errors.New("API timeout")}
	}, func(err error, backoff time.Duration) {
		fmt.Printf("  Retry notification: %v, waiting %v\n", err, backoff)
	})

	if err != nil {
		fmt.Printf("  Final error: %v\n", err)
	}
}
