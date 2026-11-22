# Flint extensions

Extensions for [Flint](https://github.com/flint-ide/flint). An extension is a separate program, in any language, that talks to Flint over JSON-RPC 2.0 on stdin and stdout. It can add commands, sidebar panels, a settings page, status bar items and read-only tools for agents. Flint draws its UI from data, or, for a **webview**, shows the extension's own HTML in a sandboxed frame that has no network and no access to Flint beyond a small bridge.

How extensions work, the manifest and every protocol message are documented in the Flint repository:

- [Extensions](https://github.com/flint-ide/flint/blob/main/site/guides/extensions.md): installing, permissions and Settings ▸ Extensions
- [Write an extension](https://github.com/flint-ide/flint/blob/main/site/extensions/write-an-extension.md)
- [Protocol reference](https://github.com/flint-ide/flint/blob/main/site/extensions/protocol.md)

## Extensions here

| Folder | Extension | What it does |
|---|---|---|
| [`stats`](stats) | Project Stats | Counts the files of the open projects by type: a panel, a status bar item and an agent tool. Also the example in Flint's docs. |
| [`docker`](docker) | Docker | Containers of the Docker Engine grouped by Compose project: start, stop, restart, remove, compose up/down/pull, logs and shells in terminal tabs. Replaces Flint's built-in Docker panel. |
| [`kubernetes`](kubernetes) | Kubernetes | Clusters through kubectl in its own webview: apps with health, workloads, pods, services, jobs, rollouts, scaling, logs and shells, diagnosis for agents and the `kubernetes_status` agent tool. Replaces Flint's built-in Kubernetes panel. |
| [`aws`](aws) | AWS Profiles | The AWS CLI profiles with their credentials: log in (SSO, saml2aws), refresh, check identity, terminals as a profile. Replaces Flint's built-in AWS Profiles panel. |
| [`ruby`](ruby) | Ruby | Ruby files: highlighting, comments, completion, go to definition and diagnostics through Ruby LSP or Solargraph, and Run Tests in File with RSpec. A language is data: no program. |
| [`python`](python) | Python | Debug File with debugpy and Run Tests in File with pytest for Python, which Flint already highlights and serves itself. Data only: no program. |
| [`rust`](rust) | Rust | Run Tests with `cargo test` (the whole package) for Rust, which Flint already serves itself. Data only: no program. |
| [`cpp`](cpp) | C and C++ | C and C++ files: highlighting, comments, and completion, go to definition and diagnostics through clangd. Data only: no program. |
| [`java`](java) | Java | Java files: highlighting, comments, and completion, go to definition and diagnostics through Eclipse JDT LS (`jdtls`). Data only: no program. |
| [`csharp`](csharp) | C# | C# files: highlighting, comments, and completion, go to definition and diagnostics through csharp-ls or OmniSharp. Data only: no program. |
| [`php`](php) | PHP | PHP files: highlighting, comments, a language server (Intelephense or Phpactor) and Run Tests in File with PHPUnit. Data only: no program. |
| [`kotlin`](kotlin) | Kotlin | Kotlin files: highlighting, comments, and completion, go to definition and diagnostics through kotlin-language-server or kotlin-lsp. Data only: no program. |
| [`lua`](lua) | Lua | Lua files: highlighting, comments, lua-language-server and Run Tests in File with busted (results for each test, from TAP). Data only: no program. |
| [`elixir`](elixir) | Elixir | Elixir files: highlighting, comments, a language server (ElixirLS, Expert or Next LS), Debug File with ElixirLS's debug adapter and Run Tests in File with `mix test`. Data only: no program. |
| [`flint-themes`](flint-themes) | Flint Themes | Flint's own color themes: Graphite (dark stone gray with a spark of orange) and Paper (warm light). |
| [`catppuccin`](catppuccin) | Catppuccin | Color themes: Latte, Frappé, Macchiato, Mocha. |
| [`tokyo-night`](tokyo-night) | Tokyo Night | Color themes: Night, Storm, Day. |
| [`gruvbox`](gruvbox) | Gruvbox | Color themes: Dark, Light. |
| [`nord`](nord) | Nord | Color theme: Nord. |

## Install one

**From the extension index (prebuilt).** In Flint, open **Settings ▸ Extensions ▸ Extension index** and click **Browse Extensions**, then **Install…** on an extension. Flint downloads the signed index this repository publishes with each [release](../../releases), checks its signature, downloads the archive for your platform, checks it against the index and shows the extension's program and permissions before installing. **Check for Updates** there offers **Update to x.y.z…** when a newer version was released. Flint contacts the index only when you click.

This needs a Flint with the extension index ([format](https://github.com/flint-ide/flint/blob/main/site/extensions/index-format.md)) and the index's public key built in. The key pair is not generated yet: until a maintainer sets the `FLINT_EXTINDEX_KEY` secret here (see [Releases](#releases)) and adds the public key to Flint, no index is published and Flint says the index is not configured. Use one of the other two ways meanwhile.

**From this repository.** **Settings ▸ Extensions ▸ Repository** lists the extensions here and builds a Go extension on your machine when you click **Install…** (needs `git` and Go).

**From a folder.** Each extension is a folder with a `flint-extension.json` manifest and the program it names. To install `stats`:

```bash
cd stats && go build -o stats .
```

Then in Flint run **Install Extension from Folder…** from the command palette and pick the `stats` folder, or run `flint --install-extension stats` in a terminal. Flint shows the extension's program and permissions before copying it to its config folder. Manage it in **Settings ▸ Extensions**.

## Add one

- One folder per extension, named after it, with `flint-extension.json` at its root.
- A **color theme** is an extension without a program: just the manifest with `contributes.themes` and a README (see Themes in [`CLAUDE.md`](CLAUDE.md)). Users pick it in **Settings ▸ Appearance ▸ Color theme**.
- Every extension has a `README.md`: what it adds (panels, commands, status items, agent tools), its settings, its permissions and what it does with them (what it reads, whether it uses the network or stores anything), and how to build and install it. [`stats/README.md`](stats/README.md) is the model.
- Go extensions are their own module (`go.mod` in the folder) and use only the standard library, like Flint itself.
- Declare only the permissions the extension uses, and describe in the manifest what it does with them.
- No telemetry, no network access the user doesn't ask for, and secrets only through Flint's `secrets/*` methods (the system keyring), never in settings or files.
- CI checks that every extension has a manifest with the required fields and a README, vets, builds and tests every Go module, and runs **Flint's own contract checker** (`cmd/flint-extcheck` in the Flint repository) on every extension: manifest and webview rules, a start of the program, every panel resolved, a clean shutdown. It also runs daily against Flint's `main`. Add a line to the table above.

## Releases

The [Release workflow](.github/workflows/release.yml) publishes what Flint's **Extension index** installs from. It runs when a tag `v*` is pushed, or by hand (**Actions ▸ Release ▸ Run workflow**, with the tag to publish). It:

1. builds every Go extension for `darwin-arm64`, `darwin-amd64`, `linux-amd64` and `linux-arm64` (`CGO_ENABLED=0`, the program named as `run.command` says);
2. packs each extension as a `.tar.gz`, one per platform for a program and one (`any`) for themes and languages;
3. writes `index.json` and signs it (`index.json.sig`) with `cmd/flint-extindex` from a Flint checkout, the same code Flint verifies with;
4. verifies the signature and every archive, then uploads the archives, `index.json` and `index.json.sig` as the release's assets. Flint reads `releases/latest/download/index.json`.

Setup, once, by a maintainer:

- In a Flint checkout run `go run ./cmd/flint-extindex keygen`. Store the **secret key** as this repository's Actions secret **`FLINT_EXTINDEX_KEY`** and nowhere else; put the **public key** into Flint (`OfficialPublicKey` in `internal/extensions/index/sign.go`).
- Optionally set the repository variable `FLINT_EXTINDEX_PUBLIC_KEY` to that public key: the workflow then checks the signature against the key Flint has before uploading.

Without the secret the workflow fails in its first step, with a message that says so, and publishes nothing: an unsigned index is never uploaded. The workflow needs a Flint that has `cmd/flint-extindex` (the extension index change in flint-ide/flint); until that is merged, run it with the **flint-ref** input set to that branch.

To release an extension's update, raise `version` in its manifest and merge; the next release carries it.

## License

MIT, like Flint.
