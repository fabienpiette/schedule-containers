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

- **Cron scheduling** — Start and stop containers on any cron expression (`0 8 * * 1-5` = weekdays at 8am)
- **Tags** — Define schedule templates, apply to multiple containers at once
- **Web dashboard** — Go templates + HTMX, single binary, no JS build step
- **REST API** — Full CRUD for schedules, tags, presets, import/export, container start/stop
- **YAML import/export** — Backup and restore schedules as YAML
- **Per-container locking** — Prevents race conditions when concurrent cron jobs target the same container

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
# Start the server (web dashboard + scheduler)
schedule-containers serve

# Add a schedule (start weekdays 8am, stop 6pm)
schedule-containers schedule add my-app "0 8 * * 1-5" "0 18 * * 1-5"

# Tags — reusable schedule templates
schedule-containers tag add business-hours --start "0 8 * * 1-5" --stop "0 18 * * 1-5"
schedule-containers tag apply business-hours --containers my-app,redis

# Export and import schedules as YAML
schedule-containers schedule export schedules.yaml
schedule-containers schedule import schedules.yaml --dry-run
```

### API

```bash
# Create a schedule
curl -X POST http://localhost:8080/api/schedules \
  -H "Content-Type: application/json" \
  -d '{"container_name":"my-app","start_cron":"0 8 * * 1-5","stop_cron":"0 18 * * 1-5","enabled":true}'

# Start a container now
curl -X POST http://localhost:8080/api/containers/my-app/start

# Export schedules and tags as YAML
curl http://localhost:8080/api/export -o config.yaml
```

### Dashboard

Open `http://localhost:8080` — view containers, manage schedules, start/stop containers, create and apply tags, export/import YAML.

For all options: `schedule-containers --help`

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PATH` | `/data/schedule-containers.db` | SQLite database path |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker socket address |
| `WEB_PORT` | `8080` | Web server port |
| `WEB_HOST` | `0.0.0.0` | Web server bind address |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `PRESETS_PATH` | *(empty — uses embedded)* | Path to custom presets YAML; if set and file doesn't exist, embedded defaults are copied to it |

## Known Issues

- No authentication — designed for private networks behind a reverse proxy (Caddy, Nginx)
- Docker socket access grants full container control; consider `tecnativa/docker-socket-proxy` for restricted access
- CLI `schedule add` writes to the database directly; a running server picks up changes on restart

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — codebase orientation for contributors

## Contributing

Contributions welcome — open an issue or pull request. See [Architecture](docs/ARCHITECTURE.md) for codebase orientation.

## License

[MIT](LICENSE)