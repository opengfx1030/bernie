# Quick start

From a bare Linux host to a model that answers. One page.

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

`up` does the doctor, builds what is missing, picks the recipe that fits
the host, starts the server and opens the dashboard. It stops at the first
step that needs root and prints that command:

```
up: stopped: No AMD GPU is visible to bernie.
  sudo modprobe amdgpu
```

Run the command as root, then run `bernie up` again.

## 3. Open the dashboard

`up` prints the address on its first line:

```
dashboard: http://127.0.0.1:7860/
```

Open it in a browser. On a host that is not only yours the address carries
a token (`http://host:7860/?token=…`). Keep it to yourself.

## 4. What you see

The Setup tab opens with one sentence about the host and one next action.
The checklist under it is the path from a bare host to a serving model.
Every step says what it changes and holds the command that does it.

| The step says | What it means | What you do |
|---|---|---|
| `blocked` | Only you can fix it | Copy the command, run it as root, press `Re-check` |
| `todo` | bernie can do it | Press the button that names the step |
| `running` | It is working now | Watch the phase and the log under the row |
| `done` | It is finished | Nothing |

## 5. First success

The last step of the path asks the server one question. From a terminal:

```bash
bernie smoke
```

It prints the time to the first token, the number of tokens and the output
speed. Then:

```bash
bernie bench        # how fast is this host?
bernie logs <name>  # what is the server saying?
bernie stop <name>  # stop the server
```

## 6. When something is wrong

Every error on the page names the command that fixes it. The same
information is in the terminal:

```bash
bernie status --steps   # the checklist of the Setup tab, as text
bernie status --json    # the same, for a script
bernie doctor           # every check with its fix
```

## Notes

- `bernie up` needs no root. It stops and asks when a step needs root.
- The dashboard is loopback only by default. `--listen 0.0.0.0:7860`
  opens it to the network and turns the token on.
- More: [install.md](install.md), [spec.md](spec.md), [mcp.md](mcp.md).
