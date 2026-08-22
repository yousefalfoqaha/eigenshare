# AGENTS.md

All Go commands run from `backend/`.

## Verify

```
gofmt -w .
go vet ./...
go test ./...
```

Run a specific test: `go test -run TestName ./...`

## Run the server

```
mise run dev
```

Or directly: `go run .` from `backend/`.

Listens on `:8080`. Reads `schema.sql` (embedded in `infra/`) and writes `./data.db` and `./data/` relative to the cwd; both are gitignored.

On first run (empty DB), seeds a default admin account: username `admin`, password `admin`. Change it immediately via the users page.

## Layout

- `backend/infra` — `Database` (sqlite via modernc), embeds `schema.sql`. No domain imports.
- `backend/timestamp` — shared `Timestamp` type.
- `backend/middleware` — `Logging` middleware.
- `backend/user` — domain: `User` (with `IsAdmin`), `Session`, `NewUser`/`NewSession` (validation only, no crypto), sentinel errors, `CreateRequest`/`UpdateRequest` DTOs. `UserRepository` + `SessionRepository` over `infra.Database`.
- `backend/auth` — `Credentials` DTO, `AuthService` (PBKDF2-HMAC-SHA256 password hashing via stdlib `crypto/*`, `SeedAdmin`, `CreateUser`, `UpdateUser`, `DeleteUser` with self-delete + last-admin + self-demote guards), `RequireAuth` + `RequireAdmin` + `OptionalAuth` middleware, private `userKey{}` context key. `auth.UserFromContext(ctx)` reads the principal.
- `backend/share` — one package: domain (`Share`, `File`, `ShareSummary`, constructors, sentinel errors), `ShareRequest`/`FileRequest` DTO (json-tagged, service input), `ShareRepository` (lookup by ID or handle), `FileStorage` (disk), `ShareService` (returns domain errors only — no `net/http`/`api` imports), `ExpiryJob`.
- `backend/web` — HTML pages + JSON/streaming endpoints. `Page`/`SharesView`/`ShareView`/`SharePublicView`/`UsersView`/`UserFormView` view structs, `Error`/`NewError`/`MakeHandler`/`JSON`/`shareAPIError` (maps `share.Err*` → HTTP status), embedded `templates/*.html` + `static/static.css`. Owns ALL HTTP routes.
- `backend/main.go` — wiring. Protected mux (management routes) is mounted at `/` behind `auth.RequireAuth`; admin user routes are additionally wrapped with `auth.RequireAdmin`; public routes (`/login`, `/logout`, `/static/`, `/s/{handle}`) are registered directly on the main mux with `OptionalAuth` on share routes.

## Routes

| Route | Type | Access |
|---|---|---|
| `GET /` | redirect → `/shares` | authed |
| `GET /static/` | CSS | public |
| `GET/POST /login` | HTML form | public |
| `POST /logout` | redirect | public |
| `GET /s/{handle}` | HTML public share view | public (optional auth) |
| `GET /s/{handle}/download` | zip | public (optional auth) |
| `GET /s/{handle}/files/{fileID}/download` | individual file | public (optional auth) |
| `GET /shares` | HTML management list | authed |
| `GET /shares/new` | HTML form + JS (auto-generated handle) | authed |
| `POST /shares` | JSON (JS create) | authed |
| `GET /shares/{handle}/edit` | HTML management view | authed |
| `GET /shares/{handle}/progress` | JSON (JS poll) | authed |
| `POST /shares/{handle}/delete` | form POST → redirect | authed |
| `PUT /shares/{handle}/files/{fileID}/stream` | stream (JS chunked upload) | authed |
| `GET /users` | HTML list | authed |
| `GET /users/new` | HTML form | admin |
| `POST /users` | form POST → redirect | admin |
| `GET /users/{id}/edit` | HTML form (prefilled) | admin |
| `POST /users/{id}` | form POST → redirect | admin |
| `POST /users/{id}/delete` | form POST → redirect | admin |

## Conventions

- Password hashing: stdlib PBKDF2 only (no `golang.org/x/crypto`). Format `pbkdf2-sha256:iter:saltB64:hashB64`, 600k iterations.
- ShareService returns domain sentinel errors (`share.Err*`), never HTTP status codes. The `web.shareAPIError` mapper translates them to HTTP statuses.
- DTOs (`ShareRequest`, `FileRequest`, `Credentials`, `CreateRequest`, `UpdateRequest`) own json/form tags and are the service input contract. Controllers decode directly into them — no mapping layer.
- New page data structs: reuse domain models where they fit (`share.ShareSummary`, `*share.Share`, `*user.User`); add a view wrapper only for page chrome + presentation data that doesn't belong on the domain. Do not scatter per-page structs.
- Share handles are the public URL identifier (`/s/{handle}`), min 3 chars. The management namespace (`/shares/{handle}/edit`) is separate from the public namespace (`/s/{handle}`).
- No comments in code unless asked.
