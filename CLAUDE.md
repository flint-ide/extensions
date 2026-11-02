# Flint extensions: rules for agents

This repository holds Flint's level-2 extensions: separate programs that talk to Flint over JSON-RPC 2.0 on stdin/stdout. The contract is defined in the Flint repository, not here; read it before changing a manifest or a message:

- Design: `docs/design/extensions.md` in [flint-ide/flint](https://github.com/flint-ide/flint) (Level 2, Webviews, Separate packages)
- Protocol reference: `site/extensions/protocol.md` (every method, limit and error code)
- Writing one: `site/extensions/write-an-extension.md`

If an extension needs something the contract doesn't offer, the change goes to Flint first (manifest, host, docs and a conformance test there), then here. Never rely on behavior the protocol reference doesn't document.

## Layout

- One folder per extension, named after it (`stats/`, `notes/`, later `database/`, `kubernetes/`, `docker/`), with `flint-extension.json` at its root.
- Every extension has a `README.md` in the format of `stats/README.md`: the id/version/contract/permissions/starts table, What it adds, Settings, Privacy (what it reads, network, storage), Build and install, Files. Keep it in step with the manifest.
- Add a row to the table in the root `README.md` for a new extension.

## Manifest

- `id` is `publisher.name`; every contributed id starts with `<id>.`.
- Declare only the permissions the code uses, and say in the README what it does with each.
- `run.command` is a clean relative path inside the folder; the built binary is git-ignored (`.gitignore` in the folder).
- Raise `version` with every user-visible change; raise `contract` only when Flint's contract major changes.

## Code

- Go extensions are their own module (`go.mod` in the folder, `module github.com/flint-ide/extensions/<name>`), standard library only unless a dependency is unavoidable and discussed first.
- stdout carries protocol messages only; logs go to stderr.
- Bound everything the extension sends (Flint's limits: 4 MiB per message, 5,000 items, label lengths): answers past a limit are cut or refused by Flint.
- Secrets only through `secrets/*` (the system keyring), never in settings, files or logs. No telemetry, and no network access the user didn't ask for.
- Webview pages are **one self-contained HTML file** each: inline every script, style, font and image (the frame loads nothing and has no network). Use Flint's theme variables (`--bg-editor`, `--fg`, `--accent`, `--font-ui`, …) instead of fixed colors, and talk to the process only through `window.flint`.

## Checks before a pull request

Run in the extension's folder (CI runs the same for every module):

```bash
gofmt -l . && go vet ./... && go build -o /dev/null . && go test ./...
```

Then check it against the contract with Flint's own checker, the `contract` job of CI. The checker uses Flint's manifest and host code: it validates the manifest and the webview files, starts the program, resolves every declarative panel, sends each webview a request, lists the agent tools and checks for a clean shutdown. Build the program first, under the name in `run.command`:

```bash
go build -o <run.command> .
git clone https://github.com/flint-ide/flint ../flint   # once
(cd ../flint && go run ./cmd/flint-extcheck "$OLDPWD")
```

CI also runs it every day against Flint's `main`, so a contract change in Flint that breaks an extension shows up here. An extension that needs an unreleased contract fails this job until that Flint change is merged; say so in the pull request.

Add tests for every request the extension answers (see `notes/main_test.go`: requests, notifications sent, calls to Flint answered by the test). Test concurrency with `go test -race`.

## Git

- Work on a branch and open a pull request; never push to `main`.
- Conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`, `ci:`; scope with the extension name when it helps (`fix(stats): …`).
- An extension that needs an unreleased Flint says so in its README (the Contract row) and in the pull request, with the Flint pull request it depends on.
