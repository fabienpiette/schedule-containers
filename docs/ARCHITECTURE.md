# Architecture

This document describes the high-level architecture of `schedule-containers`.
If you want to familiarize yourself with the codebase, you are in the right place.

## Bird's Eye View

Schedule Containers is a single Go binary that keeps Docker containers running on a schedule. You define cron expressions for when a container should start and stop, and the scheduler ensures those actions happen at the right time. Stacks let you schedule groups of containers (Docker Compose stacks) together with a single cron pair. Tags let you define a schedule template and apply it to multiple containers at once. On-demand wake lets stopped containers be started via a URL and automatically stopped after inactivity. Role-based authentication (reader, writer, admin) protects all management endpoints; OIDC login via Pocket ID or any OpenID Connect provider works alongside local passwords. A web dashboard and REST API provide runtime management; a CLI handles offline operations and YAML import/export.

On startup, the `serve` command loads persisted schedules, stacks, and tags from SQLite, registers them with an in-process cron runner, starts the on-demand manager, and starts an HTTP server. If OIDC is configured, the login page shows a Pocket ID button alongside local password auth. If no users exist, all requests redirect to `/setup` to create the initial admin account. After setup, all management endpoints require session-based authentication with role checks. When a cron job fires, the scheduler calls the Docker API to start or stop the target container (or all containers in a stack). When a user accesses `/wake/<container>/`, the on-demand manager starts the container and redirects to the configured URL once healthy.

```
                    ┌─────────────────────┐
                    │   CLI (cobra)        │
                    │  serve / schedule /   │
                    │  containers / tag     │
                    └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐
                    │   internal/cli/       │
                    │   serve.go            │──── composition root
                    └─┬────────┬────────┬──┘
                      │        │        │
          ┌───────────▼┐  ┌──▼────┐ ┌▼──────────┐
          │  store/      │  │ web/  │ │ ondemand/ │
          │  (SQLite)    │  │(HTTP) │ │(wake+idle)│
          └──────┬──────┘  └──┬────┘ └─────┬─────┘
                 │            │             │
          ┌──────▼────────────▼────────────▼───┐
          │            scheduler/                │
          │ (cron runner, per-container mutex)   │
          └──────────────┬──────────────────────┘
                         │
                ┌────────▼────────┐
                │     docker/       │
                │   (Docker SDK)   │
                └────────┬────────┘
                         │
                ┌────────▼────────┐
                │  Docker socket   │
                └─────────────────┘
```

The auth layer sits inside `web/` as middleware — `requireRole` and `firstRunRedirect` wrap HTTP handlers. Session tokens are stored in SQLite alongside users. OIDC flow adds `/auth/oidc/login` and `/auth/oidc/callback` as public routes.

## Code Map

### `internal/models/`

Pure data types: `Schedule`, `Stack`, `Container`, `CronPreset`, `Tag`, `User`, `Session`, and `Role`. No logic, no dependencies. Every other package imports this one. The `Role` type defines `reader`, `writer`, and `admin` with an `AtLeast` method for permission checks.

Key files: `schedule.go`, `stack.go`, `user.go`

### `internal/config/`

Environment-based configuration. Reads `DB_PATH`, `DOCKER_HOST`, `WEB_PORT`, `WEB_HOST`, `LOG_LEVEL`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL` with sensible defaults. `OIDCEnabled()` returns true only when all four OIDC variables are set.

Key files: `config.go`

### `internal/auth/`

Password hashing and token generation. A leaf package — no imports from other internal packages.

Key files: `auth.go`

### `internal/store/`

SQLite persistence for schedules, stacks, tags, users, and sessions. Uses `modernc.org/sqlite` (pure Go, no CGO). `Open` runs versioned migrations on startup. CRUD operations cover schedules, stacks, tags, and user/session management. `DeleteTag` cascades to all schedules with the matching `tag_id`. A unique index on `(tag_id, container_name)` prevents duplicate schedules for the same tag+container. Sessions include expiry and are cleaned up on startup.

Key files: `store.go`

**Architecture Invariant:** The store never imports from `scheduler`, `web`, `ondemand`, or `docker`. It is a leaf dependency.

### `internal/cronpresets/`

Cron preset management backed by a YAML file. Loads presets from an embedded `presets.yaml` by default, or from a custom file specified via `PRESETS_PATH`. Create and delete operations persist to the YAML file with a mutex for concurrency safety.

Key files: `presets.go`, `presets.yaml`

### `internal/docker/`

Docker SDK wrapper. `NewClient` takes a Docker host string and returns a `*Client` with container management operations: `ListContainers`, `StartContainer`, `StopContainer`, `IsRunning`, `GetContainer`. `InspectContainer` returns health status and exposed ports. `ContainerStats` returns a `<-chan StatsSnapshot` stream for idle monitoring.

Key files: `client.go`, `stats.go`

**Architecture Invariant:** The `Client` type methods match the `scheduler.DockerActionClient` interface exactly. The `OnDemandDockerClient` interface in `ondemand` extends it with inspection and stats methods.

### `internal/scheduler/`

The cron engine. Wraps `robfig/cron/v3` with a per-container mutex to serialize concurrent start/stop actions on the same container. `AddSchedule` and `AddStack` validate cron expressions before registering them. Uses standard 5-field cron format (`min hour day month weekday`), not 6-field with seconds.

Key files: `scheduler.go`

**Architecture Invariant:** The `DockerActionClient` interface takes `context.Context` as its first parameter. Cron callbacks pass `context.Background()` — not tied to an HTTP request lifecycle.

### `internal/ondemand/`

On-demand wake and idle monitoring. `OnDemandManager` handles the wake lifecycle (start container, wait for health, redirect) and idle tracking (stream Docker stats, stop after inactivity) for containers and stacks with `OnDemandEnabled=true`. Health checks use Docker health checks, TCP port reachability, or running-state fallback.

Key files: `ondemand.go`, `idle.go`

**Architecture Invariant:** On-demand works independently of cron scheduling. A schedule with `Enabled=false, OnDemandEnabled=true` has no cron start/stop but still has a wake URL and idle monitor. The ondemand package holds its own per-container mutex map for wake serialization, separate from the scheduler's.

### `internal/yamlconfig/`

YAML import/export for schedules and tags. `FromSchedulesAndTags` serializes schedules and tags to YAML bytes, grouping tag-derived schedules under their tag. `ToSchedulesAndTags` parses YAML into schedule and tag models.

Key files: `config.go`

### `internal/web/`

HTTP server, REST API, and HTML dashboard (Go templates + HTMX). Routes serve both JSON and HTML based on the `Accept` header. Authentication is middleware-based: `firstRunRedirect` redirects to `/setup` when no users exist; `requireRole` enforces role-based access per route group. OIDC login uses PKCE with short-lived state cookies. Wake routes render a standalone page with HTMX polling for health status.

Key files: `server.go`, `api.go`, `handlers.go`

**Architecture Invariant:** The web layer depends on `SchedulerService`, `OnDemandService`, and `StackOnDemandService` interfaces, not concrete types.

### `internal/cli/`

CLI commands backed by Cobra. `serve.go` is the composition root — it wires together the store, Docker client, scheduler, on-demand manager, preset service, and web server. `schedule.go` and `tag.go` are thin wrappers over the store for offline operations.

Key files: `serve.go`, `schedule.go`, `tag.go`

**Architecture Invariant:** CLI commands outside `serve` only touch the store directly — they do not interact with the scheduler or on-demand manager. Changes take effect on next `serve` restart.

### `cmd/schedule-containers/`

Entry point. Minimal `main.go` that delegates to `internal/cli`.

## Invariants

- **Dependency direction:** `models` ← `config` ← `auth`/`store`/`cronpresets` ← `docker` ← `scheduler`/`ondemand` ← `yamlconfig` ← `web`/`cli`. No cycles. `store`, `auth`, and `cronpresets` are leaves; they never import from scheduler, ondemand, web, or docker. `scheduler` depends on Docker via the `DockerActionClient` interface, not direct import.
- **Tags are linked to schedules via `tag_id`:** A nullable `tag_id` column on the `schedules` table links each schedule to its tag. Tag-derived schedules cannot have their cron expressions edited independently — update the tag instead. Deleting a tag cascades to all its schedules.
- **Tags are persisted in SQLite** — not in YAML. Presets are in YAML; tags are user data in the DB.
- **Store is offline-only for CLI:** CLI commands write directly to SQLite. Changes made while the server is running take effect on next restart.
- **Per-container serialization:** The scheduler holds a map of `sync.Mutex` per container name. Two cron jobs targeting the same container never run concurrently. The ondemand package has its own separate mutex map for wake requests. Stacks use the same per-container locks — each container in a stack is locked individually.
- **Cron format:** Always 5-field standard (`min hour day month weekday`), not 6-field with seconds.
- **On-demand independence:** `OnDemandEnabled` works independently of `Enabled`. A schedule with `Enabled=false, OnDemandEnabled=true` has no cron start/stop but still has a wake URL and idle monitor. The `toggle` API endpoint only affects cron registration.
- **OnDemandURL required when enabled:** When `OnDemandEnabled` is true, `OnDemandURL` must be a valid URL. The API validates this on create and update.
- **Role-based access:** All management endpoints require authentication. Three roles — reader (read-only), writer (create/modify, no delete), admin (full access including user management). Wake URLs remain public. First run creates the initial admin account via `/setup`.
- **No OIDC auto-linking:** OIDC users are matched by subject claim (`oidc_subject`) or auto-provisioned as `reader`. The system never links an OIDC identity to an existing local account by username — this prevents account takeover. Admins can manually link an OIDC account to a local account via the admin panel (`POST /admin/users/{id}/link-oidc`), which transfers the `oidc_subject`, deletes the OIDC account, and invalidates both users' sessions.
- **Single binary:** Templates, static assets, and default presets are embedded via `//go:embed`. No external files needed at runtime except the SQLite database, optional presets YAML override, and Docker socket.

## Cross-Cutting Concerns

- **Error handling:** Errors are logged with `slog` and returned to the caller. The scheduler and idle monitor log and continue on container start/stop failures — no retries, since the cron job will fire again or the idle tracker will re-register on the next wake.
- **Logging:** Structured text via `log/slog`. Levels: `DEBUG` (container discovery), `INFO` (schedule fires, container wake), `WARN` (missing containers, invalid cron), `ERROR` (Docker API failures). Configured via `LOG_LEVEL` env var, default `info`.
- **Configuration:** All via environment variables with defaults. No config files. See `internal/config/config.go`.
- **Testing:** Unit tests with mocked dependencies. The `OnDemandDockerClient`, `OnDemandService`, and `StackOnDemandService` interfaces allow testing wake, idle, and stack logic with mocks. Docker client uses a transformation function (`transformContainers`) that's unit-testable without a Docker daemon. Web handlers tested with `httptest`.
- **Concurrency:** The scheduler serializes cron actions per container using `sync.Mutex`. The on-demand manager serializes wake requests per container using a separate mutex map. The idle monitor spawns a goroutine per tracked container that streams Docker Stats and checks idle thresholds on a 5-second interval. The store uses SQLite's built-in serialization for concurrent reads/writes.
- **Authentication:** Session tokens stored in SQLite, set as `HttpOnly` + `Secure` cookies. Passwords hashed with bcrypt. OIDC login uses PKCE with short-lived state cookies. Role checks are middleware — `requireRole(RoleReader)`, `requireRole(RoleWriter)`, `requireRole(RoleAdmin)`. `firstRunRedirect` middleware redirects to `/setup` when no users exist.
- **Database migrations:** Run automatically on startup in `store.Open()`. A `schema_version` table tracks the version. New columns are added via migration steps — never edit existing steps.

## A Typical Change

To add a new schedule field (e.g., `timezone`):

1. Add the field to `internal/models/schedule.go` — `Timezone string`
2. Add the column in `internal/store/store.go` — migration in `migrate()`, add to `CREATE TABLE` and all CRUD queries
3. Update `internal/web/api.go` — the JSON decoder picks up the new field automatically; add validation if needed
4. Update `internal/web/handlers.go` — add `Timezone` to `ScheduleView` and the template data
5. Update `internal/web/templates/schedules.html` — add a column to the schedules table
6. Update `internal/cli/schedule.go` — add a `--timezone` flag to the `add` command
7. Update `internal/yamlconfig/config.go` — add the field to `ScheduleEntry` and the serialization/deserialization logic
8. Add tests in `internal/store/store_test.go`