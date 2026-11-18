# Java

Java in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through the Eclipse JDT language server. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.java` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a Java file in a trusted folder |

## What it adds

- **Java files**: `.java` (which covers `module-info.java` and `package-info.java`).
- **Highlighting**: keywords, the names after `class`, `interface`, `enum`, `record` and `@interface`, primitive and capitalised type names, `@Annotations`, strings and character literals with escapes, `"""` text blocks, numbers (decimal, hex, binary, with `_` and suffixes), operators, and `//`, `/* … */` and `/** … */` comments with their `@tags`.
- **Toggle Comment** with `//`, and `/*` `*/` for blocks.
- **Language server**: [Eclipse JDT LS](https://github.com/eclipse-jdtls/eclipse.jdt.ls), started through its `jdtls` launcher. Install it with `brew install jdtls`, or unpack a JDT LS release and put its `bin` folder on your `PATH`. The launcher is a Python script and the server needs a JDK to run (Java 21 or newer for current releases). Without it, you still get highlighting and comments.

### Not included

- **Debugger**: there is no standalone Java debug adapter that speaks the Debug Adapter Protocol on standard input and output. The usual one, `java-debug`, runs as a plug-in inside JDT LS and is reached over a TCP port the language server hands out, which a Flint debugger can't use.
- **Test runner**: Maven and Gradle pick tests by class name and by project or module, not by file path. Flint's variables give the file (`${file}`, `${fileBasename}`), not a class name, and one extension can't tell whether the project uses Maven or Gradle. Run tests from the terminal: `mvn test -Dtest=OrderServiceTest` or `gradle test --tests OrderServiceTest`.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server is your own installed program. Flint starts it only in trusted folders, because JDT LS imports the project's Maven or Gradle build, and a Gradle build is code that runs. It reads the project like any language server and keeps its index in its own data folder outside the project. Importing a build can download the project's dependencies, and a Gradle distribution named by the wrapper, from the repositories the build names.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Java under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language: files, comments, the language server and the highlighting grammar |
