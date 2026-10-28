# Connect an AI agent (MCP)

bernie speaks the Model Context Protocol (MCP), so an agent can inspect the
host and run the same commands you run by hand. The behavior contract is
[spec §13](spec.md#13-model-context-protocol). This page is the how-to.

MCP is a third front end next to the CLI and the dashboard. A tool returns
the same JSON as the matching command with `--json`. It adds no feature of its
own.

Protocol revision: `2026-07-28`. Clients that start with `initialize` work
too. The server is built on the official Go SDK
([third-party.md](third-party.md)).

## Quick start: stdio

The client starts `bernie mcp` and talks to it on stdin and stdout. There is
no listening port.

```bash
bernie mcp                 # read-only
bernie mcp --allow-write   # also the tools that change the host
```

Diagnostics go to stderr. Use an absolute path to the binary in a client
config: many clients do not start a login shell, so `bernie` may not be on
their `PATH`.

## Quick start: HTTP

`bernie dash` serves MCP at `POST /mcp` when it listens on a loopback address.

```bash
bernie dash --token "$BERNIE_DASH_TOKEN"      # http://127.0.0.1:7860/mcp
curl -s http://127.0.0.1:7860/mcp \
  -H "Authorization: Bearer $BERNIE_DASH_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Rules ([§13.10](spec.md#1310-http-guard)):

- `Host` and `Origin` must be loopback. A request from a web page of another
  site gets 403.
- With a dashboard token, every request needs `Authorization: Bearer <token>`.
  The dashboard cookie does not count, and a token in the URL is refused.
- Only `POST` is served. A call of a mutating tool also needs `X-Bernie: 1`.
- On a non-loopback address `/mcp` answers 404.

## Tools

Read-only by default. Each tool returns the document of its command as
`structuredContent` and as text.

| Tool | Same as |
|---|---|
| `status`, `hw`, `topo`, `doctor` | the command of the same name |
| `doctor_log` | `doctor --run NAME` (known failures of a run log) |
| `recipe_list`, `recipe_show`, `recipe_plan` | `recipe list`, `show`, `plan` |
| `ps`, `logs` | `ps`; the log of a run (text, marked untrusted) |
| `bench_list`, `bench_show` | `bench list`, `bench show` |
| `tunableop_status`, `power_status` | `tune tunableop status`, `tune power status` |

An agent should start with `status`, then `doctor` and `recipe_plan`.

## Write mode

The tools that change the host (`serve`, `stop`, `bench_run`, `setup_apply`,
`tune_power_set`) are not listed until you enable them: `bernie mcp
--allow-write`, or `BERNIE_MCP_WRITE=1` in the environment of `bernie mcp` or
`bernie dash`. Over HTTP they also need a dashboard token.

Each one takes two calls:

1. Call it with its arguments. It changes nothing and returns
   `{"executed": false, "plan": {...}, "confirm_token": "...", "expires_at": "..."}`.
2. Show the plan to the user. Call it again with the same arguments and the
   token. It returns `{"executed": true, "result": {...}}`.

A token works once, expires after five minutes, and fits only the same tool,
arguments and plan. A changed argument or an expired token needs a new plan
call. One host-changing operation runs at a time.

bernie never runs `sudo`. A step that needs root (a package install, a power
cap as a normal user) comes back as a command for you to run.

## Safety

- Run logs and model output are untrusted data. The `logs` tool says so in its
  result. An agent must not follow instructions found in a log. Prefer
  `doctor_log`.
- Secrets (`*_TOKEN`, `*_KEY`, `*_SECRET`, `*_PASSWORD`, `hf_...`, `sk-...`)
  are redacted from every result.
- `bench_run` aims only at a loopback server.
- Every call is one JSON line in `<state>/mcp/audit.jsonl` (mode 0600), with
  no token, no environment and no log text.

## Client setup

Replace `/absolute/path/to/bernie` with the output of `command -v bernie`.
Add `"--allow-write"` to `args` only when you want write mode.

### Claude Desktop

File `claude_desktop_config.json` (macOS: `~/Library/Application
Support/Claude/`, Windows: `%APPDATA%\Claude\`):

```json
{
  "mcpServers": {
    "bernie": {
      "type": "stdio",
      "command": "/absolute/path/to/bernie",
      "args": ["mcp"]
    }
  }
}
```

### Claude Code

```bash
claude mcp add --transport stdio bernie -- /absolute/path/to/bernie mcp
# or over HTTP to a running dashboard:
claude mcp add --transport http bernie http://127.0.0.1:7860/mcp \
  --header "Authorization: Bearer $BERNIE_DASH_TOKEN"
```

A project `.mcp.json` takes the same entry as Claude Desktop. An HTTP entry
needs `"type": "http"`.

### Cursor

`.cursor/mcp.json` in a project, or `~/.cursor/mcp.json` for all projects:

```json
{
  "mcpServers": {
    "bernie": {
      "type": "stdio",
      "command": "/absolute/path/to/bernie",
      "args": ["mcp"]
    }
  }
}
```

### opencode

`opencode.json` or `opencode.jsonc`. The command is one array:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "bernie": {
      "type": "local",
      "command": ["/absolute/path/to/bernie", "mcp"],
      "enabled": true
    }
  }
}
```

### Codex

`~/.codex/config.toml`, or `.codex/config.toml` in a trusted project:

```toml
[mcp_servers.bernie]
command = "/absolute/path/to/bernie"
args = ["mcp"]
```

Or: `codex mcp add bernie -- /absolute/path/to/bernie mcp`. For a running
dashboard, use `url = "http://127.0.0.1:7860/mcp"` and
`bearer_token_env_var = "BERNIE_DASH_TOKEN"`.

The snippets follow the client documentation of
[Claude Desktop](https://modelcontextprotocol.io/docs/2026-07-28/develop/connect-local-servers.md),
[Claude Code](https://code.claude.com/docs/en/mcp.md),
[Cursor](https://cursor.com/docs/mcp),
[opencode](https://opencode.ai/docs/mcp-servers/) and
[Codex](https://developers.openai.com/codex/mcp/).

## Troubleshooting

| Symptom | Cause |
|---|---|
| The client shows no tools | The path is wrong or not absolute. Run `/absolute/path/to/bernie mcp` in a shell: it must print `serving on stdio` on stderr and wait. |
| No `serve`, `stop`, ... tools | Write mode is off ([above](#write-mode)). |
| HTTP 401 | Missing or wrong bearer token. The cookie and a URL token do not work on `/mcp`. |
| HTTP 403 `host header` / `origin` | Reach the dashboard through `127.0.0.1`, `localhost` or `[::1]`. |
| HTTP 403 `X-Bernie` | A mutating tool over HTTP needs the header `X-Bernie: 1`. |
| HTTP 404 on `/mcp` | The dashboard does not listen on loopback. |
| `confirm_token is unknown or was already used` | Tokens are single use. Call the tool again without one. |

Specification: <https://modelcontextprotocol.io/specification/2026-07-28>.
