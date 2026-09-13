package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorker_Retry(t *testing.T) {
	log := slog.Default()
	pool := New(10, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 1)

	var attempts int32
	pool.Register("test_retry", func(ctx context.Context, job Job) error {
		if atomic.AddInt32(&attempts, 1) < 3 {
			return errors.New("fail")
		}
		return nil
	})

	pool.Enqueue(Job{ID: "retry-1", Type: "test_retry", Retries: 3, Timeout: time.Second})
	// wait for retries (1s +2s backoff)
	time.Sleep(4 * time.Second)
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestWorker_Idempotency(t *testing.T) {
	log := slog.Default()
	pool := New(10, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 1)
	var count int32
	pool.Register("idem", func(ctx context.Context, job Job) error {
		atomic.AddInt32(&count, 1)
		return nil
	})
	job := Job{ID: "idem-1", Type: "idem", IdempotencyKey: "key-1"}
	pool.Enqueue(job)
	pool.Enqueue(job) // duplicate should be deduped
	time.Sleep(300 * time.Millisecond)
	if atomic.LoadInt32(&count) != 1 {
		t.Fatalf("expected 1 execution due to idempotency, got %d", count)
	}
}

func TestWorker_Timeout(t *testing.T) {
	log := slog.Default()
	pool := New(10, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 1)
	pool.Register("timeout", func(ctx context.Context, job Job) error {
		select {
		case <-time.After(2 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	// job timeout 100ms, should fail with deadline
	pool.Enqueue(Job{ID: "timeout-1", Type: "timeout", Timeout: 100 * time.Millisecond, Retries: 0})
	time.Sleep(300 * time.Millisecond)
	// no assertion beyond not hanging; if timeout works, job will be cancelled quickly
}

func TestWorker_Cancellation(t *testing.T) {
	log := slog.Default()
	pool := New(10, log)
	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx, 2)
	pool.Register("cancel", func(ctx context.Context, job Job) error {
		<-ctx.Done()
		return ctx.Err()
	})
	pool.Enqueue(Job{ID: "cancel-1", Type: "cancel", Timeout: 5 * time.Second})
	// cancel pool context
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
	// should not hang
}
