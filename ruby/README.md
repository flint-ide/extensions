# Ruby

Ruby in Flint's editor: syntax highlighting, comments, and completion, hover, go to definition, references and diagnostics through a Ruby language server. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.ruby` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a Ruby file in a trusted folder |

## What it adds

- **Ruby files**: `.rb`, `.rake`, `.gemspec`, `.ru`, `.erb` and `Gemfile`, `Rakefile`, `Guardfile`, `Podfile`, `Brewfile`, `Vagrantfile`.
- **Highlighting**: keywords, `def` names, symbols and `key:` labels, strings with `#{…}` interpolation, `%w{…}` literals, `@instance`, `@@class` and `$global` variables, constants, numbers, and `=begin`/`=end` blocks.
- **Toggle Comment** with `#`, and `=begin`/`=end` for blocks.
- **Language server**: Flint runs the first one installed, [Ruby LSP](https://shopify.github.io/ruby-lsp/) (`ruby-lsp`) or [Solargraph](https://solargraph.org) (`solargraph stdio`). Install one with `gem install ruby-lsp` (or `gem install solargraph`). Without one, you still get highlighting and comments.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language. The language server is your own installed program. Flint starts it only in trusted folders, because a Ruby server loads the project's `Gemfile` and may run its code. It reads the project like any language server. Ruby LSP and Solargraph don't use the network on their own, though Bundler may if your `Gemfile` asks it to.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Ruby under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language: files, comments, language servers and the highlighting grammar |
