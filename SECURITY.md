# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities **privately** — do not open a public issue.

Use GitHub's [private vulnerability reporting](https://github.com/fabienpiette/schedule-containers/security/advisories/new)
("**Report a vulnerability**" on the repository's **Security** tab). This opens a
private channel between you and the maintainers.

Please include:

- A description of the vulnerability and its impact
- Steps to reproduce (a proof of concept if possible)
- The affected version or commit

**What to expect:**

- Acknowledgement within 5 business days
- An assessment and, if accepted, a fix timeline
- Credit in the published advisory once a fix is released (unless you prefer to remain anonymous)

## Supported Versions

This project is pre-1.0. Security fixes target the most recent release and the
`main` branch. Older tags are not maintained.

| Version                          | Supported |
| -------------------------------- | --------- |
| latest release / `main`          | ✅        |
| older                            | ❌        |

## Scope

`schedule-containers` talks to the Docker Engine through its API socket and
serves a web dashboard. When assessing risk, note:

- **Docker socket access is privileged.** Granting access to the Docker socket
  gives full control over the host's containers. Run behind a socket proxy
  (e.g. [`tecnativa/docker-socket-proxy`](https://github.com/Tecnativa/docker-socket-proxy))
  to limit what this service can do.
- **Docker Engine vulnerabilities are out of scope here.** This application is a
  *client* of the Docker daemon; vulnerabilities in the daemon itself are
  remediated by updating the host's Docker Engine, not by this project.

In scope: authentication and session handling, the OIDC login flow, the web
UI/API, the on-demand wake endpoints, and this project's own dependencies.
