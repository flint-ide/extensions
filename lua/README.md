# Lua

Lua in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through lua-language-server, and the tests of a file with Busted. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.lua` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a Lua file in a trusted folder, and Busted when you run a file's tests there |

## What it adds

- **Lua files**: `.lua`, `.rockspec` and `.luacheckrc`.
- **Highlighting**: keywords, `function` names (also `M.name` and `obj:method`), `true`/`false`/`nil`/`self`, the standard library globals, strings with `\` escapes, long strings `[[…]]` and `[=[…]=]`, block comments `--[[…]]`, `::labels::`, `<const>`/`<close>` attributes, decimal and hex numbers, and operators.
- **Toggle Comment** with `--`, and `--[[`/`]]` for blocks.
- **Language server**: Flint runs [lua-language-server](https://luals.github.io) (`lua-language-server`). Install it with `brew install lua-language-server` or see its [install page](https://luals.github.io/#install). Without it, you still get highlighting and comments.
- **Tests**: **Run Tests in File** runs [Busted](https://lunarmodules.github.io/busted/) as `busted -o TAP <file>` in the project folder, and Flint reads each test's result from Busted's TAP output. Install it with `luarocks install busted`.

### Not included

- **A debugger.** No Lua debug adapter is a program on `PATH` that speaks DAP on standard input and output; the existing ones run inside another editor's extension host.
- **Running single tests.** Busted takes test names only as a Lua pattern (`--filter`), and Flint passes the selected names as plain arguments, which Busted would read as files. The whole file always runs.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server and Busted are your own installed programs. Flint starts them only in trusted folders: Busted runs the project's code, and lua-language-server reads the project and its `.luarc.json` like any language server.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Lua under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language: files, comments, the language server, the highlighting grammar and the Busted test runner |
