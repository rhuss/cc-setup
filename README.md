# cc-mcp-setup

Interactive CLI to manage which MCP servers are active per project in [Claude Code](https://docs.anthropic.com/en/docs/claude-code). Define all your servers once in a central config, then cherry-pick which ones to enable for each project. This keeps Claude's context clean by loading only the tools you actually need.

## Install

```bash
# Build from source
make build

# Install to ~/.local/bin
make install
```

No runtime dependencies. Single static binary.

## Configuration

All server definitions live in a single central config file:

```
~/.config/mcp-setup/mcp.json
```

Respects `XDG_CONFIG_HOME` if set.

The format mirrors Claude Code's `mcpServers` entries exactly, plus a `description` field for display:

```json
{
  "servers": {
    "my-jira": {
      "description": "Company Jira instance",
      "type": "http",
      "url": "https://mcp-jira.example.com/mcp",
      "headers": {
        "Authorization": "Basic dXNlcjpwYXNz"
      }
    },
    "filesystem": {
      "description": "Local filesystem access",
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/projects"]
    }
  }
}
```

When writing to Claude's config, the `description` field is stripped and everything else is copied verbatim. The tool doesn't need to know about auth encoding or server-specific logic.

See `sample-servers.json` for a complete example with all transport types.

## Usage

### Interactive server selection (default)

```bash
mcp-setup
```

1. Choose scope: user-global (`~/.claude.json`) or project (`./mcp.json`)
2. Checkbox-select which servers to enable (pre-checked if already configured)
3. Review summary of adds/removes
4. Apply

Servers not in the central config are left untouched in Claude's config. You can safely use this alongside manually configured servers.

### Add a server interactively

```bash
mcp-setup add my-new-server
```

Walks through transport type, URL/command, authentication, and description in an interactive form. Writes directly to the central config.

### Remove servers

```bash
# Remove specific servers
mcp-setup remove my-server another-server

# Interactive removal (no args)
mcp-setup remove
```

### Import from existing config

If you already have servers configured in Claude Code, import them into the central config:

```bash
mcp-setup import
```

This reads from your project `.mcp.json`, user `~/.claude.json`, or both, and merges them into the central config. Existing entries are not overwritten.

### Print version

```bash
mcp-setup version
```

### Help

```bash
mcp-setup --help
mcp-setup <command> --help
```

## Server types

The tool supports all Claude Code MCP transport types:

**HTTP (streamable):**
```json
{
  "description": "My HTTP server",
  "type": "http",
  "url": "https://example.com/mcp",
  "headers": {
    "Authorization": "Bearer your_token"
  }
}
```

**SSE (server-sent events):**
```json
{
  "description": "My SSE server",
  "type": "sse",
  "url": "https://example.com/sse"
}
```

**stdio (local process):**
```json
{
  "description": "Local MCP server",
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path"],
  "env": {
    "SOME_VAR": "value"
  }
}
```

## How it works

The tool reads server definitions from `~/.config/mcp-setup/mcp.json` and writes to Claude Code's config files. When you select servers:

- **Selected servers** are written to the target config (`.mcp.json` or `~/.claude.json`), with the `description` field stripped
- **Unchecked servers** (that exist in the central config) are removed from the target config
- **Unknown servers** (not in the central config) are left untouched

This means you can use `mcp-setup` alongside manually configured servers without conflicts.
