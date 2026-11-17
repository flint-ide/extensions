# PHP

PHP in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through a PHP language server, plus **Run Tests in File** with PHPUnit. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.php` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a PHP file in a trusted folder, and PHPUnit when you run a file's tests there |

## What it adds

- **PHP files**: `.php` and `.phtml`.
- **Highlighting**: HTML outside `<?php … ?>` and `<?= … ?>` (tag names only), keywords, `function`, `class`, `interface`, `trait` and `enum` names, `$variables` and `$this`, `->` and `?->` properties, type names and casts, class names, `true`/`false`/`null` and magic constants, `#[…]` attributes, numbers, strings with escapes and `$var` / `{$expr}` interpolation, heredocs, `//`, `#` and `/* … */` comments.
- **Toggle Comment** with `//`, and `/*`/`*/` for blocks.
- **Language server**: Flint runs the first one installed, [Intelephense](https://intelephense.com) (`intelephense --stdio`) or [Phpactor](https://phpactor.readthedocs.io) (`phpactor language-server`). Install one with `npm install -g intelephense`, or install Phpactor as its documentation describes. Without one, you still get highlighting and comments.
- **Run Tests in File** with PHPUnit: Flint runs `php vendor/bin/phpunit <file>` in the project folder, so it uses the PHPUnit your project installs with Composer (`composer require --dev phpunit/phpunit`) and its `phpunit.xml`. Only the exit code counts: 0 passed.

### Not included

- **A debugger.** The PHP debug adapter ([vscode-php-debug](https://github.com/xdebug/vscode-php-debug)) is not a program on `PATH`, and it works by waiting for Xdebug to connect, which Flint's debugger doesn't do (it starts an adapter and sends `launch`). So **Debug File** isn't offered for PHP.
- **Running single tests.** PHPUnit doesn't print [TAP](https://testanything.org/) any more, so Flint reads only its exit code and runs a whole file at a time.

### Limits of the highlighting

It is a simple grammar, not a parser. Keywords are matched in lower case only (PHP accepts `IF` too). A heredoc ends at the first line that holds only an upper-case name (such as `SQL;`), so a line like that inside the text ends it early. HTML attributes, `<script>` and `<style>` aren't highlighted.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server and PHPUnit are your own installed programs. Flint starts them only in trusted folders: PHPUnit runs your project's code and its bootstrap file, and a language server reads the whole project, `vendor/` included. Intelephense and Phpactor work on the local files; what else they do is up to them and their own settings.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to PHP under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language (files, comments, language servers and the highlighting grammar) and the PHPUnit test runner |
