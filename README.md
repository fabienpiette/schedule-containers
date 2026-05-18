<h3 align="center">Schedule Docker container start/stop with cron expressions.<br>Web dashboard + CLI. Single binary. No dependencies.</h3>

---

<p align="center">
  <img src="docs/demo.gif" alt="Demo" width="600">
</p>

## Quick Start

```bash
# Clone and run
git clone https://github.com/gndm/schedule-containers.git
cd schedule-containers
make run
# Open http://localhost:8080
```

Or with Docker:

```bash
docker compose up -d
# Open http://localhost:8080
```

## Features

- **Cron scheduling** — Start and stop containers on any cron expression (`0 8 * * 1-5` = weekdays at 8am)
- **Cron preset selectors** — Default presets from embedded YAML, plus create/delete custom presets via API or web UI
- **YAML import/export** — Export schedules as YAML, import from file or API
- **Web dashboard** — Go templates + HTMX, single binary, no JS build step
- **REST API** — Full CRUD for schedules, presets, import/export, container start/stop
- **CLI** — `schedule-containers schedule add my-app "0 8 * * *" "0 18 * * *"`
- **Portainer-aware** — Detects compose stacks from `com.docker.compose.project` labels
- **Per-container locking** — Prevents race conditions when concurrent cron jobs target the same container

## Install

**Prerequisites:** Go 1.25+ (build), Docker (runtime)

### From source

```bash
git clone https://github.com/gndm/schedule-containers.git
cd schedule-containers
make build
```

### Docker (recommended)

```bash
docker compose up -d
```

Push to your own registry with `make docker-release`.

## Usage

### CLI

```bash
# Start the server (web dashboard + scheduler)
schedule-containers serve

# List discovered containers
schedule-containers containers list

# Add a schedule (start weekdays 8am, stop 6pm)
schedule-containers schedule add my-app "0 8 * * 1-5" "0 18 * * 1-5"

# List schedules
schedule-containers schedule list

# Remove a schedule
schedule-containers schedule remove <id>

# Export schedules to YAML
schedule-containers schedule export schedules.yaml

# Import schedules from YAML (dry-run)
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

# Toggle a schedule on/off
curl -X POST http://localhost:8080/api/schedules/<id>/toggle

# List cron presets
curl http://localhost:8080/api/presets

# Create a custom preset
curl -X POST http://localhost:8080/api/presets \
  -H "Content-Type: application/json" \
  -d '{"label":"Late start","expression":"0 10 * * 1-5","category":"Custom"}'

# Export schedules as YAML
curl http://localhost:8080/api/export -o schedules.yaml

# Import schedules from YAML
curl -X POST http://localhost:8080/api/import \
  -H "Content-Type: application/yaml" \
  --data-binary @schedules.yaml
```

### Dashboard

Open `http://localhost:8080` for the web interface — view containers, manage schedules, start/stop containers with one click. Use the preset dropdowns to quickly set cron expressions, manage custom presets at `/presets`, and export/import schedules as YAML.

For all options: `schedule-containers --help`

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PATH` | `/data/schedule-containers.db` | SQLite database path |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker socket address |
| `WEB_PORT` | `8080` | Web server port |
| `WEB_HOST` | `0.0.0.0` | Web server bind address |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `PRESETS_PATH` | *(empty — uses embedded)* | Path to custom presets YAML file; if set and file doesn't exist, embedded defaults are copied to it |

## Known Issues

- No authentication — designed for private networks behind a reverse proxy (Caddy, Nginx)
- Docker socket access grants full container control; consider `tecnativa/docker-socket-proxy` for restricted access
- CLI `schedule add` only writes to the database; a running server picks up changes on restart

## Documentation

- [Architecture & design spec](docs/superpowers/specs/2025-05-13-schedule-containers-design.md)
- [Implementation plan](docs/superpowers/plans/2025-05-13-schedule-containers.md)

## License

[MIT](LICENSE)