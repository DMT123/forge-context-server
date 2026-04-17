# Forge Context Server

**Your portable second brain — accessible to any AI agent via the Model Context Protocol.**

A Go-based MCP server that serves your personal and business context (identity, projects, documents, decisions, memories) to any MCP-compatible AI agent — Claude Desktop, Cursor, ChatGPT (via proxy), custom agents, etc.

## Philosophy

- **Own your context.** Don't let it live in a vendor's database.
- **Portable.** Run on localhost, homelab, VPS — your choice.
- **Composable.** Any MCP client can use it.
- **Write-capable.** Agents persist observations back to your brain.

## Features

- **5 MCP tools**: `get_identity`, `list_projects`, `search_documents`, `get_document`, `get_context_bundle`, `add_memory`
- **3 source backends** (pluggable, add more):
  - `workspace` — any filesystem tree of markdown files
  - `obsidian` — Obsidian vaults with frontmatter + tags + daily logs
  - `memories` — writable backing store for agent memories
- **2 transports**: stdio (for local agents) and HTTP (for networked access)
- **Single Go binary** — ~8MB, <10MB RAM at rest
- **Battle-tested** in David's homelab on Proxmox via Tailscale + Cloudflare Tunnel

## Quick Start

```bash
git clone https://github.com/DMT123/forge-context-server
cd forge-context-server
go build ./cmd/forge

# Edit configs/dev.yaml to point at YOUR workspace / vault
./forge --config=configs/dev.yaml
```

## Claude Desktop Integration

Add to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS):

```json
{
  "mcpServers": {
    "forge": {
      "command": "/absolute/path/to/forge",
      "args": ["--config=/absolute/path/to/configs/dev.yaml"],
      "type": "stdio"
    }
  }
}
```

Restart Claude Desktop. Five new tools appear under the 🔨 icon.

## Production Deployment

See `docs/deployment.md` (coming soon) for:
- Cross-compilation for Linux
- systemd service definition (hardened: NoNewPrivileges, ProtectSystem)
- Tailscale-only firewall rules
- Cloudflare Tunnel for public HTTPS

## Status

v0.1.0 — shipping. More sources + write capabilities landing.

## License

MIT (TBC).
