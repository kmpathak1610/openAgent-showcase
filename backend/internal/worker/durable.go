package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"openagent/internal/repository"
)

// DurableWorker extends Pool with PG-backed durable queue (P1-4)
type DurableWorker struct {
	pool     *Pool
	repo     *repository.DB
	workerID string
}

func NewDurable(pool *Pool, repo *repository.DB) *DurableWorker {
	return &DurableWorker{
		pool:     pool,
		repo:     repo,
		workerID: uuid.NewString()[:8],
	}
}

func (d *DurableWorker) EnqueueDurable(ctx context.Context, job Job) error {
	if job.IdempotencyKey == "" {
		job.IdempotencyKey = job.ID + ":" + job.Type
	}
	if job.Timeout == 0 {
		job.Timeout = 30 * time.Second
	}
	if job.Retries == 0 {
		job.Retries = 3
	}
	payloadJSON, _ := json.Marshal(job.Payload)
	// Insert with ON CONFLICT DO NOTHING for idempotency
	_, err := d.repo.ExecContext(ctx, `INSERT INTO jobs (organization_id, type, payload, status, attempts, max_retries, idempotency_key, next_retry_at) VALUES ((SELECT organization_id FROM tasks WHERE id = $1::uuid LIMIT 1), $2, $3, 'pending', 0, $4, $5, now()) ON CONFLICT (idempotency_key) DO NOTHING`, job.ID, job.Type, payloadJSON, job.Retries, job.IdempotencyKey)
	if err != nil {
		// Fallback: try without organization_id (for non-task jobs like ingest_document)
		_, err2 := d.repo.ExecContext(ctx, `INSERT INTO jobs (type, payload, status, attempts, max_retries, idempotency_key, next_retry_at) VALUES ($1, $2, 'pending', 0, $3, $4, now()) ON CONFLICT (idempotency_key) DO NOTHING`, job.Type, payloadJSON, job.Retries, job.IdempotencyKey)
		if err2 != nil {
			return fmt.Errorf("enqueue durable: %w (first: %v)", err2, err)
		}
	}
	// Also enqueue in-memory for fast path (if worker is running, it will claim from DB anyway)
	d.pool.Enqueue(job)
	return nil
}

func (d *DurableWorker) StartDurable(ctx context.Context, concurrency int) {
	for i := 0; i < concurrency; i++ {
		d.pool.wg.Add(1)
		go func(workerNum int) {
			defer d.pool.wg.Done()
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					job, err := d.claimJob(ctx)
					if err != nil {
						if err != sql.ErrNoRows {
							d.pool.log.Error("durable claim failed", "err", err)
						}
						continue
					}
					if job == nil {
						continue
					}
					d.executeClaimed(ctx, *job)
				}
			}
		}(i)
	}
}

func (d *DurableWorker) claimJob(ctx context.Context) (*Job, error) {
	tx, err := d.repo.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id uuid.UUID
	var jobType string
	var payloadJSON []byte
	var retries, attempts int
	var timeout sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT id, type, payload, max_retries, attempts FROM jobs WHERE status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &jobType, &payloadJSON, &retries, &attempts)
	if err != nil {
		return nil, err
	}
	// Claim
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status = 'claimed', claimed_by = $2, claimed_at = now(), lease_until = now() + interval '30 seconds', updated_at = now() WHERE id = $1`, id, d.workerID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var payload map[string]any
	_ = json.Unmarshal(payloadJSON, &payload)
	job := &Job{
		ID:      id.String(),
		Type:    jobType,
		Payload: payload,
		Retries: retries,
		Timeout: 30 * time.Second,
	}
	if timeout.Valid {
		job.Timeout = time.Duration(timeout.Int64) * time.Second
	}
	_ = attempts
	return job, nil
}

func (d *DurableWorker) executeClaimed(ctx context.Context, job Job) {
	d.pool.mu.RLock()
	h, ok := d.pool.handlers[job.Type]
	d.pool.mu.RUnlock()
	if !ok {
		d.pool.log.Error("no handler for durable job", "type", job.Type)
		d.markFailed(ctx, job.ID, "no handler")
		return
	}
	// Mark running
	_, _ = d.repo.ExecContext(ctx, `UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = now() WHERE id = $1::uuid`, job.ID)
	var lastErr error
	for attempt := 0; attempt <= job.Retries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, job.Timeout)
		err := h(attemptCtx, job)
		cancel()
		if err == nil {
			_, _ = d.repo.ExecContext(ctx, `UPDATE jobs SET status = 'succeeded', completed_at = now(), updated_at = now() WHERE id = $1::uuid`, job.ID)
			d.pool.log.Info("durable job succeeded", "id", job.ID, "attempt", attempt)
			return
		}
		lastErr = err
		if attempt < job.Retries {
			backoff := time.Duration(1<<attempt) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			_, _ = d.repo.ExecContext(ctx, `UPDATE jobs SET status = 'pending', next_retry_at = now() + $2::interval, updated_at = now() WHERE id = $1::uuid`, job.ID, fmt.Sprintf("%d seconds", int(backoff.Seconds())))
			time.Sleep(backoff / 10) // small sleep to avoid tight loop, real backoff via next_retry_at
		}
	}
	// Terminal failure -> dead
	_, _ = d.repo.ExecContext(ctx, `UPDATE jobs SET status = 'dead', updated_at = now() WHERE id = $1::uuid`, job.ID)
	d.pool.log.Error("durable job dead after retries", "id", job.ID, "err", lastErr)
}

func (d *DurableWorker) markFailed(ctx context.Context, jobID, reason string) {
	_, _ = d.repo.ExecContext(ctx, `UPDATE jobs SET status = 'dead', updated_at = now() WHERE id = $1::uuid`, jobID)
}
