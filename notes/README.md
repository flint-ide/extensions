# Notes

A small notes extension whose UI is its own HTML: a panel listing the notes and a tab per note. It is the example of **webviews**, Flint's way for an extension to draw its own UI, so its code is meant to be read. Notes live in the extension's memory and are gone when it stops; it shows the mechanics, not a notes app.

| | |
|---|---|
| Id | `example.notes` |
| Version | 0.1.0 |
| Contract | 1, with webviews (Flint with [flint-ide/flint#57](https://github.com/flint-ide/flint/pull/57) or later) |
| Permissions | none |
| Starts | when its panel or one of its tabs is shown, or its command runs |

## What it adds

- **Panel "Notes"** (webview, `ui/list.html`): the notes; click one to open it, **+ New** adds one. It reloads when the extension says a note changed (`webview/post`).
- **Tab "Note"** (webview, `ui/note.html`): a title and text. **⌘S** saves (the page keeps the shortcut; other shortcuts still reach Flint). The tab shows the unsaved dot while edited and takes the note's title when saved. Open tabs come back after a restart (`"restore": true`); their notes don't, as they live in memory.
- **Command Notes: New Note**: adds a note and opens its tab (`window/openTab`).

## How the pages talk to the extension

Flint loads each page into a sandboxed frame without network and defines `window.flint` before the page's scripts:

- `flint.request(method, params)` reaches this program as `webview/request { view, key, method, params }` (`list`, `new`, `open`, `get`, `save` in `main.go`); the answer resolves the promise.
- `flint.onMessage(fn)` receives what the program sends with `webview/post`.
- `flint.key` is the tab's key, here the note's id; `flint.setTitle`, `flint.setDirty` and `flint.showMessage` update the tab and show toasts.
- Flint's theme arrives as CSS variables on `:root` (`--fg`, `--bg-editor`, `--accent`, `--font-ui`, …), so the pages follow light, dark and custom themes.

Each page is one self-contained file: everything is inline, as the frame can't load anything. The full list of bridge calls and the policy are in the [protocol reference](https://github.com/flint-ide/flint/blob/main/site/extensions/protocol.md#webviews).

## Privacy

It reads nothing from your projects, sends nothing over the network and stores nothing on disk.

## Build and install

Needs Go 1.27 or newer; only the standard library is used.

```bash
go build -o notes .
```

In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The manifest: the command, the webview panel and the tab view |
| `main.go` | Requests from Flint and from the pages (`webview/request`), tabs and messages back |
| `rpc.go` | JSON-RPC over stdin and stdout |
| `ui/list.html`, `ui/note.html` | The pages, each self-contained |
| `main_test.go` | The requests, saving and opening a tab |
