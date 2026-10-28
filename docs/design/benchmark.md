# Dashboard v3 — the Benchmark tab

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); the components are in
[components.md](components.md).

The question of the tab: **how fast is it, and does it read the whole
prompt?**

Wireframes: `design/benchmark-1280.svg` (a result and a comparison),
`design/benchmark-390.svg` (the same at 390 px),
`design/benchmark-empty-1280.svg` (nothing measured yet).

## The order of the page

1. The state headline and the actions.
2. The section `THE SUITES`: the suite cards.
3. The section `THE RESULT`: the table of the cells and the charts.
4. The section `THE COMPARISON`: the bars, the verdict chips and the
   receipt.
5. The section `THE HISTORY`: the table of the saved results.
6. The section `AN A/B RUN`: the baseline and the candidate.
7. The section `ADVANCED`.

## The widgets

| # | Widget | Data source | Refresh | Empty copy | Error copy |
|---|---|---|---|---|---|
| 1 | The state headline | `GET /api/bench` → the newest result and its best cell | on open and after a run | `NOTHING HAS BEEN MEASURED YET.` and `A benchmark measures the tokens per second of a running server. Run one below and this page fills with its numbers.` | `THE RESULTS COULD NOT BE READ.` plus `Try again` |
| 2 | The suite cards | `GET /api/bench/suites` → the id, the name, the description, the digest and the cells | on open | `There is no suite to run.` and `The built-in suites live in bench/suites/. One of them is the set of cells a pull request must pass.` | `The suites could not be read.` plus `Try again` |
| 3 | The digest of a suite | the same document, `digest` | with the card | the card shows `digest: unknown` | the card shows `digest: unknown` |
| 4 | The primary action `Run the suite` | `POST /api/bench/run` with the suite id (G7) | on demand | the button is disabled when no suite is picked | the button is disabled and reads `Try again` |
| 5 | The live progress of the run | `GET /api/bench/{id}` → `phase`, `done`, `total` and `GET /api/bench/{id}/events` (G7) | the job stream | the progress card is hidden | the progress card says `The run stopped.` and shows the last line |
| 6 | The table of the cells | `GET /api/bench` → `results[].cells[]` | on open and after a run | `No result yet.` and `Run a suite above to fill this table.` | `The result could not be read.` plus `Try again` |
| 7 | The five metric groups of a cell | the cell: `prefill`, `decode`, `ttft_s`, `itl_ms`, `output_tok_s` | with the table | `–` | `–` |
| 8 | The distribution of a metric | the same fields, `mean`, `p50`, `p95` | with the table | `–` | `–` |
| 9 | The server numbers of a cell | the cell, `server_side` | with the table | the column is hidden and the row says `the server does not publish its metrics` | the column is hidden |
| 10 | The flags of a cell | the cell: `recall`, `check`, `size_valid`, `output_valid`, `stream_incremental`, `incomplete`, `spread` | with the table | the row has no flag | the row has no flag |
| 11 | The comparison chart | `GET /api/bench/compare?a=ID&b=ID` → the cells and the verdicts | on demand | the section says `Pick two results to compare.` | `The two results measured different things.` and the reason |
| 12 | The verdict chips | the same document, the verdict of every metric | with the chart | `–` | `–` |
| 13 | The length curve | the cells of a ladder suite, `input_len` and `prefill`/`ttft_s` | with the result | the section says `This suite has no ladder. Run the long-context suite to see the curve.` | `The curve could not be drawn.` plus `Try again` |
| 14 | The history table | `GET /api/bench` → the saved results | on open and after a run | `No saved result yet.` and `Every benchmark is saved with its workload, so two results can be compared only when they measured the same thing.` | `The history could not be read.` plus `Try again` |
| 15 | The compare-to-baseline picker | the same document | on open | the picker is disabled | the picker is disabled |
| 16 | The receipt | `GET /api/bench/compare?a=ID&b=ID&format=markdown` (G11) | on demand | the button is disabled | the button is disabled |
| 17 | The A/B run | `POST /api/bench/ab` with the two arms (G12) and `GET /api/bench/{id}/events` | on demand | the section says `An A/B run measures one server, then the other, then the first again.` | the section shows the error of the run |
| 18 | The Advanced panel | `/api/bench`, `/api/bench/suites`, `/api/bench/compare` | on open | the panel holds the empty answer of the endpoint | the panel holds the error answer |

## The suite cards

One card per suite, a radio card each: a big radio, the name of the suite,
one line that says what it measures, the number of cells, the estimated
time and the digest in mono with a copy button.

The three built-in suites:

| Id | What it measures | Cells |
|---|---|---|
| `pr` | the cells a pull request must pass: 1024 prompt tokens and 512 output tokens at the levels 1, 4 and 8, and 16384 prompt tokens and 1024 output tokens at the same levels | 6 |
| `smoke` | one short cell: it proves that the server answers and says nothing about the speed | 1 |
| `long-context` | a ladder: one cell per prompt length, from 2048 to 16384 tokens, at the level 1 | 5 |

The digest is the sha256 of the canonical form of the suite. Two results of
different suites are never compared, so the digest is shown next to the
result of every run.

## The live progress

While a suite runs, the card under the suite cards holds one sentence and
one bar:

`Cell 2 of 6: 1k at concurrency 4 · 12.4 tok/s · 38 s so far.`

The bar is 10 px high with a 6 px radius, filled in `--key`. The sentence
is the plain text of the phase; the number that is not known shows `–`.

## The table of the cells

One table per result. The head is uppercase and 12 px. The columns:

| Column | The source |
|---|---|
| The group and the lens | `group`, `input_len`, `output_len` |
| The concurrency | `concurrency` and `num_prompts` |
| The prefill, per request | `prefill.req.mean` in tok/s |
| The prefill, per cell | `prefill.agg_tok_s` in tok/s |
| The decode, per request | `decode.req.mean` in tok/s |
| The decode, per cell | `decode.agg_tok_s` in tok/s |
| The time to first token | `ttft_s.mean`, `p50`, `p95` in seconds |
| The inter-token latency | `itl_ms.mean`, `p50`, `p95` in milliseconds |
| The output rate | `output_tok_s` in tok/s |
| The server numbers | `server_side.ttft_s` and `server_side.itl_ms` |

A number that was not measured shows `–`. The line above the table says how
the prefill and the decode numbers are measured, in one sentence: `The
prefill rate counts the queueing of the cell; the decode rate counts the
time in which at least one request was decoding.`

## The flags, in words

The flags of a cell are words, never colours alone:

| Field | The word | The meaning |
|---|---|---|
| `recall.passed` is false | `the reply missed the hidden fact` | the speed numbers of this cell mean nothing |
| `check.passed` is false | `the declared answer was wrong` | the cell asserts an answer, and it failed |
| `size_valid` is false | `the prompts were not the size the cell asked for` | the drift is in `input_size_percent` |
| `output_valid` is false | `the replies look like noise` | fewer than 8 % of the words are different |
| `stream_incremental` is false | `the server buffered the whole answer` | there is no decode window |
| `incomplete` is true | `a request was lost` | the numbers stay, no verdict uses them |
| `spread` above 1.05 | `the repetitions disagree` | the cell gets no verdict |

## The comparison

The section holds the bars of the five metric groups, one cluster per
group, the baseline in `--series-1` and the candidate in `--series-2`, the
value on every bar and a verdict chip over every cluster.

| Verdict | The chip |
|---|---|
| better | `▲ better` |
| the same | `– same` |
| worse | `▼ worse` |
| noisy | `! noisy` |
| no verdict | `the repetitions disagree, so this metric has no verdict` |

The section refuses a comparison that does not hold: `These two runs
measured different things.`, and it names the difference: the suite, the
digest or the workload.

The receipt is a ghost button: `Copy the receipt`. It copies the markdown
of `bernie bench compare A B --markdown` (G11), which holds the scope of the
two runs, their correctness, the numbers with the verdict of every metric
and the command that reproduces the run.

## The A/B run

The section holds two fields, `base` and `cand`, each a recipe id or the
URL of a server, and one primary button `Run the A/B`. The runs visit in
the order base, cand, cand, base, so a drift of the machine hits both arms
the same way. While it runs, the section shows the same progress sentence
as a suite run. When it ends, it holds the same comparison and the same
receipt.

## The copy of the tab

| Place | The text |
|---|---|
| The headline after a run | `THE LAST BENCHMARK REACHED 56.1 TOK/S.` |
| The lede after a run | `The pr suite measured 6 cells on 4 cards. The prefill was 2 310 tok/s and the decode 56.1 tok/s.` |
| The button | `Run the suite` |
| The section `THE COMPARISON` with nothing picked | `Pick two results to compare.` |
| The section `THE HISTORY` with nothing saved | `Every benchmark is saved with its workload, so two results can be compared only when they measured the same thing.` |
