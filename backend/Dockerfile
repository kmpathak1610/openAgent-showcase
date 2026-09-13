FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/server
# also build migrate helper (optional)
RUN CGO_ENABLED=0 go build -o /bin/migrate ./cmd/migrate || echo "no migrate cmd"

FROM mcr.microsoft.com/playwright:v1.51.0-jammy
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
# Playwright browsers are already in the base image (chromium, firefox, webkit)
# Ensure dependencies for headless chromium
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
ENV PLAYWRIGHT_ENABLED=true
COPY --from=build /bin/server /bin/server
COPY --from=build /bin/migrate /bin/migrate
COPY migrations /migrations
EXPOSE 8080
CMD ["/bin/server"]
