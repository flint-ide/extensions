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
| [`notes`](notes) | Notes | Notes in a panel and tabs drawn by the extension's own HTML: the example of webviews (panels and tab views, the `window.flint` bridge, session restore). |

## Install one

Each extension is a folder with a `flint-extension.json` manifest and the program it names. To install `stats`:

```bash
cd stats && go build -o stats .
```

Then in Flint run **Install Extension from Folder…** from the command palette and pick the `stats` folder. Flint shows the extension's program and permissions before copying it to its config folder. Manage it in **Settings ▸ Extensions**.

## Add one

- One folder per extension, named after it, with `flint-extension.json` at its root.
- Every extension has a `README.md`: what it adds (panels, commands, status items, agent tools), its settings, its permissions and what it does with them (what it reads, whether it uses the network or stores anything), and how to build and install it. [`stats/README.md`](stats/README.md) is the model.
- Go extensions are their own module (`go.mod` in the folder) and use only the standard library, like Flint itself.
- Declare only the permissions the extension uses, and describe in the manifest what it does with them.
- No telemetry, no network access the user doesn't ask for, and secrets only through Flint's `secrets/*` methods (the system keyring), never in settings or files.
- CI checks that every extension has a manifest with the required fields and a README, and vets, builds and tests every Go module. Add a line to the table above.

## License

MIT, like Flint.
