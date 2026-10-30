# Quick start

From a bare Linux host to a model that answers.

## 1. One line, then browser

```bash
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh -s -- --start
```

The installer puts `bernie` on `$PATH`, starts the dashboard as a
background service and prints the URL to open. The token (when the
listen address is not loopback) is persisted at
`~/.local/state/bernie/dashboard/token` and is part of the URL: anyone
with the URL can drive the host, so keep it private.

The dashboard is reachable while vLLM is still loading: the Setup tab
shows the next step and the run as `starting`. The host is checked the
first time the dashboard reads it; the Setup tab runs the steps bernie
can do without root and prints the ones that need root.

## 2. Or run the whole path

```bash
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
bernie up
```

`bernie up` checks the host, builds what is missing, picks the recipe
that fits, starts the server and opens the dashboard. It stops at the
first step that needs root and prints that command: run it as root,
then run `bernie up` again.

```
up: stopped: No AMD GPU is visible to bernie.
  sudo modprobe amdgpu
```

## 3. Open the dashboard

`up` prints the address of the dashboard when it is up:
`dashboard: http://127.0.0.1:7860/`. Open it in a browser. On a host that is
not only yours the address carries a token (`http://host:7860/?token=…`).
Keep it to yourself.

## 4. What you see

The Setup tab opens with one sentence about the host and one next action.
The setup it runs leaves a record: `bernie setup report` prints every step,
the time it took, its log and the evidence of the venv check.
The checklist under it is the path to a serving model: every step says what
it changes and holds the command that does it.

| The step says | What it means | What you do |
|---|---|---|
| `blocked` | Only you can fix it | Copy the command, run it as root, press `Re-check` |
| `todo` | bernie can do it | Press `Set up this box`: it does every step bernie can do here, in one run |
| `running` | It is working now | Watch the phase and the log under the row; `Stop` ends the run |
| `done` | It is finished | Nothing |

One step of the path may need root. The run stops there, shows the exact
command, and waits for `Re-check`.

## 5. First success

```bash
bernie smoke
```

It sends one prompt to the server and prints the URL, the model and the answer. Then `bernie bench` (how fast is this host? it prints the time to the first token, the number of tokens and the output speed), `bernie logs <name>` (what is the server saying?) and `bernie stop <name>`.

When something is wrong, the page and the terminal name the command that fixes it: `bernie status --steps`, `bernie status --json`, `bernie doctor`. `bernie --root DIR` reads another host tree (a captured host): use it for a demo or a test. `BERNIE_ROOT=DIR` does the same for a process that cannot take a flag (the MCP server, a service); the CLI, the dashboard and the MCP server all read that tree.
