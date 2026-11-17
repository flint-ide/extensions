# Elixir

Elixir in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through an Elixir language server, plus **Debug File** through ElixirLS's debug adapter and **Run Tests in File** through `mix test`. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.elixir` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open an Elixir file in a trusted folder, the debug adapter and `mix test` when you debug or test a file there |

## What it adds

- **Elixir files**: `.ex` and `.exs` (including `mix.exs` and `.formatter.exs`). Templates (`.eex`, `.heex`, `.leex`) are not included: they are HTML with embedded Elixir, which this grammar doesn't handle.
- **Highlighting**: keywords, names after `def`, `defp`, `defmacro` and friends, module names, `:atoms`, `:"quoted atoms"` and `key:` labels, strings, charlists and `"""` heredocs with `#{…}` interpolation, one-line sigils (`~r/…/`, `~w(…)`, `~s{…}` and the other delimiters) and `~S"""` heredoc sigils, `@module_attributes` (and `@doc`/`@moduledoc`), `_ignored` variables, numbers and `?a` code points, the pipe and other operators, and `<<…>>` binaries.
- **Toggle Comment** with `#`. Elixir has no block comments.
- **Language server**: Flint runs the first one installed: [ElixirLS](https://github.com/elixir-lsp/elixir-ls) (`elixir-ls`, as Homebrew installs it, or the release's `language_server.sh` on your `PATH`), [Expert](https://github.com/elixir-lang/expert) (`expert --stdio`) or [Next LS](https://github.com/elixir-tools/next-ls) (`nextls --stdio`). Install one with `brew install elixir-ls`. Without one, you still get highlighting and comments.
- **Debugger** (`mix_task`): ElixirLS's debug adapter (`elixir-ls-debugger`, as Homebrew installs it, or `debug_adapter.sh`) on standard input and output. **Debug File** runs `mix test` on the current file in the project folder, with the project's apps started, and stops at your breakpoints in the test and in the project's modules. Breakpoints in test files work because the launch loads `test/**/test_helper.exs` and `test/**/*_test.exs`, as ElixirLS requires.
- **Test runner** (`mix-test`): **Run Tests in File** runs `mix test <file>` in the project folder. ExUnit doesn't print TAP, so only the exit code counts: 0 passed. The output shows which tests failed.

### Not included

- **Debugging something other than a test file.** Flint sends one fixed `launch` per debugger, and this one runs `mix test` on the file. Debug File on a module in `lib/` makes `mix test` report that the path matches no test file; to stop in a `lib/` module, set the breakpoint there and debug a test file that calls it.
- **Running a single test.** `mix test` selects one test by `file:line`, which none of Flint's variables provide, and it doesn't print TAP, so Flint can't rerun one test from the results.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server, debug adapter and `mix` are your own installed programs. Flint starts them only in trusted folders, because they compile the project and run its code: a server builds the project with Mix, and the debugger and test runner run its tests. They read the project like any language server or build tool. None of them use the network on their own, though Mix or the language server may fetch dependencies if the project or the server's own settings ask for it.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Elixir under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language (files, comments, language servers and the highlighting grammar), the ElixirLS debugger and the `mix test` runner |
