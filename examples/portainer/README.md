# Deploying with Portainer

A ready-to-use stack for [Portainer](https://www.portainer.io/).

## Option A — Web editor (paste & go)

1. In Portainer: **Stacks → Add stack**.
2. Name it `schedule-containers`.
3. Choose **Web editor** and paste the contents of [`docker-compose.yml`](docker-compose.yml).
4. (Optional) Under **Environment variables**, set any of:
   `HOST_PORT`, `TZ`, `LOG_LEVEL`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`,
   `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL`.
5. **Deploy the stack**, then open `http://<host>:8080` — the first run prompts
   you to create an admin account.

## Option B — Git repository

1. **Stacks → Add stack → Repository**.
2. Repository URL: `https://github.com/fabienpiette/schedule-containers`
3. Compose path: `examples/portainer/docker-compose.yml`
4. Add environment variables as above and **Deploy**.

## Prerequisites

- **Image availability** — the stack pulls
  `ghcr.io/fabienpiette/schedule-containers:latest`. Cut a release tag so the
  image is published to GHCR and set the package to **Public**, or add GHCR
  credentials under Portainer's **Registries** for a private pull. Pin a
  specific version (e.g. `:1.0.0`) instead of `latest` for reproducible deploys.
- **Docker socket** — the host must expose `/var/run/docker.sock`. This grants
  full control of the host's containers; see [`SECURITY.md`](../../SECURITY.md)
  for hardening with a socket proxy.

## Data & updates

- Schedules, tags, stacks, and users persist in the named volume
  `schedule-containers-data` (`/data`). It survives stack re-deploys.
- To update: re-pull the image and redeploy the stack (Portainer:
  **Update the stack → Re-pull image and redeploy**).
