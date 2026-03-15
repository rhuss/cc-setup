# cc-setup

Interactive CLI to manage which MCP servers and plugins are active per project in [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

![cc-setup demo](docs/demo.gif)

## Why this matters

Every MCP server you enable adds its tools to Claude's context. Every plugin adds its skills. When Claude sees dozens of tools and skills, two things happen: the context window fills up with tool descriptions instead of your actual conversation, and tool selection gets noisier because the model has to pick from a larger set of candidates.

The practical effect is real. An MCP server with 20 tools that you only need in one project still consumes context in every other project. Skills from plugins you forgot you enabled compete with the ones you actually want triggered. The model does its best, but giving it a focused toolset for each project produces better results than dumping everything in globally.

`cc-setup` solves this by letting you define all your servers and plugins once in a central registry, then selectively enable only the ones each project needs. A Kubernetes project gets your cluster tools. A documentation project gets your writing tools. Nothing more.

## Features

- **Central server registry** with per-project activation via checkbox selection
- **Inherited server detection** from parent directory `.mcp.json` files, with visual distinction
- **Real-time health checks** with colored status indicators (green = OK, yellow = auth required, red = unreachable)
- **Tool permissions management** to control which tools are auto-approved per server
- **OAuth credential reuse** from Claude Code's stored tokens, with automatic refresh
- **Dual scope support** for project-local (`.mcp.json`) and user-global (`~/.claude.json`) configs
- **Plugin enable/disable** per project, for plugins installed by Claude Code
- **Import from existing configs** to bootstrap the central registry from what you already have
- All three MCP transport types: HTTP (streamable), SSE, and stdio

## Install

### macOS

**Homebrew** (recommended):

```bash
brew install cc-deck/tap/cc-setup
```

**Install script** (alternative):

```bash
curl -fsSL https://raw.githubusercontent.com/cc-deck/cc-setup/main/install.sh | sh
```

### Linux

```bash
curl -fsSL https://raw.githubusercontent.com/cc-deck/cc-setup/main/install.sh | sh
```

The install script detects your OS and architecture (amd64/arm64), downloads the correct binary, verifies the SHA256 checksum, and installs to `~/.local/bin`.

To install to a different location:

```bash
INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/cc-deck/cc-setup/main/install.sh | sudo sh
```

### Other options

Pre-built binaries for all platforms are available on the [Releases page](https://github.com/cc-deck/cc-setup/releases).

To build from source:

```bash
make build
make install   # installs to ~/.local/bin
```

No runtime dependencies. Single static binary.

## Quick start

```bash
# If you already have servers in Claude Code, import them
cc-setup import

# Or add a new server interactively
cc-setup add my-server

# Launch the management UI
cc-setup
```

## Configuration

All server definitions live in a single central config file:

```
~/.config/cc-setup/mcp.json
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

When writing to Claude's config, the `description` field is stripped and everything else is copied verbatim.

See `sample-servers.json` for a complete example with all transport types.

## Usage

### Interactive management (default)

```bash
cc-setup
```

Opens a full-screen TUI with all your registered servers. Each server shows a health indicator dot, its transport type, endpoint, and auth method.

**Key bindings:**

| Key | Action |
|-----|--------|
| `space` / `x` | Toggle server selection |
| `a` | Add a new server |
| `e` / `enter` | Edit selected server |
| `d` | Delete selected server |
| `s` | Save selection to Claude config |
| `i` | Import servers from Claude config |
| `p` | Switch to project scope |
| `u` | Switch to user scope |
| `.` | Toggle between project/user scope |
| `tab` | Switch between MCP Servers and Plugins tabs |
| `/` | Filter servers |
| `q` / `esc` | Quit |

### Health checks

When the management screen opens, each server is probed asynchronously. Status appears as a colored dot next to the server name:

- **Green** (filled circle) - server connected and initialized successfully
- **Yellow** (filled circle) - server reachable but requires OAuth authentication
- **Red** (filled circle) - server unreachable or protocol error
- **Dim** (open circle) - check still in progress

For OAuth-protected servers, the CLI automatically uses tokens stored by Claude Code (see [OAuth credential reuse](#oauth-credential-reuse) below).

### Inherited servers

Claude Code merges `.mcp.json` files from parent directories. A server defined in `~/Work/.mcp.json` is effective in `~/Work/myproject/` even without a local `.mcp.json`. `cc-setup` detects these inherited servers and displays them alongside locally configured ones, so the management screen reflects the actual effective state.

**How it works:**

When in project scope, `cc-setup` walks from the current directory upward to the filesystem root, reading `.mcp.json` files along the way. Servers found in parent directories (that also exist in the central registry) appear in the list with distinct visual styling:

| Style | Meaning |
|-------|---------|
| Green `[x]` | Locally configured and enabled |
| Muted green `[x]` | Inherited from a parent directory and enabled |
| Dim `[ ]` | Not configured (normal server) |
| Grey `[ ]` | Inherited but disabled |

Inherited servers also show dimmed name and detail text when not focused, making it easy to distinguish them from locally managed servers at a glance.

**Conflict resolution:** If the same server appears in both the local `.mcp.json` and a parent directory, the local definition wins and the server is treated as non-inherited (normal styling). Among parent directories, the closest parent wins.

**Save behavior for inherited servers:**

- Checking an inherited server is a no-op on save (it's already active via the parent config, no redundant entry is written to the local `.mcp.json`)
- Unchecking an inherited server adds it to `disabledMcpjsonServers` in `.claude/settings.local.json`, which tells Claude Code to suppress it for this project
- Re-checking a previously disabled inherited server removes it from `disabledMcpjsonServers`

**Detail view:** Pressing `e`/`enter` on any server shows a "Source" field indicating which `.mcp.json` file provides the server definition.

Inheritance only applies in project scope. In user scope, inherited servers are not shown.

### Tool permissions

Enter a server's detail view (`e`/`enter`) and select "Tool permissions" to discover its tools and configure which ones Claude Code may auto-approve.

- Permissions are written to `settings.local.json` (project or user scope)
- When all tools are selected, a wildcard entry (`mcp__<server>__*`) is used
- Individual tool entries (`mcp__<server>__<tool>`) are written for partial selections
- The central config's `autoApprove` field is kept in sync

Tool annotations are shown when provided by the server: read-only tools show an eye icon, destructive tools show a warning icon.

### Add a server interactively

```bash
cc-setup add my-new-server
```

Walks through transport type, URL/command, authentication, and description in an interactive form. Writes directly to the central config.

### Remove servers

```bash
# Remove specific servers
cc-setup remove my-server another-server

# Interactive removal (no args)
cc-setup remove
```

### Import from existing config

If you already have servers configured in Claude Code, import them into the central config:

```bash
cc-setup import
```

This reads from your project `.mcp.json`, user `~/.claude.json`, or both, and merges them into the central config. Existing entries are not overwritten.

### Print version

```bash
cc-setup version
```

## Plugin management

The Plugins tab (press `tab` to switch) lets you control which Claude Code plugins are active per project. Unlike MCP servers, plugins are not managed through a central config. Instead, `cc-setup` discovers plugins directly from Claude Code's own plugin cache (`~/.claude/plugins/cache/`).

Installing, updating, and removing plugins is still handled by Claude Code itself. What `cc-setup` does is let you toggle which installed plugins are enabled or disabled, per scope.

**How it works:**

- Plugins are discovered from Claude Code's cache directory
- The enabled/disabled state is stored in `settings.json` (user or project scope)
- User scope (`~/.claude/settings.json`) sets the baseline
- Project scope (`.claude/settings.json`) stores only overrides on top of the user baseline
- Press `space`/`x` to toggle individual plugins, `a` to toggle all

**Why this matters:** Plugins add skills to Claude's context. A plugin with 20 skills that you only need for one type of project still competes for attention in every other project. Disabling irrelevant plugins per project keeps skill selection focused.

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

## OAuth credential reuse

MCP servers that use OAuth (like Google-authenticated servers) typically show as "auth required" (yellow dot) because the CLI has no credentials to present. When Claude Code has already authenticated with these servers, their OAuth tokens are stored in `~/.claude/.credentials.json`.

`cc-setup` reads these stored credentials and automatically injects them as Bearer tokens when connecting to matching servers. This works transparently for health checks and tool discovery.

**How it works:**

1. On each HTTP/SSE connection, the CLI checks if the server definition has no static `Authorization` header
2. If so, it looks for a matching entry in Claude Code's `mcpOAuth` credentials (matched by server name)
3. If found, the access token is injected as a `Bearer` header on every request
4. If the token has expired, the CLI attempts a refresh using the stored refresh token via RFC 8414 token endpoint discovery
5. Refreshed tokens are written back to `.credentials.json` so Claude Code benefits too

**Behavior on failure:** If no credentials are found, the token is expired and refresh fails, or any other error occurs, the CLI falls back to unauthenticated requests. This is the same behavior as before, you just see the yellow dot instead of green.

Servers with static `Authorization` headers configured in their definition are never wrapped with OAuth. Stdio servers are unaffected since they don't use HTTP.

## How it works

The tool reads server definitions from `~/.config/cc-setup/mcp.json` and writes to Claude Code's config files. When you select servers:

- **Selected servers** are written to the target config (`.mcp.json` or `~/.claude.json`), with the `description` field stripped
- **Inherited servers** that remain checked are not written to the local config (they are already active via a parent `.mcp.json`)
- **Unchecked servers** (that exist in the central config) are removed from the target config
- **Unchecked inherited servers** are added to `disabledMcpjsonServers` in `.claude/settings.local.json` for the current project
- **Unknown servers** (not in the central config) are left untouched

This means you can use `cc-setup` alongside manually configured servers without conflicts.

## Files

| File | Purpose |
|------|---------|
| `~/.config/cc-setup/mcp.json` | Central server registry |
| `~/.claude.json` | Claude Code user-global config |
| `.mcp.json` | Claude Code project-local config (also read from parent directories for inheritance) |
| `~/.claude/settings.local.json` | User-scoped tool permissions |
| `.claude/settings.local.json` | Project-scoped tool permissions and `disabledMcpjsonServers` |
| `~/.claude/.credentials.json` | Claude Code's OAuth tokens (read-only by this tool, except for token refresh write-back) |
