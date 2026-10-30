<div align="center">

<img src="web/public/logo.svg" alt="Octopus Logo" width="100">

### Octopus

A personal fork of [bestruirui/octopus](https://github.com/bestruirui/octopus) (based on upstream v0.13.13). 简体中文见 [README_zh.md](README_zh.md)。

</div>

## Changes from upstream

- **CNY display**: configurable USD→CNY rate; amounts can be shown in CNY, and model prices can be entered in CNY (stored as USD under the hood).
- **Ollama preset**: quick setup for a local Ollama server (no API key required).
- **Repo migration**: Go module and author info moved to `91xusir`; in-app update check points to this repo's [Releases](https://github.com/91xusir/octopus/releases).
- **Docker publishing removed**: binaries only.

## Usage

Download a package from [Releases](https://github.com/91xusir/octopus/releases) and run:

```bash
./octopus start
```

Then open http://localhost:8080 (default `admin` / `admin` — change the password right away).

For installation, configuration and feature docs, see the [upstream README](https://github.com/bestruirui/octopus).
