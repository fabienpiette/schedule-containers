<h3 align="center">Schedule Docker containers on cron — start, stop, or wake on demand.<br>Stacks, auth, and a dashboard. Single binary, no dependencies.</h3>

<p align="center">
  <img src="docs/demo.gif" alt="Demo" width="600">
</p>

## Quick Start

```bash
docker compose up -d
# Open http://localhost:8080 — first run creates an admin account
```

## Features

- **Cron scheduling** — Start and stop containers on any 5-field cron expression (`0 8 * * 1-5` = weekdays at 8am)
- **Stacks** — Schedule groups of containers together with shared cron expressions and on-demand wake
- **On-demand wake** — Wake stopped containers via `/wake/<container>/`, auto-redirect when healthy
- **Inactivity auto-stop** — Stop containers after configurable idle timeout (monitors CPU and network activity)
- **Authentication** — Login, sessions, and three roles (reader, writer, admin). First run prompts for admin setup. OIDC login via Pocket ID or any OpenID Connect provider
- **Tags** — Reusable schedule templates applied to multiple containers at once

## Install

**Prerequisites:** Docker (runtime), Go 1.25+ (build from source)

### Docker (recommended)

```bash
docker compose up -d
```

Push to your own registry with `make docker-release`.

### From source

```bash
git clone https://github.com/fabienpiette/schedule-containers.git
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

### On-Demand Wake

Configure a reverse proxy to redirect to the wake URL when the upstream is down:

```caddy
app.example.com {
    reverse_proxy app:8080
    handle_errors {
        @is_down expression {http.error.status_code} in [502, 503]
        handle @is_down {
            redir https://schedule-containers.example.com/wake/app/ permanent
        }
    }
}
```

When a user accesses the container's URL while it's stopped, they're redirected to the wake page. The container starts, a health check runs, and the user is redirected to the running service.

### API

```bash
curl -X POST http://localhost:8080/api/schedules \
  -H "Content-Type: application/json" \
  -d '{"container_name":"my-app","start_cron":"0 8 * * 1-5","stop_cron":"0 18 * * 1-5","enabled":true}'

curl -X POST http://localhost:8080/api/stacks \
  -H "Content-Type: application/json" \
  -d '{"name":"my-stack","start_cron":"0 8 * * 1-5","stop_cron":"0 18 * * 1-5","enabled":true}'

curl http://localhost:8080/api/containers/my-app/health
curl http://localhost:8080/api/schedules/{id}/wake-url
```

### Dashboard

Open `http://localhost:8080` — view containers, manage schedules and stacks, start/stop containers, create and apply tags, configure on-demand wake. First run redirects to a setup page to create the admin account.

For all options: `schedule-containers --help`

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PATH` | `/data/schedule-containers.db` | SQLite database path |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker socket address |
| `WEB_PORT` | `8080` | Web server port |
| `WEB_HOST` | `0.0.0.0` | Web server bind address |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `TZ` | `UTC` | Timezone for cron evaluation |
| `PRESETS_PATH` | *(empty — uses embedded)* | Custom presets YAML; if set and file doesn't exist, embedded defaults are copied to it |
| `OIDC_ISSUER` | *(empty)* | Pocket ID or OIDC provider URL (no trailing slash) |
| `OIDC_CLIENT_ID` | *(empty)* | OIDC client ID |
| `OIDC_CLIENT_SECRET` | *(empty)* | OIDC client secret |
| `OIDC_REDIRECT_URL` | *(empty)* | OIDC callback URL (e.g. `https://yourapp.com/auth/oidc/callback`) |

OIDC is disabled unless all four `OIDC_*` variables are set. When enabled, the login page shows a "Login with Pocket ID" button alongside local password auth.

## Known Issues

- **Docker socket access** — Grants full container control; consider `tecnativa/docker-socket-proxy` for restricted access
- **CLI doesn't hot-reload** — `schedule add` writes directly to SQLite; a running server picks up changes on restart
- **OIDC auto-provisioning** — New OIDC users are created with the `reader` role. Admins can promote them or link them to an existing local account via the admin panel.

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — codebase orientation for contributors

## Contributing

Contributions welcome — open an issue or pull request. See [Architecture](docs/ARCHITECTURE.md) for codebase orientation; `make build`, `make test`, and `make vet` cover the common development tasks.

## Acknowledgments

Thanks to all [contributors](https://github.com/fabienpiette/schedule-containers/graphs/contributors).

## License

[AGPL-3.0](LICENSE)