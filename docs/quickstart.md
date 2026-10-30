# Quick start

From a bare Linux host to a model that answers.

## 1. One line, then browser

```bash
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
bernie
```

`bernie` (no command) shows the host in one screen and opens the
dashboard in the foreground. The URL is printed, for example
`http://127.0.0.1:7860/`. To reach it from another machine, start
the dashboard on the LAN: `bernie dash --listen 0.0.0.0:7860` (the
URL then carries a token; keep it private).

## 2. Or run the whole path

```bash
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

## 3. The dashboard

The dashboard binds to `127.0.0.1:7860` by default; the URL is printed
when it is up. On a host that is not only yours the address carries a
token (`http://host:7860/?token=…`). Keep it to yourself.

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
