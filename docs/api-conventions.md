# API Conventions

## Base URL

```
/api/v1
```

## Authentication

```
Authorization: Bearer <JWT>
```

JWT payload:

```json
{
  "sub": "user-uuid",
  "org": "organization-uuid",
  "role": "owner|admin|member",
  "exp": 1717000000
}
```

Public routes: `POST /api/v1/auth/login`, `POST /api/v1/auth/register`, `GET /health`, `GET /ready`.

## Response Envelope

Success:

```json
{ "data": { ... }, "meta": { "page": 1, "pageSize": 20, "total": 100 } }
```

Error:

```json
{ "error": { "code": "VALIDATION_ERROR", "message": "human readable", "details": {...} } }
```

## Pagination

```
GET /api/v1/channels?projectId=...&page=1&pageSize=20
```

Defaults: `page=1`, `pageSize=20`, max 100.

## Sorting

```
GET /api/v1/messages?channelId=...&sort=created_at&order=desc
```

## Idempotency

Write endpoints accept `Idempotency-Key` header (stored 24h, not enforced in Phase 1 skeleton).

## WebSocket

```
GET /ws?token=<JWT>
```

- Auth via query `token` (browser WS cannot set headers).
- Messages are JSON: `{ "type": "message.created|typing|presence|task.updated", "payload": {...} }`
- Org-scoped rooms: server joins connection to `org:<id>` on connect.

## Status Codes

- 200 OK, 201 Created, 204 No Content, 400 Validation, 401 Unauthorized, 403 Forbidden, 404 Not Found, 409 Conflict, 422 Unprocessable, 500 Internal.

## Versioning

URL versioning (`/api/v1`). Breaking changes bump version, old version maintained 6 months.
