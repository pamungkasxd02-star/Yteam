package util

import (
	"context"
	"strings"
	"time"
)

type RetryOptions struct {
	Attempts int
	Delay    time.Duration
	Factor   float64
	MaxDelay time.Duration
	RetryIf  func(error) bool
	OnRetry  func(attempt int, err error)
}

func Retry(ctx context.Context, fn func() error, options RetryOptions) error {
	if options.Attempts <= 0 {
		options.Attempts = 3
	}
	if options.Delay <= 0 {
		options.Delay = 500 * time.Millisecond
	}
	if options.Factor <= 0 {
		options.Factor = 2
	}
	if options.MaxDelay <= 0 {
		options.MaxDelay = 10 * time.Second
	}
	if options.RetryIf == nil {
		options.RetryIf = IsTransient
	}
	var last error
	for attempt := 0; attempt < options.Attempts; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			last = err
			if attempt == options.Attempts-1 || !options.RetryIf(err) {
				return err
			}
			if options.OnRetry != nil {
				options.OnRetry(attempt+1, err)
			}
		}
		delay := time.Duration(float64(options.Delay) * pow(options.Factor, attempt))
		if delay > options.MaxDelay {
			delay = options.MaxDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}

func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, value := range []string{"load failed", "network connection was lost", "network request failed", "failed to fetch", "econnreset", "econnrefused", "etimedout", "socket hang up"} {
		if strings.Contains(message, value) {
			return true
		}
	}
	return false
}

func pow(base float64, exponent int) float64 {
	result := 1.0
	for i := 0; i < exponent; i++ {
		result *= base
	}
	return result
}
