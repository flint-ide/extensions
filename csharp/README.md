# C#

C# in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through a C# language server. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.csharp` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a C# file in a trusted folder |

## What it adds

- **C# files**: `.cs` and `.csx`.
- **Highlighting**: keywords and common contextual keywords (`async`, `await`, `var`, `get`, `set`, `init`, `record`, `where`, …), the names after `class`, `struct`, `interface`, `enum`, `record` and `namespace`, built-in types, generic and declared types, `[Attributes]` at the start of a line, `#if`/`#region` and other preprocessor lines, regular, verbatim (`@"…"`), interpolated (`$"…{x}…"`, `$@"…"`) and raw (`"""…"""`) strings, character literals, `@identifiers`, numbers with suffixes, `//` and `///` comments and `/* … */` blocks.
- **Toggle Comment** with `//`, and `/*`/`*/` for blocks.
- **Language server**: Flint runs the first one installed, [csharp-ls](https://github.com/razzmatazz/csharp-language-server) (`csharp-ls`) or [OmniSharp](https://github.com/OmniSharp/omnisharp-roslyn) (`OmniSharp -lsp`). Install csharp-ls with `dotnet tool install --global csharp-ls` (it needs the .NET SDK, and `~/.dotnet/tools` on your `PATH`); for OmniSharp, unpack a release and put its `OmniSharp` program on your `PATH`. Both load the `.sln` or `.csproj` in the project folder. Without one, you still get highlighting and comments.

### Not included

- **Debugger**: [netcoredbg](https://github.com/Samsung/netcoredbg) speaks the Debug Adapter Protocol on standard input and output (`netcoredbg --interpreter=vscode`), but its `launch` needs `program`, the path of the built `.dll` (such as `bin/Debug/net8.0/App.dll`). None of the variables a manifest can use (`${file}`, `${fileBasename}`, `${fileDirname}`, `${workspaceFolder}`) gives that path, and Flint doesn't build the project before it launches, so this extension adds no debugger.
- **Test runner**: `dotnet test` runs the tests of a project or solution, not of one file, and refuses to start in a folder with more than one project or solution file. A runner can only name the file (`${file}`, `${fileBasename}` with its `.cs`), which `dotnet test` can't filter by, so **Run Tests in File** would run every test of the project. This extension adds no test runner rather than one that does more than it says.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server is your own installed program. Flint starts it only in trusted folders, because loading a project evaluates its MSBuild files, which can run code. It reads the project like any language server. Packages the project uses must be restored for full results; `dotnet restore` downloads them from NuGet or the feeds the project names.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to C# under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language: files, comments, language servers and the highlighting grammar |
