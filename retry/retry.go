package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"syscall"
	"time"
)

type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	JitterFactor   float64
}

func DefaultConfig() Config {
	return Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  1.5,
		JitterFactor:   0.1,
	}
}

type Retryable func() error

type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string {
	return e.Err.Error()
}

func (e RetryableError) Unwrap() error {
	return e.Err
}

func Do(ctx context.Context, config Config, operation Retryable) error {
	var lastErr error

	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		fmt.Printf("Attempt %d/%d: %v\n", attempt, config.MaxRetries, lastErr)

		if !IsRetryable(lastErr) {
			return fmt.Errorf("not retryable error: %w", lastErr)
		}

		if attempt == config.MaxRetries {
			break
		}

		backOff := calculateBackoff(attempt, config)

		select {
		case <-time.After(backOff):
		case <-ctx.Done():
			return fmt.Errorf("timed out: %w", lastErr)
		}

	}
	return fmt.Errorf("retries exceeded: %w", lastErr)
}

func calculateBackoff(attempt int, config Config) time.Duration {

	backoff := float64(config.InitialBackoff) * math.Pow(config.BackoffFactor, float64(attempt))

	if backoff > float64(config.MaxBackoff) {
		backoff = float64(config.MaxBackoff)
	}

	jitter := (rand.Float64() - 0.5) * config.JitterFactor * backoff

	finalBackoff := backoff + jitter

	if finalBackoff < 0 {
		finalBackoff = 0
	}

	return time.Duration(finalBackoff)

}

func DoWithNotify(ctx context.Context, config Config, operation Retryable, notify func(error, time.Duration)) error {
	var lastErr error
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		if !IsRetryable(lastErr) {
			return fmt.Errorf("not retryable error: %w", lastErr)
		}

		if attempt == config.MaxRetries {
			break
		}
		backoff := calculateBackoff(attempt, config)
		if notify != nil {
			notify(lastErr, backoff)
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return fmt.Errorf("timed out: %w", lastErr)
		}

	}

	return fmt.Errorf("retries exceeded: %w", lastErr)
}

func IsRetryable(err error) bool {
	// Don't retry context cancellations
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Always retry our explicitly marked errors
	var retryableErr RetryableError
	if errors.As(err, &retryableErr) {
		return true
	}

	// Network errors are usually retryable
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Temporary() || netErr.Timeout()
	}

	// Connection refused/reset
	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) {
		return errors.Is(syscallErr, syscall.ECONNREFUSED) ||
			errors.Is(syscallErr, syscall.ECONNRESET)
	}

	return false
}
func IsRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
