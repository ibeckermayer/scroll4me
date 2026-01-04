# scroll4me

AI-powered X/Twitter digest — get the signal without the doomscrolling.

## Prerequisites

- [Go](https://golang.org/) (1.21+)
- [Task](https://taskfile.dev/) — a task runner

## Building

```bash
task build
```

This creates the binary at `./bin/scroll4me`.

## Running

scroll4me can run in two modes:

- **Tray App**: Run with no arguments. A system tray icon appears with menu options.
- **CLI**: Run individual commands directly from the terminal.

```bash
# Tray app mode
./bin/scroll4me

# CLI mode (see --help for all commands)
./bin/scroll4me --help
```

## Getting Started

Follow these steps to set up and generate your first digest:

| Step                  | Tray App Menu     | CLI Command                    |
| --------------------- | ----------------- | ------------------------------ |
| Open Config           | "Edit Config"     | `./bin/scroll4me open config`  |
| Add Anthropic API key | _(edit the file)_ | _(edit the file)_              |
| Reload Config         | "Reload Config"   | _(auto-loads on each CLI run)_ |
| Login to X            | "Login to X"      | `./bin/scroll4me login`        |
| Generate Digest       | "Generate Digest" | `./bin/scroll4me step all`     |

### Notes

- The **login** command opens a browser window where you log in to X normally. Your session cookies are saved to disk for subsequent scrapes.
- After generating a digest, it opens automatically. You can also view the last digest via "View Last Digest" in the tray menu or `./bin/scroll4me open digest`.

## Full Command Reference

```
./bin/scroll4me --help
```
