package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/harikishoreadabala/go-retry-mechanism/retry"
	_ "github.com/lib/pq"
)

// OrderService handles database operations with retry logic
type OrderService struct {
	db          *sql.DB
	retryConfig retry.Config
}

// NewOrderService creates a new service instance
func NewOrderService(db *sql.DB) *OrderService {
	return &OrderService{
		db: db,
		retryConfig: retry.Config{
			MaxRetries:     3,
			InitialBackoff: 50 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
			BackoffFactor:  2.0,
			JitterFactor:   0.1,
		},
	}
}

// CreateOrder creates an order with retry logic for transient failures
func (s *OrderService) CreateOrder(ctx context.Context, customerID string, amount float64) error {
	return retry.Do(ctx, s.retryConfig, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			if isTransientDBError(err) {
				return retry.RetryableError{Err: err}
			}
			return err
		}
		defer tx.Rollback()

		// Insert order
		var orderID int
		err = tx.QueryRowContext(ctx, `
			INSERT INTO orders (customer_id, amount, status, created_at) 
			VALUES ($1, $2, 'pending', NOW()) 
			RETURNING id`,
			customerID, amount,
		).Scan(&orderID)

		if err != nil {
			if isTransientDBError(err) {
				return retry.RetryableError{Err: err}
			}
			return err
		}

		// Update customer balance
		_, err = tx.ExecContext(ctx, `
			UPDATE customers 
			SET balance = balance - $1,
			    updated_at = NOW()
			WHERE id = $2`,
			amount, customerID,
		)

		if err != nil {
			if isTransientDBError(err) {
				return retry.RetryableError{Err: err}
			}
			return err
		}

		return tx.Commit()
	})
}

// isTransientDBError checks if database error is retryable
func isTransientDBError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Connection errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") {
		return true
	}

	// Deadlock
	if strings.Contains(errStr, "deadlock") {
		return true
	}

	// Timeout
	if strings.Contains(errStr, "timeout") {
		return true
	}

	return false
}

// Example with notification
func (s *OrderService) CreateOrderWithLogging(ctx context.Context, customerID string, amount float64) error {
	return retry.DoWithNotify(ctx, s.retryConfig, func() error {
		// Same logic as CreateOrder
		return s.CreateOrder(ctx, customerID, amount)
	}, func(err error, duration time.Duration) {
		fmt.Printf("Retrying after error: %v, waiting: %v\n", err, duration)
	})
}
