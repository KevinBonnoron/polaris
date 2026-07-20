# Polaris

> Desktop cockpit to orchestrate multiple AI coding agents (Claude Code, GitHub Copilot, Cursor, Codex, Gemini, Mistral) across all your projects.

[![Wails](https://img.shields.io/badge/Wails-v3-DF0061?logo=go&logoColor=white)](https://wails.io/)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-6-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![Bun](https://img.shields.io/badge/Bun-1.3-000000?logo=bun&logoColor=white)](https://bun.com/)
[![Vite](https://img.shields.io/badge/Vite-8-646CFF?logo=vite&logoColor=white)](https://vitejs.dev/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-4-06B6D4?logo=tailwindcss&logoColor=white)](https://tailwindcss.com/)
[![TanStack Router](https://img.shields.io/badge/TanStack-Router-FF4154?logo=react-query&logoColor=white)](https://tanstack.com/router)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![Polaris welcome](docs/screenshots/wizard-home.png)

## Overview

Polaris is a desktop application (Wails + Go + React) that centralizes the management of your AI coding agents. Launch, monitor, and coordinate multiple agent CLIs in parallel, in isolated Git worktrees, with full visibility into their logs, token usage, and outputs.

## Features

### Multi-agent orchestration

Run multiple agents side by side across all your projects, each in its own isolated Git worktree.

| Agent | Models |
|---|---|
| **Claude Code** | Opus, Sonnet, Haiku (with extended thinking — real-time progressive streaming with 15 configurable visual styles) |
| **GitHub Copilot** | Model selected by gh CLI (no model override) |
| **Cursor** | 25+ models including thinking variants |
| **OpenAI Codex** | Models from local cache (~/.codex/models_cache.json) |
| **Google Gemini** | 2.5 Flash/Pro, Gemini 3 |
| **Mistral** | Medium 3.5, Devstral Small |
| **Custom providers** | Any provider via opencode ACP (full JSON-RPC 2.0 over stdin/stdout, supports OpenAI-compatible and Anthropic-compatible drivers) |

Agent CLIs are auto-detected from your PATH. Per-agent model defaults are persisted per project (with global fallback).

![Agent running](docs/screenshots/home-agent-working.png)

### Git & version control

- Stage/unstage files, commit with AI-generated messages, push, sync
- Branch management (list, switch, create, delete)
- Diff viewer (worktree vs HEAD)
- One-click PR creation from an agent's work (GitHub, via gh CLI)
- Each agent runs in its own Git worktree — zero interference with your main branch
- Promote a running agent to an isolated worktree post-hoc

### Integrations

| Integration | Capabilities |
|---|---|
| **GitHub** | PRs, issues, workflow runs, branches. Token auto-detected via gh CLI or GITHUB_TOKEN |
| **GitLab / Bitbucket** | Remote detection and auth only; API integration not yet implemented |
| **Jira** | Active sprint, issue creation, transitions, assignees, story points |
| **Node.js** | npm/Yarn/pnpm/Bun/Deno — detect scripts, run them, manage dependencies |
| **Python** | Poetry/PDM/uv/Pipenv/pip — scripts, dependencies, virtual envs |
| **C# / .NET** | Project detection, build and test task integration |
| **Godot** | Engine project detection |
| **Docker** | Dockerfile/Compose detection, linting, vulnerability scanning |
| **Taskfile** | Task runner detection, list and run tasks |
| **Nix devenv / devcontainer** | Dev environment detection and shell integration |
| **Sentry** | Issue list, event details, status management |
| **Dokploy** | Service status, deployment history, logs, restart/start/stop |
| **Slack / Discord / Telegram** | Webhook-based notifications |
| **Resend** | Email sending, domain management |

![Repository integration](docs/screenshots/repository.png)

### Automations

Trigger agent tasks automatically based on integration events (GitHub PRs/issues, Jira ticket transitions, Sentry alerts, Dokploy deployments), with full history and manual triggers.

### Appearance & customization

![Appearance settings](docs/screenshots/settings-appearance.png)

**Themes** — 29 built-in named themes (Catppuccin, Dracula, Nord, Tokyo Night, Gruvbox, and more) plus a custom theme importer: paste or upload a JSON token file to define your own color scheme.

**Thinking indicator** — 15 animated styles to visualize agent reasoning: Dots, Spinner, Badge, Bar, Wave, Orbit, Cursor, Breathing, Sine wave, Ellipsis, Ring, Cascade, Gradient, Bounce, Flicker. Each can run in neutral or accent color mode.

**Card animation** — 5 styles for agent cards while running: Ring, Shimmer, Bar, Glow, or None.

**Configurable status bar** — Drag-and-drop the blocks shown in the agent status bar. Available blocks: Model, Tools, Tokens, Tools used, Cost, Provider, Files, Duration, Usage (remaining or used). Add separators (·, |, —) between blocks. Order and visibility are persisted.

### IDE & terminal

Auto-detects VS Code, Cursor, Zed, Windsurf, JetBrains IDEs, Sublime Text, Xcode, and Vim / Neovim. Open any project or file directly from Polaris, with line/column navigation support. Custom IDE command templates supported.

### Notifications

In-app notification center with read/unread state. Focus-aware: notifications can pause while the app window is active.

## Stack

- **Backend**: Go + Wails v3, SQLite, agent runner with log capture
- **Frontend**: React 19, TypeScript, TanStack Router/DB/Form, Radix UI + Tailwind CSS 4, i18next (EN/FR)
- **Tooling**: Bun, Vite, Biome

## Getting started

The dev toolchain (Bun, Go, the Wails v3 CLI and [go-task](https://taskfile.dev/)) is provided by the Nix devShell. With [direnv](https://direnv.net/) the `.envrc` (`use flake`) loads it automatically; otherwise run `nix develop`.

```bash
task dev
```

`task dev` generates the Wails bindings, builds the frontend, and runs the app with hot reload — it is the single entry point and works from a fresh worktree.

To build the application locally:

```bash
task build
```

### Build with Nix

If you have [Nix](https://nixos.org/download) with flakes enabled, you can build the app straight from the repo with no other tooling installed:

```bash
nix build github:KevinBonnoron/polaris   # remote
nix build                                # from a local clone
./result/bin/polaris
```

Or run it directly:

```bash
nix run github:KevinBonnoron/polaris
```

## Scripts

| Command | Description |
|---|---|
| `task dev` | Run Wails in dev mode (bindings + frontend + hot reload) |
| `task build` | Build the desktop app to `bin/polaris` |
| `task run` | Run the last built binary |
| `task common:generate:bindings` | Regenerate Wails Go↔TS bindings |
| `task common:dev:frontend` | Run the Vite frontend dev server only |

Frontend-only scripts (run from `frontend/`): `bun run typecheck`, `bun run format`.

## License

MIT
