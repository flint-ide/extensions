# Tokyo Night

Tokyo Night, a clean theme celebrating the lights of downtown Tokyo at night: Night, Storm and Day (light). A color theme is data: this extension has no program and runs no code.

| | |
|---|---|
| Id | `flint.tokyo-night` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never (no program) |

## What it adds

- **Tokyo Night** (dark, `flint.tokyo-night.night`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.
- **Tokyo Night Storm** (dark, `flint.tokyo-night.storm`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.
- **Tokyo Night Day** (light, `flint.tokyo-night.day`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.

Pick one in **Settings ▸ Appearance ▸ Color theme**, or with **Use …** on the extension in **Settings ▸ Extensions**. Flint copies the theme into your custom theme, so it stays when you uninstall the extension.

## Contrast

The colors are Tokyo Night's own. In **Day**, the blue text on the sidebar reaches 4:1, a little under WCAG's 4.5:1 for small text.

## Source and license

The palettes of [tokyonight.nvim](https://github.com/folke/tokyonight.nvim) (after the original [Tokyo Night VS Code theme](https://github.com/enkia/tokyo-night-vscode-theme) by enkia).

tokyonight.nvim is Apache-2.0 licensed (Copyright folke); the original Tokyo Night theme is MIT-licensed (Copyright enkia).

## Privacy

It reads nothing, sends nothing over the network and stores nothing: it is a list of colors.

## Install

Nothing to build. In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The themes: CSS variables and terminal colors |
