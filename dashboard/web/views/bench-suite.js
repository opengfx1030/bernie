"use strict";
// ---------- Bench suites, the verdict and the length curve ----------

/** benchSuiteSelect is the suite picker of the run form. */
function benchSuiteSelect() {
  const sel = h("select", { name: "suite", id: "b-suite" });
  put(sel, h("option", { value: "" }, "no suite (the cells of the form)"));
  return sel;
}

/** benchSuiteCells counts the cells of a suite. */
function benchSuiteCells(s) {
  let n = 0;
  for (const g of (s && s.groups) || []) n += (g.concurrency || []).length;
  return n;
}

/** benchLoadSuites fills the suite picker with the built-in suites. */
async function benchLoadSuites(sel) {
  try {
    const list = await api("/api/bench/suites");
    for (const s of list || []) {
      put(sel, h("option", { value: s.id }, `${s.id} — ${s.name} (${benchSuiteCells(s)} cells)`));
    }
  } catch (e) {
    // The form still runs a plain benchmark without a suite.
  }
}

/** benchResultCard is the content of the result card: the numbers of the
 *  run, the suite line, the measurement gates, the context-length curve
 *  and the compare control. The metric help, the cell tables and the
 *  recall note come from bench.js. */
function benchResultCard(card, r) {
  put(card,
    h("h3", "Result " + r.id),
    h("div.small.muted", `${r.model} @ ${r.url}`),
    benchSuiteLine(r),
    h("div.small.muted", METRIC_HELP),
    cellsView(r.cells),
    recallNote(r.cells),
    benchFlags(r.cells),
    benchCurve(r.cells),
    benchCompareControl(r));
}

/** benchSuiteLine names the suite of a result. */
function benchSuiteLine(r) {
  if (!r || !r.suite) return null;
  return h("div.small.muted", `Suite ${r.suite.id} · digest ${String(r.suite.digest || "").slice(0, 12)} · version ${r.suite.version}`);
}

/** benchFlags names the measurement gates of the cells that failed. */
function benchFlags(cells) {
  const bad = (cells || []).filter((c) => c.size_valid === false || c.output_valid === false || c.stream_incremental === false);
  if (!bad.length) return null;
  const parts = [];
  for (const c of bad) {
    const why = [];
    if (c.size_valid === false) why.push(`prompt ${num(c.input_size_percent, 1)} % off the requested size`);
    if (c.output_valid === false) why.push("the replies repeat one word");
    if (c.stream_incremental === false) why.push("the server buffered the whole answer");
    parts.push(`${c.group ? c.group + " " : ""}c=${c.concurrency}: ${why.join(", ")}`);
  }
  return h("div.finding",
    h("div", h("b", "The measurement is not a speed number")),
    h("div.small.muted", parts.join(" · ")));
}

/** benchVerdict is the table of the verdict of two runs. */
function benchVerdict(v) {
  if (!v) return null;
  const rows = [];
  for (const cv of v.cells || []) {
    for (const m of cv.metrics || []) {
      rows.push(h("tr",
        h("td", `${cv.group ? cv.group + " " : ""}c=${cv.concurrency}`),
        h("td", m.label),
        h("td.num", num(m.base, 2)),
        h("td.num", num(m.cand, 2)),
        h("td.num", (m.delta_pct > 0 ? "+" : "") + num(m.delta_pct, 1) + " %"),
        h("td", h("span.chip." + m.verdict, m.verdict))));
    }
  }
  const head = h("div",
    h("h3", "Verdict"),
    h("div.small.muted", `${v.summary.better} better, ${v.summary.same} same, ${v.summary.worse} worse, ${v.summary.noisy} noisy · recall ${v.summary.recall_ok ? "ok" : "failed"}`));
  if (!rows.length) {
    return h("div", head, h("div.empty", "No metric could be compared."));
  }
  const notes = (v.reasons || []).map((r) => h("div.small.muted", r));
  return h("div", head, h("div.scroll", h("table",
    h("thead", h("tr", h("th", "cell"), h("th", "metric"), h("th.num", "base"), h("th.num", "cand"), h("th.num", "delta"), h("th", "verdict"))),
    h("tbody", rows))), ...notes);
}

/** benchCompareControl lets the operator compare the shown result with a
 *  saved one. It returns the block; the verdict lands in the card. */
function benchCompareControl(r, onVerdict) {
  const sel = h("select", { id: "b-compare" });
  const out = h("div");
  const btn = h("button", { type: "button" }, "Compare");
  btn.addEventListener("click", async () => {
    const id = sel.value;
    if (!id || !r || !r.id) return;
    btn.disabled = true;
    put(out, loading());
    try {
      const v = await api(`/api/bench/compare?a=${encodeURIComponent(r.id)}&b=${encodeURIComponent(id)}`);
      put(out, benchVerdict(v));
    } catch (e) {
      put(out, failed(e));
    }
    btn.disabled = false;
  });
  const block = h("div.row",
    h("label", { for: "b-compare" }, "Compare to"), sel, btn);
  api("/api/bench").then((list) => {
    const results = (Array.isArray(list) ? list : []).slice().sort((a, b) => new Date(b.time) - new Date(a.time));
    for (const s of results) {
      if (r && s.id === r.id) continue;
      put(sel, h("option", { value: s.id }, `${s.id} · ${s.tag || s.model}`));
    }
  }).catch(() => {});
  return h("div", block, out);
}

/** benchCurve draws the context-length curve of a result: the prefill
 *  speed and the time to first token of every group. It is plain SVG: no
 *  library and no innerHTML. */
function benchCurve(cells) {
  const groups = [];
  const seen = new Set();
  for (const c of cells || []) {
    const key = c.group || "";
    const len = c.input_len || 0;
    if (!len || seen.has(key)) continue;
    seen.add(key);
    // The lowest concurrency of a group is the cleanest measurement.
    const best = groups.find((g) => g.group === key);
    if (!best) groups.push({ group: key, len, cell: c });
    else if ((c.concurrency || 0) < (best.cell.concurrency || 0)) best.cell = c;
  }
  if (groups.length < 2) return null;
  groups.sort((a, b) => a.len - b.len);
  const prefill = groups.map((g) => (g.cell.prefill || {}).agg_tok_s || 0);
  const ttft = groups.map((g) => ((g.cell.ttft_s || {}).p50 || 0) * 1000);
  const maxP = Math.max(...prefill, 1);
  const maxT = Math.max(...ttft, 1);
  const w = 360, hgt = 190, left = 46, right = 46, top = 26, bottom = 34;
  const plotW = w - left - right, plotH = hgt - top - bottom;
  const x = (i) => left + (groups.length === 1 ? plotW / 2 : (plotW * i) / (groups.length - 1));
  const yP = (v) => top + plotH - (plotH * v) / maxP;
  const yT = (v) => top + plotH - (plotH * v) / maxT;
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", `0 0 ${w} ${hgt}`);
  svg.setAttribute("class", "curve");
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", "The prefill speed and the time to first token of every group");
  const add = (name, attrs, text) => {
    const el = document.createElementNS("http://www.w3.org/2000/svg", name);
    for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, v);
    if (text != null) el.textContent = text;
    svg.appendChild(el);
    return el;
  };
  // The axes.
  add("line", { x1: left, y1: top, x2: left, y2: top + plotH, class: "axis" });
  add("line", { x1: left, y1: top + plotH, x2: left + plotW, y2: top + plotH, class: "axis" });
  add("text", { x: 4, y: top + 8, class: "axis-label" }, `prefill ${num(maxP, 0)} tok/s`);
  add("text", { x: w - 4, y: top + 8, class: "axis-label", "text-anchor": "end" }, `TTFT ${num(maxT, 0)} ms`);
  // The series.
  const points = (vals, y) => vals.map((v, i) => `${x(i)},${y(v)}`).join(" ");
  add("polyline", { points: points(prefill, yP), class: "series-1" });
  add("polyline", { points: points(ttft, yT), class: "series-2" });
  groups.forEach((g, i) => {
    add("circle", { cx: x(i), cy: yP(prefill[i]), r: 4, class: "series-1" });
    add("circle", { cx: x(i), cy: yT(ttft[i]), r: 4, class: "series-2" });
    add("text", { x: x(i), y: hgt - 16, class: "axis-label", "text-anchor": "middle" }, g.group || `${g.len}`);
    add("text", { x: x(i), y: hgt - 4, class: "axis-label", "text-anchor": "middle" }, `${g.len} tok`);
  });
  const legend = h("div.small.muted", "● prefill tok/s (left) · ● TTFT p50 ms (right) · the lowest concurrency of every group");
  return h("div", h("h3", "Context length"), svg, legend);
}
