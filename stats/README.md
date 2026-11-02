# Project Stats

Counts the files of your open projects by type, with the largest files of each type. It is also the example extension in Flint's docs ([Write an extension](https://github.com/flint-ide/flint/blob/main/site/extensions/write-an-extension.md)), so its code is meant to be read.

| | |
|---|---|
| Id | `example.stats` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | `workspace.read` |
| Starts | when its panel is shown, one of its commands runs, or an agent calls its tool |

## What it adds

- **Panel "Project Stats"**: a tree of file types (by count or by size), each with its largest files. Click a file, or choose **Open** in its context menu, to open it.
- **Status bar item**: the number of files counted, for example "4 files". Click it to count again.
- **Commands**:
  - **Project Stats: Count Files Again** (`⌥⇧⌘S`; Flint drops the shortcut if you already use it)
  - **Project Stats: Open Largest File**: the largest file of the most common type
- **Agent tool `stats_summary`**: files and bytes per type with the largest files, so an agent can get a feel for an unfamiliar repository. It only reads. Turn it off in **Settings ▸ Extensions**.

## Settings

In **Settings ▸ Project Stats**, stored in `settings.json` under `extensions.example.stats`:

| Setting | Default | Meaning |
|---|---|---|
| `exclude` | `node_modules`, `vendor`, `dist`, `build`, `target` | Folder names never counted |
| `topFiles` | `10` | Largest files listed per type (1–50) |
| `countHidden` | `false` | Also count files and folders whose names start with a dot |
| `sortBy` | `count` | `count`: most files first; `size`: most bytes first |

## Privacy

It reads file names and sizes in your trusted project folders, never their contents. It sends nothing over the network and stores nothing outside Flint's settings.

## Build and install

Needs Go 1.27 or newer; only the standard library is used.

```bash
go build -o stats .
```

In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The manifest: commands, panel, status item, settings schema, agent tool, permissions |
| `main.go` | Requests and notifications from Flint |
| `rpc.go` | JSON-RPC over stdin and stdout |
| `scan.go` | Walking the folders and counting |
