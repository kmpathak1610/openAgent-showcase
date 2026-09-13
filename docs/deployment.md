# Deployment

## Docker Compose (local/prod)
```bash
docker compose up --build -d
# postgres (pgvector) → 5432, backend → 8080, frontend nginx → 4200
docker compose logs -f
```

## Production Hardening
- **DB pool:** `SetMaxOpenConns 20`, `MaxIdle 5`, `ConnMaxLifetime 30m`, `ConnMaxIdle 5m`, `PingContext 5s`
- **Rate limiting:** `middleware.RateLimit(10/s burst 20)` per IP/org/path → 429
- **Metrics:** `GET /metrics` Prometheus, `GET /health`/`/ready`
- **Logs:** `slog` JSON, `X-Request-ID`, latency, status
- **Migrations:** `go run -C backend ./cmd/migrate -url $DATABASE_URL up` (HNSW indexes, not concurrently in migration — for 1M+ chunks use `CREATE INDEX CONCURRENTLY` manually)
- **Worker:** in-memory channel → swap to durable `pg` queue or BullMQ + Redis for `WS` pub/sub
- **Secrets:** `JWT_SECRET` ≥32 chars, `credentials_encrypted` AES-GCM, never log raw `credentials`

## Scaling
- **WS:** single hub → Redis pub/sub for `BroadcastToOrg`/`Channel`
- **RAG:** HNSW already, for 1M+ use `ivfflat` tuning, `pgbouncer`
- **LLM:** stub → real via `llm.Registry` (no domain changes), set `OPENAI_API_KEY` etc.

## Health Checks
- `postgres` healthcheck `pg_isready`
- `backend` depends_on `postgres` healthy
- `frontend` nginx `try_files` → `index.html`
