# DavzyVault

**Your portable second brain — accessible to any AI agent via the Model Context Protocol.**

A Go-based MCP server that serves your personal and business context (identity, projects, documents, decisions, memories) to any MCP-compatible AI agent — Claude Desktop, Cursor, ChatGPT (via proxy), custom agents, etc.

## Philosophy

- **Own your context.** Don't let it live in a vendor's database.
- **Portable.** Run on localhost, homelab, VPS — your choice.
- **Composable.** Any MCP client can use it.
- **Write-capable.** Agents persist observations back to your brain.

## Features

- **6 MCP tools**: `get_identity`, `list_projects`, `search_documents`, `get_document`, `get_context_bundle`, `add_memory`
- **3 source backends** (pluggable):
  - `workspace` — any filesystem tree of markdown files
  - `obsidian` — Obsidian vaults with frontmatter + tags + daily logs
  - `memories` — writable backing store for agent memories
- **2 transports**: stdio (for local agents) and HTTP (for networked access)
- **Single Go binary** — ~8MB, <10MB RAM at rest

## Quick Start

```bash
git clone https://github.com/DMT123/davzy-vault
cd davzy-vault
go build -o davzy-vault ./cmd/davzy-vault

./davzy-vault --config=configs/dev.yaml
```

## Claude Desktop Integration

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "davzy-vault": {
      "command": "/absolute/path/to/davzy-vault",
      "args": ["--config=/absolute/path/to/configs/dev.yaml"],
      "type": "stdio"
    }
  }
}
```

## History

Previously known as "Forge Context Server". Renamed 2026-04-18 to disambiguate from the Forge DevOps agent and the (future) Forge Model Foundry.

## License

MIT (TBC).
