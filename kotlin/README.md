# Kotlin

Kotlin in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through a Kotlin language server. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.kotlin` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a Kotlin file in a trusted folder |

## What it adds

- **Kotlin files**: `.kt` and `.kts` (Kotlin scripts, including `build.gradle.kts` and `settings.gradle.kts`).
- **Highlighting**: keywords and modifiers, the names declared by `fun`, `class`, `interface`, `object`, `typealias`, `val` and `var` (extension receivers such as `String.` too), annotations like `@JvmStatic` and `@file:JvmName`, labels like `loop@`, strings and `"""raw strings"""` with `$name` and `${…}` templates and `\n`, `A` escapes, character literals, `` `backtick names` ``, types, numbers (`0xFF`, `0b1010`, `1_000L`, `2.5f`, `10u`), common standard-library functions, `it`, operators such as `?.`, `?:`, `!!` and `..<`, `//` comments, nested `/* … */` blocks with KDoc `@param` tags, and a `#!` line in scripts.
- **Toggle Comment** with `//`, and `/* … */` for blocks.
- **Language server**: Flint runs the first one installed, [Kotlin Language Server](https://github.com/fwcd/kotlin-language-server) (`kotlin-language-server`) or JetBrains' [kotlin-lsp](https://github.com/Kotlin/kotlin-lsp) (`kotlin-lsp --stdio`). Install one with `brew install kotlin-language-server` (or `brew install JetBrains/utils/kotlin-lsp`). Both work best on a Gradle or Maven project. Without one, you still get highlighting and comments.

### Not included

- **Debugger.** [kotlin-debug-adapter](https://github.com/fwcd/kotlin-debug-adapter) speaks DAP on standard input and output, but its `launch` needs `mainClass`, the fully qualified class of the `main` function (`com.example.AppKt`), which no variable Flint offers can give, and the project must be compiled first. Debug Kotlin from Gradle or IntelliJ IDEA instead.
- **Test runner.** Gradle and Maven select tests by class name, not by file, so no command can honestly run the tests of one file.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server is your own installed program. Flint starts it only in trusted folders, because a Kotlin server imports the project's Gradle or Maven build, which runs the build scripts' code. It reads the project like any language server. Importing the build may download dependencies, as Gradle or Maven would.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Kotlin under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language: files, comments, language servers and the highlighting grammar |
