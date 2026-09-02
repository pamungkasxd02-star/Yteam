package util

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryOnlyRetriesTransientErrors(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return errors.New("network request failed")
		}
		return nil
	}, RetryOptions{Delay: time.Millisecond})
	if err != nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
	attempts = 0
	err = Retry(context.Background(), func() error { attempts++; return errors.New("validation failed") }, RetryOptions{Delay: time.Millisecond})
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}
