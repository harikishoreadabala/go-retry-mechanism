package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrySuccess(t *testing.T) {
	attempts := 0

	operation := func() error {
		attempts++
		if attempts < 3 {
			return RetryableError{errors.New("retryable error")}
		}
		return nil
	}

	config := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  1.5,
		JitterFactor:   0.5,
	}

	err := Do(context.Background(), config, operation)

	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}

}

func TestRetryMaxAttemptsExceeded(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return RetryableError{Err: errors.New("always fails")}
	}

	config := Config{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		BackoffFactor:  2.0,
		JitterFactor:   0.1,
	}

	err := Do(context.Background(), config, operation)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if attempts != 2 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	operation := func() error {
		attempts++
		if attempts == 2 {
			cancel()
		}
		return RetryableError{errors.New("temporary failure")}
	}

	config := Config{
		MaxRetries:     5,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		BackoffFactor:  2.0,
		JitterFactor:   0.1,
	}
	err := Do(ctx, config, operation)

	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}

	if attempts != 2 {
		t.Fatalf("expected at most 3 attempts, got %d", attempts)
	}

}

func TestBackOffCalculation(t *testing.T) {
	config := Config{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		BackoffFactor:  2.0,
		JitterFactor:   0,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{10, 5 * time.Second},
	}

	for _, tt := range tests {
		backoff := calculateBackoff(tt.attempt, config)
		if backoff != tt.expected {
			t.Fatalf("expected backoff of %v, got %v", tt.expected, backoff)
		}
	}
}
