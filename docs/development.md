# Development

## Prereqs
- Go 1.25+, Node 22+, Docker, `make` (GnuWin32/scoop/choco) or use raw commands
- `cp .env.example .env` → set `DATABASE_URL`, `JWT_SECRET` (≥32 chars)

## Services
```bash
docker compose up -d postgres      # pgvector:16, 5432, healthcheck
go run -C backend ./cmd/migrate -url "$DATABASE_URL" up
# or: make migrate-up (from root) or go run -C backend ./cmd/migrate up (from root with -C)

# Backend (2 terminals)
cd backend && go run ./cmd/server          # :8080, /health /ready /metrics
# Frontend
cd frontend && npm install && npm start    # :4200 proxies /api /ws → :8080
# Full stack docker
docker compose up --build
```

## Make Targets (root)
- `make migrate-up` — `go run -C backend ./cmd/migrate -url $DATABASE_URL up`
- `make test` — `go test -C backend ./... -count=1 && npm --prefix frontend test`
- `make run-backend` — `cd backend && go run ./cmd/server`
- `make run-frontend` — `cd frontend && npm start`

## Tests
```bash
go test -C backend ./... -count=1          # 16 packages
npm --prefix frontend test                  # 2 suites 7 tests
npm --prefix frontend run build             # 325kB
```

## Migrations
- `backend/migrations/*.sql` ordered 000001-000009, `go run -C backend ./cmd/migrate up` applies, `down` drops (dev only)
- pgvector HNSW indexes: `document_chunks.embedding`, `memories.embedding`, `embeddings.embedding`

## Debugging
- Backend: `LOG_LEVEL=debug go run -C backend ./cmd/server`, structured `slog` JSON, `/metrics`
- Frontend: `ng serve --proxy-config proxy.conf.json`, `WsService` logs, `authGuard` redirects to `/login`
- Worker: logs `job enqueued/succeeded/failed` with retries

## Windows
- `make` via `winget install GnuWin32.Make` or `scoop install make` or use raw `go run -C backend ...`
- PowerShell: `$env:DATABASE_URL="postgres://..."` then `cd backend; go run ./cmd/migrate ...`
