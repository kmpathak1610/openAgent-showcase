package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Job represents async work with Phase 4 enhancements
type Job struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"` // agent_run, ingest_document, etc.
	Payload    map[string]any `json:"payload"`
	Retries    int            `json:"retries"`     // max retries
	Timeout    time.Duration  `json:"timeout"`     // per-attempt timeout
	IdempotencyKey string     `json:"idempotencyKey"` // dedup key, defaults to ID
}

type Handler func(ctx context.Context, job Job) error

type Pool struct {
	queue    chan Job
	handlers map[string]Handler
	log      *slog.Logger
	wg       sync.WaitGroup
	mu       sync.RWMutex
	// idempotency: map key -> expiry
	idemMu sync.Mutex
	idem   map[string]time.Time
}

func New(size int, log *slog.Logger) *Pool {
	return &Pool{
		queue:    make(chan Job, size*4),
		handlers: make(map[string]Handler),
		log:      log,
		idem:     make(map[string]time.Time),
	}
}

func (p *Pool) Register(jobType string, h Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[jobType] = h
}

func (p *Pool) Enqueue(job Job) {
	if job.IdempotencyKey == "" { job.IdempotencyKey = job.ID + ":" + job.Type }
	if job.Timeout == 0 { job.Timeout = 30 * time.Second }
	if job.Retries == 0 { job.Retries = 3 }
	// idempotency check (24h window)
	p.idemMu.Lock()
	if exp, ok := p.idem[job.IdempotencyKey]; ok && time.Now().Before(exp) {
		p.idemMu.Unlock()
		p.log.Info("job deduplicated (idempotent)", "id", job.ID, "key", job.IdempotencyKey)
		return
	}
	p.idem[job.IdempotencyKey] = time.Now().Add(24 * time.Hour)
	// cleanup old entries occasionally
	if len(p.idem) > 1000 {
		for k, exp := range p.idem {
			if time.Now().After(exp) { delete(p.idem, k) }
		}
	}
	p.idemMu.Unlock()

	select {
	case p.queue <- job:
		p.log.Info("job enqueued", "id", job.ID, "type", job.Type)
	default:
		p.log.Error("job queue full, dropping", "id", job.ID)
	}
}

func (p *Pool) Start(ctx context.Context, concurrency int) {
	for i := 0; i < concurrency; i++ {
		p.wg.Add(1)
		go func(worker int) {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-p.queue:
					p.mu.RLock()
					h, ok := p.handlers[job.Type]
					p.mu.RUnlock()
					if !ok {
						p.log.Error("no handler for job", "type", job.Type)
						continue
					}
					// execute with retries, timeout, cancellation
					var lastErr error
					for attempt := 0; attempt <= job.Retries; attempt++ {
						// check cancellation before attempt
						select {
						case <-ctx.Done():
							p.log.Info("job cancelled (pool shutdown)", "id", job.ID)
							lastErr = context.Canceled
							break
						default:
						}
						attemptCtx, cancel := context.WithTimeout(ctx, job.Timeout)
						err := h(attemptCtx, job)
						cancel()
						if err == nil {
							p.log.Info("job succeeded", "id", job.ID, "attempt", attempt)
							lastErr = nil
							break
						}
						lastErr = err
						if ctx.Err() != nil {
							p.log.Info("job cancelled during execution", "id", job.ID, "err", err)
							break
						}
						// retryable? For Phase 4, all errors are retryable except context
						if attempt < job.Retries {
							backoff := time.Duration(1<<attempt) * time.Second
							if backoff > 30*time.Second { backoff = 30 * time.Second }
							p.log.Warn("job failed, retrying", "id", job.ID, "attempt", attempt, "backoff", backoff, "err", err)
							select {
							case <-time.After(backoff):
							case <-ctx.Done():
								lastErr = context.Canceled
								break
							}
						} else {
							p.log.Error("job failed after retries", "id", job.ID, "err", err)
						}
					}
					if lastErr != nil && lastErr != context.Canceled {
						// could persist to dead-letter queue; for now just log
					}
				}
			}
		}(i)
	}
}

func (p *Pool) Stop() { p.wg.Wait() }

// CancelJob is a no-op for in-memory queue (jobs already in flight can be cancelled via context)
// For Phase 4, we expose a method to signal cancellation via context; callers should cancel via parent context
