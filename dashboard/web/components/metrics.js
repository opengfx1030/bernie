// The numbers of a benchmark: the table of the cells, the verdict chip, the
// bar comparison and the length curve. Plain DOM and plain SVG: no innerHTML,
// no image, no dependency.
"use strict";

/** METRIC_HELP says in one sentence how the prefill and the decode numbers
 *  of a cell are measured. */
const METRIC_HELP = "The prefill rate counts the queueing of the cell; the decode rate counts the time in which at least one request was decoding.";

/** VERDICTS are the four verdicts of a comparison: the icon, the word and the
 *  token of the colour. The word always carries the meaning. */
const VERDICTS = {
  better: { icon: "▲", word: "better", tone: "ok" },
  same: { icon: "–", word: "same", tone: "skip" },
  worse: { icon: "▼", word: "worse", tone: "fail" },
  noisy: { icon: "!", word: "noisy", tone: "warn" },
};

/** NO_VERDICT is the sentence of a metric that cannot be judged. */
const NO_VERDICT = "the repetitions disagree, so this metric has no verdict";

/** METRIC_COLS are the columns of the table of the cells. The unit of every
 *  number sits in the head of its column. */
const METRIC_COLS = [
  { key: "cell", label: "CELL", left: true },
  { key: "conc", label: "CONC", left: true },
  { key: "prefill_req", label: "PREFILL REQ", unit: "tok/s" },
  { key: "prefill_cell", label: "PREFILL CELL", unit: "tok/s" },
  { key: "decode_req", label: "DECODE REQ", unit: "tok/s" },
  { key: "decode_cell", label: "DECODE CELL", unit: "tok/s" },
  { key: "ttft", label: "TTFT S", unit: "s", span: 3 },
  { key: "itl", label: "ITL MS", unit: "ms", span: 3 },
  { key: "out", label: "OUT TOK/S", unit: "tok/s" },
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

/** The helpers below still belong to the old Dashboard tab: they move to
 *  components/charts.js when its owner rewrites that file, and they leave
 *  this file with it. They are here so that the Dashboard tab keeps working
 *  in the meantime. */

/** tile is one number with its unit, its context and a small chart. */
function tile(label, value, unit, sub, values, max, aria) {
  return h("article.tile",
    h("p.tile-l", label),
    h("p.tile-n", String(value), unit ? h("small", unit) : null),
    h("p.tile-s", sub),
    sparkSvg(values || [], max, aria));
}

/** busyRing draws the load of a card as a ring: the number in the middle and
 *  the track behind it. */
function busyRing() {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("class", "ring");
  svg.setAttribute("viewBox", "0 0 84 84");
  svg.setAttribute("role", "img");
  const bg = document.createElementNS(ns, "circle");
  bg.setAttribute("class", "ring-bg");
  const fg = document.createElementNS(ns, "circle");
  fg.setAttribute("class", "ring-fg");
  for (const c of [bg, fg]) {
    c.setAttribute("cx", "42");
    c.setAttribute("cy", "42");
    c.setAttribute("r", "36");
  }
  const circumference = 2 * Math.PI * 36;
  fg.setAttribute("stroke-dasharray", circumference + " " + circumference);
  fg.setAttribute("stroke-dashoffset", String(circumference));
  const n = document.createElementNS(ns, "text");
  n.setAttribute("class", "ring-n");
  n.setAttribute("x", "42");
  n.setAttribute("y", "44");
  const u = document.createElementNS(ns, "text");
  u.setAttribute("class", "ring-u");
  u.setAttribute("x", "42");
  u.setAttribute("y", "56");
  u.textContent = "%";
  svg.append(bg, fg, n, u);
  return svg;
}

/** setRing writes the load of a card into its ring. */
function setRing(ring, pct) {
  if (!ring) return;
  const p = Math.max(0, Math.min(100, pct || 0));
  const circumference = 2 * Math.PI * 36;
  ring.querySelector(".ring-fg").setAttribute("stroke-dashoffset", String(circumference * (1 - p / 100)));
  ring.querySelector(".ring-n").textContent = String(Math.round(p));
  ring.setAttribute("aria-label", "The card is busy " + Math.round(p) + " percent");
}

/** metricMeter is one labelled value with an optional bar. */
function metricMeter(name, bar = true, title = "", hint = "") {
  return h("div.metric", { "data-m": name },
    h("div.lbl", { title: title }, h("span", name), h("b")),
    bar ? h("div.bar", h("i")) : null,
    hint ? h("div.small.muted", hint) : null);
}

/** statusPill is a compact state label. The word carries the meaning. */
function statusPill(text, state) {
  return h("span.pill", { class: "pill " + state }, text);
}

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
    if (v > 1.05) { add("the repetitions disagree", "warn"); break; }
  }
  return out;
}

/** metricHead is the head of the table of the cells: the names of the groups
 *  and the names of the distribution under the two groups that have one. */
function metricHead(withServer) {
  const cols = METRIC_COLS.filter((c) => c.key !== "server" || withServer);
  const top = h("tr", cols.map((c) => h("th", {
    class: c.left ? "" : "num", colspan: c.span || 1,
  }, c.label, c.unit ? h("small", c.unit) : null)));
  const bottom = [];
  for (const c of cols) {
    if (c.span && c.span > 1) {
      for (const w of ["mean", "p50", "p95"]) bottom.push(h("th", { class: "num" }, w));
    } else {
      bottom.push(h("th", { class: c.left ? "" : "num" }, ""));
    }
  }
  return h("thead", top, h("tr", bottom));
}

/** cellRow is one row of the table of the cells: the five groups of numbers,
 *  the server numbers and the flags, in words. */
function cellRow(c, withServer) {
  const flags = cellFlags(c);
  const side = c.server_side
    ? (c.server_side.ttft_s ? benchDist(c.server_side.ttft_s, "p50", 3) + " s" : "–") +
      " / " + (c.server_side.itl_ms ? benchDist(c.server_side.itl_ms, "p50", 1) + " ms" : "–")
    : "the server does not publish its metrics";
  return h("tr",
    h("td", h("span.bench-cell", c.group || "–"), h("small.bench-lens", cellLens(c))),
    h("td", c.concurrency + " × " + c.num_prompts),
    h("td.num", benchRate(c.prefill, "mean", 1)),
    h("td.num", benchRate(c.prefill, "agg", 1)),
    h("td.num", benchRate(c.decode, "mean", 1)),
    h("td.num", benchRate(c.decode, "agg", 1)),
    h("td.num", benchDist(c.ttft_s, "mean", 3)),
    h("td.num", benchDist(c.ttft_s, "p50", 3)),
    h("td.num", benchDist(c.ttft_s, "p95", 3)),
    h("td.num", benchDist(c.itl_ms, "mean", 1)),
    h("td.num", benchDist(c.itl_ms, "p50", 1)),
    h("td.num", benchDist(c.itl_ms, "p95", 1)),
    h("td.num", c.output_tok_s ? benchNum(c.output_tok_s, 1) : "–"),
    withServer ? h("td.num.small", side) : null,
    h("td", flags.length
      ? flags.map((f) => h("span.bench-flag." + f.tone, f.text))
      : h("span.muted", "no flag")));
}

/** metricTable is the table of the metrics of one cell: the five groups of
 *  numbers with their distribution, the server numbers and the flags. */
function metricTable(cell) {
  const list = cell ? [cell] : [];
  const withServer = list.some((c) => c.server_side);
  return h("div.scroll", h("table.bench-table", metricHead(withServer),
    h("tbody", list.map((c) => cellRow(c, withServer)))));
}

/** cellsTable is the table of the cells of a result. */
function cellsTable(cells) {
  const list = cells || [];
  if (!list.length) return emptyState("No result yet.", "Run a suite above to fill this table.");
  const withServer = list.some((c) => c.server_side);
  return h("div.scroll", h("table.bench-table", metricHead(withServer),
    h("tbody", list.map((c) => cellRow(c, withServer)))));
}

/** verdictChip is the icon and the word of a verdict. The colour never
 *  carries the meaning alone. */
function verdictChip(v) {
  const d = VERDICTS[v];
  if (!d) return h("span.pill.verdict.skip", "–");
  return h("span.pill.verdict." + d.tone, d.icon + " " + d.word);
}

/** compareGroups picks, for every group of the comparison, the cell with the
 *  lowest concurrency: it is the cleanest measurement of the group. */
function compareGroups(cmp) {
  const out = [];
  for (const m of COMPARE_METRICS) {
    let best = null;
    for (const cell of (cmp && cmp.cells) || []) {
      const mv = (cell.metrics || []).find((x) => x.metric === m.metric);
      if (!mv) continue;
      if (!best || (cell.concurrency || 0) < (best.cell.concurrency || 0)) {
        best = { cell: cell, m: mv };
      }
    }
    if (!best) continue;
    out.push({
      metric: m.metric, label: m.label, unit: m.unit,
      base: best.m.base, cand: best.m.cand, verdict: best.m.verdict,
      higher: best.m.higher_is_better,
      cell: (best.cell.group || "") + " at " + best.cell.concurrency,
    });
  }
  return out;
}

/** svgEl builds one element of an SVG drawing. */
function svgEl(name, attrs, text) {
  const el = document.createElementNS("http://www.w3.org/2000/svg", name);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, v);
  if (text != null) el.textContent = text;
  return el;
}

/** barCompare draws the five metric groups of two runs: the baseline in the
 *  first series colour, the candidate in the second, the value on every bar
 *  and the verdict of every group. */
function barCompare(cmp) {
  const groups = compareGroups(cmp);
  if (!groups.length) return emptyState("Pick two results to compare.");
  const rowH = 52, labelW = 104, valueW = 92, width = 640;
  const plotL = labelW, plotR = width - valueW - 118;
  const plotW = Math.max(60, plotR - plotL);
  const height = 12 + groups.length * rowH;
  const svg = svgEl("svg", {
    class: "bench-bars", viewBox: "0 0 " + width + " " + height,
    role: "img", "aria-label": "The five metric groups of the two runs",
  });
  const noVerdict = [];
  groups.forEach((g, i) => {
    const top = 8 + i * rowH;
    const max = Math.max(g.base || 0, g.cand || 0, 0.0001);
    const digits = benchDigits(g.unit);
    svg.appendChild(svgEl("text", { class: "bar-label", x: 0, y: top + 12 }, g.label));
    svg.appendChild(svgEl("text", { class: "bar-sub", x: 0, y: top + 26 }, g.cell));
    [["base", g.base, "s1", top + 2], ["cand", g.cand, "s2", top + 22]].forEach(([who, value, cls, y]) => {
      const w = value > 0 ? Math.max(2, (value / max) * (plotW - 60)) : 0;
      if (w > 0) {
        svg.appendChild(svgEl("rect", {
          class: "bar-rect " + cls, x: plotL, y: y, width: w, height: 12, rx: 6,
        }));
      } else {
        svg.appendChild(svgEl("line", {
          class: "bar-missing", x1: plotL, x2: plotL + 40, y1: y + 6, y2: y + 6,
        }));
      }
      svg.appendChild(svgEl("text", {
        class: "bar-value", x: plotL + w + 8, y: y + 10,
      }, benchNum(value, digits) + " " + g.unit));
      void who;
    });
    const d = VERDICTS[g.verdict];
    const text = d ? d.icon + " " + d.word : "– no verdict";
    const cw = 14 + text.length * 6.2;
    const cx = width - cw - 4;
    svg.appendChild(svgEl("rect", {
      class: "chip-bg " + (d ? d.tone : "skip"), x: cx, y: top + 4, width: cw, height: 20, rx: 999,
    }));
    svg.appendChild(svgEl("text", {
      class: "chip-text " + (d ? d.tone : "skip"), x: cx + cw / 2, y: top + 18, "text-anchor": "middle",
    }, text));
    if (!d) noVerdict.push(g.label);
  });
  const legend = h("div.bench-legend",
    h("span", h("i.key.s1"), "baseline"),
    h("span", h("i.key.s2"), "candidate"));
  const note = noVerdict.length
    ? h("p.small.muted", NO_VERDICT + ": " + noVerdict.join(", ") + ".")
    : null;
  return h("div.bench-compare", legend, svg, note);
}

/** ladderPoints picks the cells of a ladder: one per prompt length, the one
 *  with the lowest concurrency. */
function ladderPoints(cells) {
  const best = new Map();
  for (const c of cells || []) {
    if (!c.input_len) continue;
    const key = c.input_len;
    const old = best.get(key);
    if (!old || (c.concurrency || 0) < (old.concurrency || 0)) best.set(key, c);
  }
  return Array.from(best.values()).sort((a, b) => a.input_len - b.input_len);
}

/** lengthCurve draws the prefill speed and the time to first token against
 *  the prompt length. The x axis is logarithmic. It returns null when the
 *  result has no ladder. */
function lengthCurve(cells) {
  const points = ladderPoints(cells);
  if (points.length < 2) return null;
  const width = 640, height = 260, left = 64, right = 64, top = 26, bottom = 44;
  const plotW = width - left - right, plotH = height - top - bottom;
  const lens = points.map((p) => p.input_len);
  const minL = Math.min(...lens), maxL = Math.max(...lens);
  const logMin = Math.log(minL), logSpan = Math.log(maxL) - logMin || 1;
  const prefill = points.map((p) => (p.prefill && p.prefill.agg_tok_s) || 0);
  const ttft = points.map((p) => (p.ttft_s && p.ttft_s.p50) || 0);
  const maxP = Math.max(...prefill, 1), maxT = Math.max(...ttft, 0.001);
  const x = (len) => left + plotW * ((Math.log(len) - logMin) / logSpan);
  const yP = (v) => top + plotH - (plotH * v) / maxP;
  const yT = (v) => top + plotH - (plotH * v) / maxT;
  const svg = svgEl("svg", {
    class: "bench-curve", viewBox: "0 0 " + width + " " + height,
    role: "img", "aria-label": "The prefill speed and the time to first token against the prompt length",
  });
  svg.appendChild(svgEl("line", { class: "axis", x1: left, y1: top, x2: left, y2: top + plotH }));
  svg.appendChild(svgEl("line", { class: "axis", x1: left, y1: top + plotH, x2: left + plotW, y2: top + plotH }));
  svg.appendChild(svgEl("text", { class: "axis-label", x: 0, y: top + 8 }, "prefill " + benchNum(maxP, 0) + " tok/s"));
  svg.appendChild(svgEl("text", { class: "axis-label", x: width, y: top + 8, "text-anchor": "end" }, "TTFT " + benchNum(maxT, 2) + " s"));
  const line = (values, y) => values.map((v, i) => x(lens[i]) + "," + y(v)).join(" ");
  svg.appendChild(svgEl("polyline", { class: "curve-line s1", points: line(prefill, yP) }));
  svg.appendChild(svgEl("polyline", { class: "curve-line s2", points: line(ttft, yT) }));
  points.forEach((p, i) => {
    svg.appendChild(svgEl("circle", { class: "curve-dot s1", cx: x(p.input_len), cy: yP(prefill[i]), r: 4 }));
    svg.appendChild(svgEl("circle", { class: "curve-dot s2", cx: x(p.input_len), cy: yT(ttft[i]), r: 4 }));
    svg.appendChild(svgEl("text", {
      class: "axis-label", x: x(p.input_len), y: top + plotH + 18, "text-anchor": "middle",
    }, String(p.input_len)));
    svg.appendChild(svgEl("text", {
      class: "point-label", x: x(p.input_len), y: yP(prefill[i]) - 9, "text-anchor": "middle",
    }, benchNum(prefill[i], 0)));
    svg.appendChild(svgEl("text", {
      class: "point-label s2", x: x(p.input_len), y: yT(ttft[i]) + 16, "text-anchor": "middle",
    }, benchNum(ttft[i], 2) + " s"));
  });
  const legend = h("div.bench-legend",
    h("span", h("i.key.s1"), "prefill tok/s (left)"),
    h("span", h("i.key.s2"), "TTFT p50 s (right)"));
  return h("div.bench-curve-wrap", svg, legend);
}
