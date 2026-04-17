# Forge Context Server

**Your portable second brain — accessible to any AI agent via the Model Context Protocol.**

A Go-based MCP server that serves David Thorburn's personal and business context (identity, projects, documents, decisions) to any MCP-compatible AI agent — Hector, Claude Desktop, Cursor, Codex, etc.

## Philosophy

- **Own your context.** Don't let it live in a vendor's database.
- **Portable.** Run on localhost, VPS, homelab — your choice.
- **Composable.** Any MCP client can use it.
- **Resellable.** This is an Eldradesk product pattern, not just a personal tool.

## Architecture

```
┌──────────────────────────────────┐
│  Forge Context Server (this)     │
│  Go binary, ~15MB, single bin    │
│                                  │
│  ├── Sources (pluggable)         │
│  │   ├── filesystem (workspace)  │
│  │   ├── obsidian (vaults)       │
│  │   ├── github (repos)          │
│  │   └── postgres (structured)   │
│  ├── Tools (MCP exposed)         │
│  │   ├── get_identity            │
│  │   ├── get_projects            │
│  │   ├── search_documents        │
│  │   ├── get_recent_decisions    │
│  │   └── get_context_bundle      │
│  └── Transport                   │
│      ├── stdio (local agents)    │
│      ├── http (network agents)   │
│      └── sse (streaming)         │
└──────────────────────────────────┘
          │ MCP
   ┌──────┴──────┬──────────┐
   │             │          │
 Hector       Cursor    Claude
                        Desktop
```

## Quick Start

```bash
# Build
go build -o forge ./cmd/forge

# Run locally (stdio — for IDE integration)
./forge --transport=stdio --config=configs/dev.yaml

# Run as HTTP server (for network agents)
./forge --transport=http --port=8080 --config=configs/prod.yaml
```

## Configuration

See `configs/` for example configurations. Each source type has its own section.

## Deployment

This server is designed to run on Proxmox VM `context-mcp-server` (192.168.1.144).
Connect via Tailscale for secure remote access from any agent, any device.

## Status

🚧 Scaffold only — this is day 1 of development.
