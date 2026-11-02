# Docker

The containers of your Docker Engine (Docker Desktop, Colima, OrbStack, Rancher Desktop or a plain daemon), grouped by Compose project: start, stop, restart and remove them, run Compose up, down and pull, and open their logs or a shell in a terminal tab. This was Flint's built-in Docker panel; it now installs separately.

| | |
|---|---|
| Id | `flint.docker` |
| Version | 0.1.0 |
| Contract | 1, with terminal tabs (Flint with `window/openTerminal`) |
| Permissions | `terminal` |
| Starts | when its panel is shown or its command runs |

## What it adds

- **Panel "Docker"**: a tree of Compose projects and the containers outside any project (running ones first). A project shows how many of its containers run ("1/2 running", or "not started" for a compose file in your project folders that has no containers yet); expand it for its containers, by service. Each container shows its status, image and published ports; hover for its id, Compose service and port mappings. While the panel is on screen it refreshes every 3 seconds, and after every action.
  - **Container menu**: Logs, Open Shell, Start or Stop, Restart, Remove… (asks first; only for stopped containers), and Open localhost:&lt;port&gt; for up to three published ports.
  - **Project menu**: Up (`docker compose up -d`), Restart, Stop, Down… (asks first), Pull Images, and Logs (`docker compose logs -f`).
  - When the engine can't be reached the panel says so, with a **Retry** button. An engine reached over plain `tcp://` without TLS is flagged at the top.
- **Terminal tabs**: Logs runs `docker logs -f --tail <N> <container>` and Open Shell runs `docker exec -it <container> sh -c "command -v bash >/dev/null && exec bash || exec sh"` (bash when the container has it, else sh) in a normal terminal tab, so you see the command and can stop it.
- **Status bar item**: the number of running containers, for example "Docker: 2 running". Click it to refresh.
- **Command**: **Docker: Refresh Containers**.
- Results and errors of actions are shown as toasts.

The panel itself talks to the Docker Engine API and needs no docker CLI. **Logs, Open Shell and Compose up, down and pull need the docker CLI** (and its Compose plugin): install Docker Desktop, or `docker-ce-cli` and `docker-compose-plugin`. Without it, the panel says so and hides Up, Down and Pull; Compose Restart and Stop then act on the project's containers through the API.

## Settings

In **Settings ▸ Docker**, stored in `settings.json` under `extensions.flint.docker`:

| Setting | Default | Meaning |
|---|---|---|
| `host` | `""` | Engine address: `unix:///path/docker.sock`, `tcp://host:2376` or `https://host:2376`. Empty finds the engine like the docker CLI: the current docker context (`~/.docker/config.json`, with its TLS material), then the default sockets of Docker Desktop, Colima, OrbStack and Rancher Desktop |
| `logTailLines` | `500` | Past lines a log tab starts with (1–10,000) |

When `host` is set, log and shell tabs and Compose get `DOCKER_HOST` (and `DOCKER_TLS_VERIFY=1` for `https://`), so the docker CLI reaches the same engine.

The manifest asks Flint for `DOCKER_HOST`, `DOCKER_CONTEXT`, `DOCKER_CONFIG`, `DOCKER_TLS_VERIFY` and `DOCKER_CERT_PATH` from your environment (`run.env`), so the panel finds the engine like the docker CLI in your shell does. The `host` setting overrides them.

## Privacy

It talks only to the Docker Engine: the local socket, or the address you set in `host`. It looks for compose files (`compose.yaml`, `docker-compose.yml`, …) in your trusted project folders and their direct subfolders, by name only, and reads `~/.docker/config.json` and the current context's metadata and certificates to find the engine. The `terminal` permission opens the log and shell tabs; nothing runs in them that you don't see. It sends nothing else over the network and stores nothing outside Flint's settings.

## Build and install

Needs Go 1.27 or newer; only the standard library is used.

```bash
go build -o docker-ext .
```

In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The manifest: command, panel, status item, settings schema, permissions |
| `main.go` | Requests and notifications from Flint, polling while the panel is shown, the engine client |
| `view.go` | The panel's tree and the status item |
| `actions.go` | Container and Compose actions, confirmations, terminal tabs |
| `settings.go` | The settings and their checks |
| `rpc.go` | JSON-RPC over stdin and stdout |
| `internal/engine/` | The Docker Engine API client (unix socket or TLS, docker contexts, Compose through the CLI) |
