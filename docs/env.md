# Environment Variables

## Required
- `DATABASE_URL` — `postgres://openagent:openagent@localhost:5432/openagent?sslmode=disable` (pgvector)
- `JWT_SECRET` — ≥32 chars, HS256, used for JWT and integration credential AES-GCM (SHA256 derived)
- `PORT` — default `8080`
- `LOG_LEVEL` — `debug|info|warn|error` (default `info`)
- `ENV` — `development|production`

## Optional LLM
- `LLM_PROVIDER` — `stub|openai|anthropic|google|openrouter|local` (default `stub`)
- `LLM_MODEL` — generic default `openai/gpt-4o-mini`
- `OPENAI_API_KEY` + `OPENAI_MODEL` (default `gpt-4o-mini`)
- `ANTHROPIC_API_KEY` + `ANTHROPIC_MODEL` (default `claude-3-5-sonnet-20241022`)
- `GOOGLE_API_KEY` + `GOOGLE_MODEL` (default `gemini-1.5-flash`)
- `OPENROUTER_API_KEY` + `OPENROUTER_MODEL` (default `anthropic/claude-3.5-sonnet`, e.g. `openai/gpt-4o`, `google/gemini-pro`, `meta-llama/llama-3.1-70b`) — **single key for many models, recommended**
- `LOCAL_MODEL` + `LOCAL_BASE_URL` (default `llama3.1:8b` at `http://localhost:11434/v1`)
- If no keys, `stub` provider/embedding (1536 dims) is used; set `OPENROUTER_API_KEY` + `OPENROUTER_MODEL` to enable real LLM without code changes

## Frontend (`frontend/src/environments/environment.ts`)
- `apiBase: '/api/v1'` (proxied)
- `wsBase: ws://localhost:8080/ws` (dev) / `wss://host/ws` (prod)

## Docker Compose
- `POSTGRES_USER/PASSWORD/DB=openagent`, `pgvector/pgvector:pg16`
- `backend` env: `DATABASE_URL=postgres://openagent:openagent@postgres:5432/...`, `JWT_SECRET=dev-...`, `PORT=8080`
- `frontend` → nginx, `80` → `4200`

## Security
- Never commit `.env`; `.env.example` only
- `JWT_SECRET` must be ≥32 chars in production, rotated via `config.Load`
- `credentials_encrypted` in `integrations` uses `JWT_SECRET` derived AES-GCM; rotation requires re-encrypt
