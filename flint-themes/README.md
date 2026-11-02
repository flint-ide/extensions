# Flint Themes

Flint's own color themes: Graphite, a dark stone gray with a spark of orange, and Paper, a warm light theme for bright rooms. A color theme is data: this extension has no program and runs no code.

| | |
|---|---|
| Id | `flint.themes` |
| Version | 0.1.0 |
| Contract | 1 |
| Permissions | none |
| Starts | never (no program) |

## What it adds

- **Flint Graphite** (dark, `flint.themes.graphite`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.
- **Flint Paper** (light, `flint.themes.paper`): editor and UI colors, syntax, git and diff colors, and the 16 terminal colors.

Pick one in **Settings ▸ Appearance ▸ Color theme**, or with **Use …** on the extension in **Settings ▸ Extensions**. Flint copies the theme into your custom theme, so it stays when you uninstall the extension.

## Source and license

Original themes made for Flint.

MIT, like this repository.

## Privacy

It reads nothing, sends nothing over the network and stores nothing: it is a list of colors.

## Install

Nothing to build. In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The themes: CSS variables and terminal colors |
