# C and C++

C and C++ in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through clangd. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.cpp` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a C or C++ file in a trusted folder |

## What it adds

- **C files** (language `c`): `.c`, `.i`.
- **C++ files** (language `cpp`): `.cpp`, `.cc`, `.cxx`, `.c++`, `.cppm`, `.ccm`, `.cxxm`, `.ixx`, `.h`, `.hpp`, `.hh`, `.hxx`, `.h++`, `.inl`, `.ipp`, `.tpp`, `.txx`, `.ii`. A `.h` header opens as C++, as in most editors; clangd still treats it as C when your compile commands say so.
- **Highlighting**: keywords, the names after `struct`, `union` and `enum` (and in C++ `class`, `namespace` and `concept`), built-in and fixed-width types (`uint32_t`), preprocessor directives and `#include` paths, strings with escape sequences (and `printf` format specifiers in C), character literals, C++ raw strings `R"delim(…)delim"`, numbers with digit separators and suffixes, `ALL_CAPS` macros, capitalized type names, operators, and `/* … */` block comments.
- **Toggle Comment** with `//`, and `/*` `*/` for blocks.
- **Language server**: [clangd](https://clangd.llvm.org) (`clangd`) for both languages. On macOS it comes with the Xcode Command Line Tools (`xcode-select --install`) or `brew install llvm`; on Debian and Ubuntu, `apt install clangd`. clangd needs to know how your code is compiled: put a `compile_commands.json` (CMake writes one with `-DCMAKE_EXPORT_COMPILE_COMMANDS=ON`) or a `compile_flags.txt` at the project root. Without clangd, you still get highlighting and comments.

### Not included

- **Debugger**: lldb-dap (formerly lldb-vscode) speaks DAP on standard input and output, but its `launch` needs `program`, the path of a compiled binary. Flint's launch variables name the source file, not a build output, so there is no honest launch configuration to ship.
- **Test runner**: C and C++ tests are run per build (`ctest` in a CMake build folder), not per source file, so **Run Tests in File** has nothing true to run.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server is your own installed program. Flint starts it only in trusted folders, because clangd reads the project's compile commands and its `.clangd` configuration. It reads the project like any language server and builds an index of it in a `.cache/clangd` folder in the project. clangd doesn't use the network.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to C and C++ under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The languages: files, comments, the language server and the highlighting grammars |
