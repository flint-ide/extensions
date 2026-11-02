# Catppuccin

Catppuccin, the soothing pastel theme, in its four flavors: Latte (light), Frappé, Macchiato and Mocha. A color theme is data: this extension has no program and runs no code.

| | |
|---|---|
| Id | `flint.catppuccin` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never (no program) |

## What it adds

- **Catppuccin Latte** (light, `flint.catppuccin.latte`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.
- **Catppuccin Frappé** (dark, `flint.catppuccin.frappe`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.
- **Catppuccin Macchiato** (dark, `flint.catppuccin.macchiato`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.
- **Catppuccin Mocha** (dark, `flint.catppuccin.mocha`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.

Pick one in **Settings ▸ Appearance ▸ Color theme**, or with **Use …** on the extension in **Settings ▸ Extensions**. Flint copies the theme into your custom theme, so it stays when you uninstall the extension.

## Contrast

The colors are Catppuccin's own. In **Latte**, yellow (types), peach (numbers) and some accents are lighter than WCAG's 3:1 against the background; that is how Latte looks everywhere. Prefer Flint Paper or Gruvbox Light if you need more contrast in a light theme.

## Source and license

The palettes of [Catppuccin](https://github.com/catppuccin/catppuccin), mapped onto Flint's theme variables after its style guide.

Catppuccin is MIT-licensed (Copyright (c) 2021 Catppuccin).

## Privacy

It reads nothing, sends nothing over the network and stores nothing: it is a list of colors.

## Install

Nothing to build. In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The themes: CSS variables and terminal colors |
