# Rust

Tests for Rust projects in Flint, through [cargo test](https://doc.rust-lang.org/cargo/commands/cargo-test.html). Rust itself is one of Flint's own languages, so highlighting, comments and the language server (rust-analyzer) are already there. This extension is data: it has no program of its own.

| | |
|---|---|
| Id | `flint.rust` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; cargo starts on **Run Tests in File**, only in a trusted folder |

## What it adds

- **Test runner** (`cargo-test`): **Run Tests in File** on a `.rs` file runs `cargo test` in the project folder. Cargo can't be pointed at the tests of one source file, so this runs **every test of the package** (or of the workspace members cargo picks from the project folder), not only the open file's; the label says so. The format is `plain`: the run passes when cargo exits with 0, which also means everything compiled. The project folder must contain a `Cargo.toml`, or sit below one. Install Rust and cargo with [rustup](https://rustup.rs).

It uses the `cargo` that is first on your `PATH`.

### Not included

- **A `languages` entry.** Flint highlights `.rs` files and runs rust-analyzer itself, and an extension can't take those files over.
- **A debugger.** lldb-dap (formerly lldb-vscode) speaks the Debug Adapter Protocol on standard input and output, but its `launch` needs the path of the compiled binary, such as `target/debug/<crate>`. The manifest's variables (`${file}`, `${fileBasename}`, `${fileDirname}`, `${workspaceFolder}`) can't name it, and nothing builds it before the launch.
- **Only the open file's tests.** Integration tests in `tests/<name>.rs` run alone with `cargo test --test <name>`, and unit tests by module path, but the manifest has no variable for a file name without its extension or a module path.
- **Results for each test.** cargo's test harness doesn't print TAP, so the runner reports the run as a whole and can't run one test again.
- **A `Cargo.toml` outside the project folder's ancestry**, such as a crate in a subfolder: there is no variable for its path, and `cargo -C` is unstable.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of one command. cargo is your own installed program, and Flint starts it only in trusted folders, because it builds and runs the project's code, including build scripts. cargo may download crates from the registries your project uses when they aren't cached yet; your code and its tests may use the network too.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Rust under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The test runner command |
| `README.md` | This page |
