# Dashboard v3 — the Logs tab

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); the components are in
[components.md](components.md).

The question of the tab: **what did the server say?**

Wireframes: `design/logs-1280.svg` (a live tail with findings),
`design/logs-390.svg` (the same at 390 px),
`design/logs-empty-1280.svg` (no log yet).

## The order of the page

1. The state headline and the actions.
2. The section `THE RUN`: the picker of the run.
3. The section `THE KNOWN PROBLEMS`: the findings of the log doctor.
4. The section `THE LOG`: the tail, the filter and the search.
5. The section `ADVANCED`.

The known problems sit above the tail: a reader who opens the tab sees what
is wrong before the noise.

## The widgets

| # | Widget | Data source | Refresh | Empty copy | Error copy |
|---|---|---|---|---|---|
| 1 | The state headline | `GET /api/runs` → the selected run and its state | on open and on every change of the run | `NO LOG YET.` and `Start the server first, then follow its log here.` plus the command `bernie serve <recipe>` | `THE LOG COULD NOT BE READ.` plus `Try again` |
| 2 | The picker of the run | `GET /api/runs` → the run records (the name, the state, the age) | on open and every 15 s | `No run has ever served on this host.` and `A run is a model server on this host: it loads the model and then answers requests.` plus the command `bernie serve <recipe>` | `The runs could not be read.` plus `Try again` |
| 3 | The findings of the log doctor | `GET /api/runs/{name}/findings` (G6); the findings of `GET /api/runs` for an exited run | on open, on `Re-scan` and after every new line that matches | `No known failure in this log.` and `bernie knows the failures of this server; none of them is in this log.` | `The findings could not be read.` plus `Try again` |
| 4 | The fix of a finding | the finding: `fix`, `doc` | with the findings | a finding without a fix shows the cause and the link only | the same |
| 5 | The tail of the log | `GET /api/runs/{name}/logs?tail=200`, a server-sent stream of one line per event | the stream | `No log yet.` and `Start the server first, then follow its log here.` | the stream sends an `error` event; the card says `The stream stopped: <reason>` and the button `Reconnect` |
| 6 | The level filter | the level of each line, see below | local, no request | the filter is hidden when there is no line | the filter is hidden |
| 7 | The search | the text of each line, a case-insensitive substring | local, no request | the search box is disabled when there is no line | the search box is disabled |
| 8 | The number of the lines | the lines in memory | with the tail | `0 lines` | `–` |
| 9 | The follow button | the scroll position of the tail | local | the button is hidden | the button is hidden |
| 10 | The download | the lines in memory, written as a file | on demand | the button is disabled | the button is disabled |
| 11 | The jump to a finding | the line number of the finding | on demand | the button is hidden when there is no finding | the button is hidden |
| 12 | The Advanced panel | `/api/runs`, `/api/runs/{name}/logs` | on open | the panel holds the empty answer of the endpoint | the panel holds the error answer |

## The picker of the run

A row of radio cards, one per run, each with the name of the run, its state
pill and its age. The newest run is picked when the tab opens. A run that
is `starting` shows its phase sentence under its name.

The run picker is the only selection of the tab; the tail follows it.

## The tail of the log

| Part | Rule |
|---|---|
| The text | 12.5 px `--font-mono` on a near-black surface (`--surface-2` in the dark palette, `#111418` in the light one), `pre-wrap`, 340 px high, and it scrolls |
| The levels | the colour of a line comes from its level; the level is also written in the gutter, so the meaning never depends on the colour |
| The follow | the tail follows the end of the log until the reader scrolls up; the button `Follow` comes back when it does |
| The empty tail | `No log yet.` and the command that starts a server |

The levels are decided by the reader, with the same rules everywhere:

| Level | When a line is one |
|---|---|
| `error` | the line holds `ERROR`, `Traceback`, `RuntimeError`, `CUDA error` or `Error:` |
| `warn` | the line holds `WARNING` or `WARN` and no error mark |
| `info` | every other line |

A line that a finding of the log doctor points at is marked as an error
whatever it holds. The rules are in one place in the code, so the page and
the doctor never disagree about a line.

## The known problems

One card per finding, in the order of the log:

| Part | What it holds |
|---|---|
| The head | the pill (`fail` or `warn`) and the id of the rule |
| The cause | one sentence: what the log said and what it means |
| The fix | the command, in a command block with a copy button |
| The doc | the link to the rule |
| The jump | the button `Go to the line` scrolls the tail to that line and marks it for a second |

The findings of a run that is still alive are scanned on demand: the button
`Re-scan` runs the log doctor again. The scan of a log that does not change
costs nothing, so the page scans it once when the run is picked.

## The download

The button `Download the log` writes the lines that are in memory to a file
named `<run>-<YYYY-MM-DD-HHMMSS>.log`, one line per line, as plain text. It
does not ask the server: the tail already holds the lines. The button is
disabled when the tail is empty.

## The state headline

| State | The headline | The lede |
|---|---|---|
| A run is serving | `THE SERVER IS RUNNING.` | `The tail follows the log of flashnext. 1 240 lines so far.` |
| A run is starting | `THE SERVER IS STARTING.` | the phase sentence of the run |
| A run is gone | `THE RUN ENDED 12 MIN AGO.` | `The log holds the last words of the server. The known problems are above.` |
| A finding is known | `THE SERVER FAILED FOR A KNOWN REASON.` | `The log matched the rule engine.kv-too-small. The fix is above.` |
| No run | `NO LOG YET.` | `Start the server first, then follow its log here.` |

## The copy of the tab

| Place | The text |
|---|---|
| The filter | `all`, `info`, `warn`, `error` |
| The search | the placeholder `Search the log` |
| The counter | `1 240 lines · 12 shown` |
| The follow button | `Follow` and `Paused` |
| The download button | `Download the log` |
| The jump button | `Go to the line` |
| The button of the findings card | `Re-scan` |
