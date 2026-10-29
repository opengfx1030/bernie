# Quick start

From a bare Linux host to a model that answers.

## 1. Install

```bash
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
bernie doctor
```

`bernie doctor` checks the host. Every line names the problem and the
command that fixes it. Read the `[FAIL]` lines first.

## 2. Run the whole path

```bash
bernie up
```

`up` checks the host, builds what is missing, picks the recipe that fits,
starts the server and opens the dashboard. It stops at the first step that
needs root and prints that command:

```
up: stopped: No AMD GPU is visible to bernie.
  sudo modprobe amdgpu
```

Run it as root, then run `bernie up` again.

## 3. Open the dashboard

`up` prints the address on its first line: `dashboard: http://127.0.0.1:7860/`.
Open it in a browser. On a host that is not only yours the address carries
a token (`http://host:7860/?token=…`). Keep it to yourself.

## 4. What you see

The Setup tab opens with one sentence about the host and one next action.
The checklist under it is the path to a serving model. Every step says what
it changes and holds the command that does it.

| The step says | What it means | What you do |
|---|---|---|
| `blocked` | Only you can fix it | Copy the command, run it as root, press `Re-check` |
| `todo` | bernie can do it | Press the button that names the step |
| `running` | It is working now | Watch the phase and the log under the row |
| `done` | It is finished | Nothing |

## 5. First success

```bash
bernie smoke
```

It sends one prompt to the server and prints the time to the first token,
the number of tokens and the output speed. Then `bernie bench` (how fast is
this host?), `bernie logs <name>` (what is the server saying?) and
`bernie stop <name>`.

## 6. When something is wrong

Every error on the page names the command that fixes it. The same is in the
terminal: `bernie status --steps` (the checklist as text), `bernie status
--json` (the same for a script), `bernie doctor` (every check with its fix).

More: [install.md](install.md), [spec.md](spec.md), [mcp.md](mcp.md).
