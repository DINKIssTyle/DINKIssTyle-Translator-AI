# Wails v3 Build Directory

This directory contains the Wails v3 beta.1 build configuration, shared tasks,
platform tasks, icons, package metadata, and installer assets.

- `config.yml` is the source of application metadata and development-mode settings.
- `Taskfile.yml` contains shared frontend, binding, icon, and packaging tasks.
- `darwin/`, `windows/`, and `linux/` contain platform-specific tasks and metadata.
- Build output is written to the repository-level `bin/` directory.

Run `wails3 build` for a production binary, `wails3 package` for the current
platform package, or `wails3 dev` for live development. When metadata changes,
regenerate platform assets with:

```bash
wails3 task common:update:build-assets
```
