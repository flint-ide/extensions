# Python

Debugging and tests for Python files in Flint: a debugger through [debugpy](https://github.com/microsoft/debugpy) and a test runner through [pytest](https://docs.pytest.org). Python itself is one of Flint's own languages, so highlighting, comments and the language server are already there. This extension is data: it has no program of its own.

| | |
|---|---|
| Id | `flint.python` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never itself; debugpy starts on **Debug File** and pytest on **Run Tests in File**, both only in a trusted folder |

## What it adds

- **Debugger** (`debugpy`): **Debug File** runs the open Python file under debugpy, with the project folder as the working directory. Flint starts the first adapter it finds, `python3 -m debugpy.adapter` or `python -m debugpy.adapter`, and talks to it on standard input and output. The program's output goes to the debug console (`"console": "internalConsole"`), and stepping stays in your own code (`"justMyCode": true`). Install it with `python3 -m pip install debugpy`.
- **Test runner** (`pytest`): **Run Tests in File** runs `python3 -m pytest <file>` in the project folder. The format is `plain`: the file passes when pytest exits with 0. Install it with `python3 -m pip install pytest`.

Both use the `python3` (or, for the debugger, `python`) that is first on your `PATH`, so debugpy and pytest must be installed for that interpreter.

## Not included

- **A `languages` entry.** Flint highlights `.py` files and runs their language server itself, and an extension can't take those files over.
- **Results for each test.** pytest prints TAP only with an extra plugin (`pytest-tap`), so the runner reports the file as a whole and can't run one test again.
- **Attaching** to a running process, and choosing the interpreter of a virtual environment that isn't first on `PATH`: the manifest has no variable for either.

## Settings

None.

## Privacy

The extension reads nothing and sends nothing: it is a description of two commands. debugpy and pytest are your own installed programs. Flint starts them only in trusted folders, because both run the project's code. debugpy's adapter talks to the program it debugs over a socket on your own machine (`127.0.0.1`). Neither tool uses the network otherwise, though your code and your pytest plugins may.

## Install

In Flint, open **Settings ▸ Extensions** and click **Install…** next to Python under **Repository**. There's nothing to build. Or run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The debugger (adapter candidates and launch arguments) and the test runner command |
