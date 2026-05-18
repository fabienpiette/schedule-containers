# Architecture

This document describes the high-level architecture of `schedule-containers`.
If you want to familiarize yourself with the codebase, you are in the right place.

## Bird's Eye View

Schedule Containers is a single Go binary that keeps Docker containers running on a schedule. You define cron expressions for when a container should start and stop, and the scheduler ensures those actions happen at the right time. It also provides a web dashboard and REST API for managing schedules and manually controlling containers.

The runtime flow is straightforward: on startup, the `serve` command loads persisted schedules from SQLite, registers them with an in-process cron runner, and starts an HTTP server. When a cron job fires, the scheduler calls the Docker API to start or stop the target container. The web dashboard and REST API let you create, toggle, and delete schedules which dynamically add or remove cron jobs. Portainer stack names are detected from Docker labels on containers.

```
                    ┌─────────────────────┐
                    │   CLI (cobra)        │
                    │  serve / schedule /   │
                    │  containers           │
                    └──────────┬───────────┘
                               │
                    ┌──────────▼───────────┐
                    │   internal/cli/       │
                    │   serve.go            │──── wires everything together
                    └──────┬───────┬───────┘
                           │       │
              ┌────────────▼┐  ┌───▼────────────┐
              │  store/      │  │  web/           │
              │  (SQLite)    │  │  (HTTP + HTML)  │
              └──────┬──────┘  └───┬─────────────┘
                     │             │
              ┌──────▼─────────────▼──────────┐
              │         scheduler/             │
              │  (cron runner, per-container    │
              │   mutex, Docker calls)          │
              └──────────────┬─────────────────┘
                             │
                    ┌────────▼────────┐
                    │   docker/       │
                    │  (Docker SDK)   │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Docker socket   │
                    └─────────────────┘
```

## Code Map

### `cmd/schedule-containers/`

Entry point. Minimal `main.go` that delegates to `internal/cli`. Nothing interesting here.

### `internal/cli/`

CLI commands backed by Cobra. `serve.go` is the most important file — it wires together the store, Docker client, scheduler, and web server. `schedule.go` and `containers.go` are thin wrappers over the store and Docker client for offline operations.

Key files: `serve.go`, `schedule.go`

**Architecture Invariant:** CLI commands outside `serve` only touch the store directly — they do not interact with the scheduler. Schedules added via CLI become active on next `serve` restart.

### `internal/models/`

Pure data types: `Schedule`, `Container`, and `CronPreset`. No logic, no dependencies. Every other package imports this one.

Key files: `schedule.go`, `container.go`, `preset.go`

### `internal/config/`

Environment-based configuration. Reads `DB_PATH`, `DOCKER_HOST`, `WEB_PORT`, `WEB_HOST`, `LOG_LEVEL` from environment variables with sensible defaults.

Key files: `config.go`

### `internal/store/`

SQLite persistence for schedules and custom cron presets. Uses `modernc.org/sqlite` (pure Go, no CGO). The `Open` function runs schema migrations on startup, including a `schema_version` table for future migrations. CRUD operations are straightforward — create, get, list, update, delete, toggle for schedules; create, list, get, delete for custom presets.

Key files: `store.go`

**Architecture Invariant:** The store never imports from `scheduler`, `web`, or `docker`. It is a leaf dependency.

### `internal/cronpresets/`

Built-in cron preset definitions. `Builtins()` returns a `[]models.CronPreset` with common schedules organized by category (Daily, Weekdays, Weekends, Specific Days, Frequent, Monthly). Presets have a `Builtin: true` flag that prevents deletion via the API.

Key files: `presets.go`

### `internal/yamlconfig/`

YAML import/export for schedules. `FromSchedules` serializes schedules to YAML bytes. `ToSchedules` parses and validates YAML into schedule models (validates cron expressions via `scheduler.ValidateCronExpression`). `ImportFromFile` reads a file and delegates to `ToSchedules`.

Key files: `config.go`

### `internal/docker/`

Thin wrapper around the Docker SDK. `NewClient` takes a Docker host string and returns a `*Client` with four operations: `ListContainers`, `StartContainer`, `StopContainer`, `IsRunning`. The `transformContainers` helper extracts container names and stack labels from the Docker API response.

Key files: `client.go`

**Architecture Invariant:** The `Client` type methods match the `scheduler.DockerActionClient` interface signature exactly — no adapter layer needed.

### `internal/scheduler/`

The cron engine. Wraps `robfig/cron/v3` with a per-container mutex to serialize concurrent start/stop actions on the same container. `AddSchedule` validates cron expressions before registering them. `RemoveSchedule` is idempotent. Uses standard 5-field cron format (min hour day month weekday), not the 6-field format with seconds.

Key files: `scheduler.go`

**Architecture Invariant:** The `DockerActionClient` interface takes `context.Context` as its first parameter. The cron callbacks pass `context.Background()` — this is intentional since cron jobs are not tied to an HTTP request lifecycle.

### `internal/web/`

HTTP server, REST API handlers, and Go templates with HTMX. Chi router for routing, `embed.FS` for templates and static assets baked into the binary. The `SchedulerService` interface decouples the web layer from the concrete `scheduler.Scheduler` type.

API routes include schedule CRUD, container start/stop, cron presets (list builtins + custom, create/delete custom), and YAML import/export. HTML pages include a dashboard, containers view, schedule creation form (with preset dropdowns), and a presets management page for CRUD on custom presets.

Key files: `server.go` (routing, setup), `api.go` (JSON endpoints), `handlers.go` (HTML rendering), `templates/`, `static/`

**Architecture Invariant:** The web layer depends on `scheduler.SchedulerService` (interface), not `scheduler.Scheduler` (concrete). This allows testing with a mock scheduler.

## Invariants

- **Dependency direction:** `models` ← `config` ← `store`/`cronpresets` ← `docker` ← `scheduler` ← `yamlconfig` ← `web`/`cli`. No cycles. `store` and `cronpresets` are leaves; they never import from scheduler, web, or docker.
- **Store is offline-only for CLI:** The `schedule add` CLI command writes to SQLite directly. The running server reads schedules from SQLite on startup. Changes made while the server is running (via API) immediately update the cron runner. CLI-only changes take effect on next server restart.
- **Per-container serialization:** The scheduler holds a map of `sync.Mutex` per container name. Two cron jobs targeting the same container will never run concurrently — they wait for the mutex.
- **Cron format:** Always 5-field standard (`min hour day month weekday`), not 6-field with seconds. `ValidateCronExpression` uses `cron.ParseStandard`.
- **No authentication:** V1 has no auth. The app runs on a private network behind Caddy. Auth is expected to be handled by the reverse proxy.
- **Single binary:** Templates and static assets are embedded via `//go:embed`. No external files needed at runtime except the SQLite database and Docker socket.

## Cross-Cutting Concerns

- **Error handling:** Errors are logged with `slog` and returned to the caller. The scheduler logs and continues on container start/stop failures — no retries, since the cron job will fire again.
- **Logging:** Structured JSON via `log/slog`. Levels: `DEBUG` (container discovery), `INFO` (schedule fires), `WARN` (missing containers, invalid cron), `ERROR` (Docker API failures). Configured via `LOG_LEVEL` env var.
- **Configuration:** All via environment variables with defaults. No config files. See `internal/config/config.go`.
- **Testing:** Unit tests with mocked dependencies for store and scheduler. Docker client uses a transformation function (`transformContainers`) that's unit-testable without a Docker daemon. Web handlers tested with `httptest`.
- **Database migrations:** Run automatically on startup in `store.Open()`. A `schema_version` table tracks the version. Phase 2 fields will be added via migration.

## A Typical Change

To add a new schedule field (e.g., `timezone`):

1. Add the field to `internal/models/schedule.go` — `Timezone string`
2. Add the column in `internal/store/store.go` — migration in `migrate()`, add to `CREATE TABLE` and all CRUD queries
3. Update `internal/web/api.go` — the JSON decoder will pick up the new field automatically
4. Update `internal/web/handlers.go` — add `Timezone` to `ScheduleView` and the template data
5. Update `internal/web/templates/dashboard.html` — add a column to the schedules table
6. Update `internal/cli/schedule.go` — add a `--timezone` flag to the `add` command
7. Update `internal/yamlconfig/config.go` — add the field to `ScheduleEntry` and the serialization/deserialization logic
8. Add tests in `internal/store/store_test.go`