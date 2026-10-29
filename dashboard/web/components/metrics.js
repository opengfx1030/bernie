// The numbers of a benchmark: the table of the cells, the verdict chip, the
// bar comparison and the length curve. Plain DOM and plain SVG: no innerHTML,
// no image, no dependency.
"use strict";

/** METRIC_HELP says in one sentence how the prefill and the decode numbers
 *  of a cell are measured, and what the columns of the latencies hold. */
const METRIC_HELP = "The prefill rate counts the queueing of the cell; the decode rate counts the time in which at least one request was decoding. The latencies are the median, the 95th percentile and the worst case over the answered requests; the REQUESTS column says how many answered. A number that came from repetitions carries their spread.";

/** VERDICTS are the four verdicts of a comparison: the icon, the word and the
 *  token of the colour. The word always carries the meaning. */
const VERDICTS = {
  better: { icon: "▲", word: "better", tone: "ok" },
  same: { icon: "–", word: "same", tone: "skip" },
  worse: { icon: "▼", word: "worse", tone: "fail" },
  noisy: { icon: "!", word: "noisy", tone: "warn" },
};

/** METRIC_COLS are the columns of the table of the cells. The unit of every
 *  number sits in the head of its column. Every distribution shows the
 *  median, the 95th percentile and the worst case; the mean stays in the
 *  JSON. */
const METRIC_COLS = [
  { key: "cell", label: "CELL", left: true },
  { key: "conc", label: "CONC", left: true },
  { key: "prefill_req", label: "PREFILL REQ", unit: "tok/s", sub: "p50" },
  { key: "prefill_cell", label: "PREFILL CELL", unit: "tok/s", sub: "agg" },
  { key: "decode_req", label: "DECODE REQ", unit: "tok/s", sub: "p50" },
  { key: "decode_cell", label: "DECODE CELL", unit: "tok/s", sub: "agg" },
  { key: "ttft", label: "TTFT", unit: "s", span: 3 },
  { key: "itl", label: "ITL", unit: "ms", span: 3 },
  { key: "out", label: "OUT", unit: "tok/s", sub: "agg" },
  { key: "requests", label: "REQUESTS", unit: "answered" },
  { key: "server", label: "SERVER", unit: "s / ms" },
  { key: "flags", label: "FLAGS", left: true },
];

/** COMPARE_METRICS are the five groups of a comparison, the name each one
 *  carries on the chart and the unit of its numbers. */
const COMPARE_METRICS = [
  { metric: "prefill.agg_tok_s", label: "PREFILL", unit: "tok/s" },
  { metric: "decode.agg_tok_s", label: "DECODE", unit: "tok/s" },
  { metric: "ttft_s.p50", label: "TTFT", unit: "s" },
  { metric: "itl_ms.p50", label: "ITL", unit: "ms" },
  { metric: "output_tok_s", label: "OUT", unit: "tok/s" },
];

/** benchNum formats a number of a benchmark. The thousands are separated by a
 *  space and a number that was not measured shows as "–". */
function benchNum(v, digits) {
  if (v == null || typeof v !== "number" || !isFinite(v)) return "–";
  const d = digits == null ? 1 : digits;
  const text = v.toFixed(d);
  const cut = text.indexOf(".");
  const whole = cut < 0 ? text : text.slice(0, cut);
  const frac = cut < 0 ? "" : text.slice(cut);
  return whole.replace(/\B(?=(\d{3})+(?!\d))/g, " ") + frac;
}

/** benchDist reads one number of a distribution. */
function benchDist(d, key, digits) {
  if (!d || d[key] == null) return "–";
  return benchNum(d[key], digits);
}

/** benchRate reads a rate of a cell: the mean of one request or the rate of
 *  the whole cell. */
function benchRate(rate, key, digits) {
  if (!rate) return "–";
  if (key === "agg") return rate.agg_tok_s ? benchNum(rate.agg_tok_s, digits) : "–";
  return benchDist(rate.req, key, digits);
}

/** benchDigits is the number of decimals a unit carries. */
function benchDigits(unit) {
  return unit === "s" ? 3 : 1;
}

/** SPREAD_STEADY is how close the repetitions of one cell must agree, from
 *  spec §10.2: the min/max ratio of a metric over the repetitions. Below it,
 *  the repetitions disagree and the number says little. */
const SPREAD_STEADY = 0.95;

/** benchSpreadText writes how much the repetitions of a cell disagree: the
 *  share between the slowest and the fastest repetition, with the number of
 *  repetitions. It is empty when the repetitions agree. */
function benchSpreadText(c, metric) {
  if (!metric) return null;
  const s = (c.spread || {})[metric];
  if (!s || s >= SPREAD_STEADY) return null;
  const reps = (c.reps || []).length;
  return "±" + benchNum((1 - s) * 100, 1) + " %" + (reps ? " · " + reps + " reps" : "");
}

/** cellLens names the lens of a cell: the prompt and the answer lengths. */
function cellLens(c) {
  const parts = [];
  if (c.input_len) parts.push(c.input_len + " tok in");
  if (c.output_len) parts.push(c.output_len + " tok out");
  return parts.join(" · ");
}

/** cellFlags names the flags of a cell in words. A cell without a flag has
 *  no word at all. */
function cellFlags(c) {
  const out = [];
  const add = (text, tone) => out.push({ text: text, tone: tone });
  if (c.recall && c.recall.asked > 0 && c.recall.passed < c.recall.asked) {
    add("the reply missed the hidden fact", "fail");
  }
  if (c.check && c.check.asked > 0 && c.check.passed < c.check.asked) {
    add("the declared answer was wrong", "fail");
  }
  if (c.size_valid === false) add("the prompts were not the size the cell asked for", "warn");
  if (c.output_valid === false) add("the replies look like noise", "warn");
  if (c.stream_incremental === false) add("the server buffered the whole answer", "warn");
  if (c.incomplete) add("a request was lost", "serious");
  for (const v of Object.values(c.spread || {})) {
    if (v < SPREAD_STEADY) { add("the repetitions disagree", "warn"); break; }
  }
  return out;
}

/** metricHead is the head of the table of the cells: the names of the groups,
 *  the unit of every number and the name of the statistic under it. */
function metricHead(withServer) {
  const cols = METRIC_COLS.filter((c) => c.key !== "server" || withServer);
  const top = h("tr", cols.map((c) => h("th", {
    class: c.left ? "" : "num", colspan: c.span || 1,
  }, c.label, c.unit ? h("small", c.unit) : null)));
  const bottom = [];
  for (const c of cols) {
    if (c.span && c.span > 1) {
      for (const w of ["p50", "p95", "max"]) bottom.push(h("th", { class: "num" }, w));
    } else {
      bottom.push(h("th", { class: c.left ? "" : "num" }, c.sub || ""));
    }
  }
  return h("thead", top, h("tr", bottom));
}

/** cellRow is one row of the table of the cells: the five groups of numbers,
 *  the server numbers and the flags, in words. The row of the cell that the
 *  headline names is marked, so the number of the headline can be found. */
function cellRow(c, withServer, mark) {
  const flags = cellFlags(c);
  const side = c.server_side
    ? (c.server_side.ttft_s ? benchDist(c.server_side.ttft_s, "p50", 3) + " s" : "\u2013") +
      " / " + (c.server_side.itl_ms ? benchDist(c.server_side.itl_ms, "p50", 1) + " ms" : "\u2013")
    : "the server does not publish its metrics";
  const number = (value, digits, metric) => {
    const spread = benchSpreadText(c, metric);
    return h("td.num", benchNum(value, digits), spread ? h("small.bench-spread", spread) : null);
  };
  const row = h("tr",
    h("td", h("span.bench-cell", c.group || "–"), h("small.bench-lens", cellLens(c)),
      mark && mark.group === c.group && mark.concurrency === c.concurrency
        ? h("span.bench-headline-tag", "the headline") : null),
    h("td", c.concurrency + " \u00d7 " + c.num_prompts),
    h("td.num", benchRate(c.prefill, "p50", 1)),
    number((c.prefill || {}).agg_tok_s, 1, "prefill.agg_tok_s"),
    h("td.num", benchRate(c.decode, "p50", 1)),
    number((c.decode || {}).agg_tok_s, 1, "decode.agg_tok_s"),
    number((c.ttft_s || {}).p50, 3, "ttft_s.p50"),
    number((c.ttft_s || {}).p95, 3, null),
    number((c.ttft_s || {}).max, 3, null),
    number((c.itl_ms || {}).p50, 1, "itl_ms.p50"),
    number((c.itl_ms || {}).p95, 1, null),
    number((c.itl_ms || {}).max, 1, null),
    number(c.output_tok_s, 1, "output_tok_s"),
    h("td.num", c.completed ? String(c.completed) : "\u2013"),
    withServer ? h("td.num.small", side) : null,
    h("td", flags.length
      ? flags.map((f) => h("span.bench-flag." + f.tone, f.text))
      : h("span.muted", "no flag")));
  if (mark && mark.group === c.group && mark.concurrency === c.concurrency) {
    row.classList.add("is-headline");
  }
  return row;
}

/** cellsTable is the table of the cells of a result. The mark is the cell the
 *  headline names, so the reader can find the number of the headline. */
function cellsTable(cells, mark) {
  const list = cells || [];
  if (!list.length) return emptyState("No result yet.", "Run a suite above to fill this table.", "bernie bench --suite smoke");
  const withServer = list.some((c) => c.server_side);
  return h("div.scroll", h("table.bench-table", metricHead(withServer),
    h("tbody", list.map((c) => cellRow(c, withServer, mark)))));
}

/** verdictChip is the icon and the word of a verdict, with the parts that
 *  explain it (the band the verdict used, the spread of the repetitions).
 *  The parts may be one string or a list of them; the empty ones are
 *  dropped. The colour never carries the meaning alone. */
function verdictChip(v, extra) {
  const d = VERDICTS[v];
  if (!d) return h("span.pill.verdict.skip", "–");
  const parts = Array.isArray(extra) ? extra.filter(Boolean) : (extra ? [extra] : []);
  const why = parts.length ? ", " + parts.join(", ") : "";
  return h("span.pill.verdict." + d.tone, d.icon + " " + d.word + why);
}
