# Ruby

Ruby in Flint's editor: syntax highlighting, comments, completion, hover, go to definition, references and diagnostics through a Ruby language server, and **Run Tests in File** with RSpec. A language is data: this extension has no program of its own.

| | |
|---|---|
| Id | `flint.ruby` |
| Version | 0.2.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; the language server starts when you open a Ruby file in a trusted folder, and RSpec when you run **Run Tests in File** in one |

## What it adds

- **Ruby files**: `.rb`, `.rake`, `.gemspec`, `.ru`, `.erb` and `Gemfile`, `Rakefile`, `Guardfile`, `Podfile`, `Brewfile`, `Vagrantfile`.
- **Highlighting**: keywords, `def` names, symbols and `key:` labels, strings with `#{…}` interpolation, `%w{…}` literals, `@instance`, `@@class` and `$global` variables, constants, numbers, and `=begin`/`=end` blocks.
- **Toggle Comment** with `#`, and `=begin`/`=end` for blocks.
- **Language server**: Flint runs the first one installed, [Ruby LSP](https://shopify.github.io/ruby-lsp/) (`ruby-lsp`) or [Solargraph](https://solargraph.org) (`solargraph stdio`). Install one with `gem install ruby-lsp` (or `gem install solargraph`). Without one, you still get highlighting and comments.
- **Test runner**: **Run Tests in File** runs `bundle exec rspec <file>` in the project folder. The file passes when RSpec exits with 0; its output is shown as it is, without a result for each example. It needs [Bundler](https://bundler.io) and a `Gemfile` that lists `rspec`.

## Not included

- **A debugger.** `rdbg` (the [`debug`](https://github.com/ruby/debug) gem) speaks the Debug Adapter Protocol only over a UNIX domain socket or a TCP port that a client attaches to (`rdbg --open`). It has no mode that speaks it on its standard input and output, which is the only way Flint starts an adapter, so this extension adds none.
- **Minitest.** A test runner is one fixed command for every file of a language, and the manifest has no way to pick a runner by file name (RSpec for `_spec.rb`, Minitest for `_test.rb`), nor says which of two runners for the same language in one extension would be used. So the extension ships RSpec only.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of the language and of the command that runs its tests. The language server, Bundler and RSpec are your own installed programs. Flint starts them only in trusted folders, because a Ruby server loads the project's `Gemfile` and may run its code, and a test run does run the project's code. The server reads the project like any language server. Ruby LSP, Solargraph and RSpec don't use the network on their own, though Bundler may if your `Gemfile` asks it to, and your tests may.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Ruby under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The language (files, comments, language servers and the highlighting grammar) and the RSpec test runner |
