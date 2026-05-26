# Polaris

> Desktop cockpit to orchestrate multiple AI coding agents (Claude Code, Copilot, Codex, Gemini, Mistral) across all your projects.

[![Wails](https://img.shields.io/badge/Wails-v2-DF0061?logo=go&logoColor=white)](https://wails.io/)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.9-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![Bun](https://img.shields.io/badge/Bun-1.3-000000?logo=bun&logoColor=white)](https://bun.com/)
[![Vite](https://img.shields.io/badge/Vite-8-646CFF?logo=vite&logoColor=white)](https://vitejs.dev/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind-4-06B6D4?logo=tailwindcss&logoColor=white)](https://tailwindcss.com/)
[![TanStack Router](https://img.shields.io/badge/TanStack-Router-FF4154?logo=react-query&logoColor=white)](https://tanstack.com/router)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Overview

Polaris is a desktop application (Wails + Go + React) that centralizes the management of your AI coding agents. Launch, monitor, and coordinate multiple agent CLIs in parallel, in isolated Git worktrees, with full visibility into their logs and outputs.

## Features

- **Multi-agent** — Native support for Claude Code, GitHub Copilot, Codex, Gemini, and Mistral. Automatic detection of installed CLIs.
- **Isolated worktrees** — Each agent works in its own Git worktree, without interfering with your main working tree.
- **Multi-project** — Manage all your repos from a single place, with per-project configurable integrations.
- **Integrations** — GitHub (PRs, issues, workflows, branches), Jira (sprints, ticket creation/transitions), auto-detected provider tokens.
- **Automations** — Schedule recurring tasks for your agents.
- **Notifications** — Native notification center with read/unread state and focus tracking.
- **Node.js runner** — Detects your `package.json` and runs scripts directly from the UI.
- **IDE detection** — Spots installed IDEs (VS Code, Cursor, Zed, JetBrains, Vim, Sublime…) to open projects in one click.
- **PR creation** — Generates a pull request from an agent's work without leaving the app.

## Stack

- **Backend**: Go + Wails v2, SQLite, agent runner with log capture
- **Frontend**: React 19, TypeScript, TanStack Router/DB/Form, Radix UI + Tailwind CSS 4, i18next
- **Tooling**: Bun, Vite, Biome

## Getting started

Requirements: [Bun](https://bun.com/), [Go](https://go.dev/) 1.22+, [Wails CLI](https://wails.io/docs/gettingstarted/installation).

```bash
bun install
bun run dev
```

To build the application:

```bash
bun run build
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
| `bun run dev` | Run Wails in dev mode (hot reload) |
| `bun run dev:web` | Run the Vite frontend only |
| `bun run build` | Production build of the desktop app |
| `bun run typecheck` | TypeScript type-check |
| `bun run format` | Format code with Biome |
| `bun run generate:bindings` | Regenerate Wails Go↔TS bindings |

## License

MIT
