DATABASE_URL ?= postgres://openagent:openagent@localhost:5432/openagent?sslmode=disable

.PHONY: migrate-up migrate-down test lint run-backend run-frontend

migrate-up:
	go run ./backend/cmd/migrate -url "$(DATABASE_URL)" up

migrate-down:
	go run ./backend/cmd/migrate -url "$(DATABASE_URL)" down

test:
	cd backend && go test ./... -count=1

lint:
	cd backend && go vet ./...
	cd frontend && npm run lint --if-present

run-backend:
	cd backend && go run ./cmd/server

run-frontend:
	cd frontend && npm start
