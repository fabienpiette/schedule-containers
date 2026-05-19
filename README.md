<h3 align="center">Schedule Docker container start/stop with cron expressions.<br>Web dashboard + CLI. Single binary. No dependencies.</h3>

<p align="center">
  <img src="docs/demo.gif" alt="Demo" width="600">
</p>

## Quick Start

```bash
docker compose up -d
# Open http://localhost:8080
```

## Features

- **Cron scheduling** — Start and stop containers on any 5-field cron expression (`0 8 * * 1-5` = weekdays at 8am)
- **Tags** — Reusable schedule templates applied to multiple containers at once
- **Web dashboard** — Go templates + HTMX with dark/light theme, inline actions, toast notifications
- **REST API** — Full CRUD for schedules, tags, presets; container start/stop; YAML import/export
- **Per-container locking** — Concurrent cron jobs targeting the same container are serialized
- **Single binary** — Templates, static assets, and default presets embedded via `//go:embed`

## Install

**Prerequisites:** Docker (runtime), Go 1.25+ (build from source)

### Docker (recommended)

```bash
docker compose up -d
```

Push to your own registry with `make docker-release`.

### From source

```bash
git clone https://github.com/gndm/schedule-containers.git
cd schedule-containers
make build
```

## Usage

### CLI

```bash
schedule-containers serve                                          # Start server + scheduler
schedule-containers schedule add my-app "0 8 * * 1-5" "0 18 * * 1-5"  # Add schedule
schedule-containers tag add business-hours --start "0 8 * * 1-5" --stop "0 18 * * 1-5"
schedule-containers tag apply business-hours --containers my-app,redis
schedule-containers schedule export schedules.yaml                 # Export
schedule-containers schedule import schedules.yaml --dry-run        # Import (dry-run)
```

### API

```bash
curl -X POST http://localhost:8080/api/schedules \
  -H "Content-Type: application/json" \
  -d '{"container_name":"my-app","start_cron":"0 8 * * 1-5","stop_cron":"0 18 * * 1-5","enabled":true}'

curl -X POST http://localhost:8080/api/containers/my-app/start
curl http://localhost:8080/api/export -o config.yaml
```

### Dashboard

Open `http://localhost:8080` — view containers, manage schedules, start/stop containers, create and apply tags.

For all options: `schedule-containers --help`

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PATH` | `/data/schedule-containers.db` | SQLite database path |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker socket address |
| `WEB_PORT` | `8080` | Web server port |
| `WEB_HOST` | `0.0.0.0` | Web server bind address |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `PRESETS_PATH` | *(empty — uses embedded)* | Custom presets YAML; if set and file doesn't exist, embedded defaults are copied to it |

## Known Issues

- **No authentication** — designed for private networks behind a reverse proxy (Caddy, Nginx)
- **Docker socket access** — grants full container control; consider `tecnativa/docker-socket-proxy` for restricted access
- **CLI doesn't hot-reload** — `schedule add` writes directly to SQLite; a running server picks up changes on restart

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — codebase orientation for contributors

## Contributing

Contributions welcome — open an issue or pull request. See [Architecture](docs/ARCHITECTURE.md) for codebase orientation and the [CLAUDE.md](CLAUDE.md) for development commands.

## License

MIT