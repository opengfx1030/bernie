// The drawings of the Benchmark tab: the comparison of two runs, cell by
// cell, and the curve of the prompt length. Plain SVG and plain DOM: no
// innerHTML, no image, no dependency. Every chart has a text form: a details
// element with the same numbers in a table.
"use strict";

/** NO_VERDICT is the sentence of a metric that cannot be judged. */
const NO_VERDICT = "the repetitions disagree, so this metric has no verdict";

/** svgEl builds one element of an SVG drawing. */
function svgEl(name, attrs, text) {
  const el = document.createElementNS("http://www.w3.org/2000/svg", name);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, v);
  if (text != null) el.textContent = text;
  return el;
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

/* ---------- the comparison, cell by cell ---------- */

/** compareCellName names a cell of a comparison: the group of the suite and
 *  the concurrency of the cell. */
function compareCellName(cell) {
  return ((cell && cell.group) || "–") + " at concurrency " + ((cell && cell.concurrency) || 0);
}

/** compareSpread reads how much the repetitions of one metric of one cell
 *  disagree: the ratio of the slowest to the fastest repetition. */
function compareSpread(cmp, cell, metric) {
  let spread = 0;
  for (const arm of [(cmp && cmp.a), (cmp && cmp.b)]) {
    const c = ((arm && arm.cells) || []).find((x) => x.group === cell.group && x.concurrency === cell.concurrency);
    const s = c && c.spread && c.spread[metric];
    if (s && s > spread) spread = s;
  }
  return spread;
}

/** compareBandText writes the band a verdict used: the noise band of the
 *  suite, in percent. The backend widens the band of the suite with the
 *  spread of the repetitions, so the band already carries the noise. */
function compareBandText(m) {
  return "band " + (m.band ? benchNum(m.band * 100, 1) + " %" : "–");
}

/** compareSpreadText writes how much the repetitions of a cell disagree: the
 *  share between the slowest and the fastest repetition. */
function compareSpreadText(spread) {
  if (!spread || spread >= SPREAD_STEADY) return null;
  return "spread ±" + benchNum((1 - spread) * 100, 1) + " %";
}

/** compareChipText writes the whole text of the verdict of one metric: the
 *  word, the band it used and, when the repetitions disagree, their spread. */
function compareChipText(m, spread) {
  const d = VERDICTS[m.verdict];
  const word = d ? d.icon + " " + d.word : "– no verdict";
  const parts = [compareBandText(m)];
  const spreadText = compareSpreadText(spread);
  if (spreadText) parts.push(spreadText);
  return word + ", " + parts.join(", ");
}

/** compareChipWidth is the width the chip of one verdict takes. */
function compareChipWidth(text) {
  return 16 + text.length * 6.3;
}

/** compareChip draws the verdict of one metric: a pill with the word and the
 *  band. It returns the width it took. */
function compareChip(svg, x, y, text, tone) {
  const w = compareChipWidth(text);
  svg.appendChild(svgEl("rect", {
    class: "chip-bg " + tone, x: x, y: y, width: w, height: 20, rx: 999,
  }));
  svg.appendChild(svgEl("text", {
    class: "chip-text " + tone, x: x + w / 2, y: y + 14, "text-anchor": "middle",
  }, text));
  return w;
}

/** compareBlock draws the comparison of one cell: the five metric groups of
 *  that cell, their bars indexed to the baseline, the change in percent and
 *  the verdict of each. The bars of every row of every cell share one scale
 *  (the baseline is 100), so the size of a change stays visible and two cells
 *  can be read against each other. */
function compareBlock(cmp, cell, scaleMax) {
  const rows = [];
  for (const m of COMPARE_METRICS) {
    const v = (cell.metrics || []).find((x) => x.metric === m.metric);
    if (v) rows.push({ m: m, v: v });
  }
  if (!rows.length) return null;
  const width = 760, rowH = 76, barX = 110, plotW = 200, valueX = 320;
  const height = 6 + rows.length * rowH;
  const svg = svgEl("svg", {
    class: "bench-bars", viewBox: "0 0 " + width + " " + height, role: "img",
    "aria-label": "The five metric groups of the cell " + compareCellName(cell) + ", indexed to the baseline",
  });
  // The bars are indexed to the baseline: the baseline is 100 and every row
  // of every cell of the chart is drawn on the same scale, so a change of
  // 5 % and a change of 50 % do not look the same.
  const scale = Math.max(100, scaleMax || 100);
  const px = (index) => (Math.max(0, Math.min(scale, index)) / scale) * plotW;
  const baseX = barX + px(100);
  rows.forEach((row, i) => {
    const y = 6 + i * rowH;
    const m = row.m, v = row.v;
    const spread = compareSpread(cmp, cell, m.metric);
    const tone = (VERDICTS[v.verdict] || {}).tone || "skip";
    const digits = benchDigits(m.unit);
    svg.appendChild(svgEl("text", { class: "bar-label", x: 0, y: y + 40 }, m.label));
    svg.appendChild(svgEl("line", {
      class: "bar-index", x1: baseX, x2: baseX, y1: y + 22, y2: y + 60,
    }));
    svg.appendChild(svgEl("rect", {
      class: "bar-rect s1", x: barX, y: y + 26, width: px(100), height: 10, rx: 5,
    }));
    const ratio = v.base > 0 ? v.cand / v.base : 0;
    const w = px(ratio * 100);
    if (w > 0) {
      svg.appendChild(svgEl("rect", { class: "bar-rect s2", x: barX, y: y + 44, width: w, height: 10, rx: 5 }));
    } else {
      svg.appendChild(svgEl("line", { class: "bar-missing", x1: barX, x2: barX + 40, y1: y + 49, y2: y + 49 }));
    }
    if (i === 0) {
      // The two arms are named on the marks themselves, not only in the key.
      svg.appendChild(svgEl("text", { class: "bar-sub", x: baseX + 6, y: y + 35 }, "baseline"));
      svg.appendChild(svgEl("text", { class: "bar-sub", x: barX + w + 6, y: y + 53 }, "candidate"));
    }
    svg.appendChild(svgEl("text", {
      class: "bar-value", x: valueX, y: y + 30,
    }, benchNum(v.base, digits) + " → " + benchNum(v.cand, digits) + " " + m.unit));
    const d = v.delta_pct;
    svg.appendChild(svgEl("text", {
      class: "bar-delta " + tone, x: valueX, y: y + 50,
    }, (d > 0 ? "+" : d < 0 ? "−" : "") + benchNum(Math.abs(d), 1) + " %"));
    const text = compareChipText(v, spread);
    compareChip(svg, width - 4 - compareChipWidth(text), y + 30, text, tone);
  });
  return svg;
}

/** compareScale is the biggest index the chart has to hold: the largest
 *  change of any metric of any cell, rounded up. Every bar of the chart is
 *  drawn on that one scale. */
function compareScale(cmp) {
  let max = 100;
  for (const cell of (cmp && cmp.cells) || []) {
    for (const v of cell.metrics || []) {
      const idx = v.base > 0 ? (v.cand / v.base) * 100 : 100;
      if (idx > max) max = idx;
    }
  }
  return Math.ceil(max / 25) * 25;
}

/** compareTable is the text form of the comparison: every cell, every metric,
 *  the two numbers, the change and the verdict. */
function compareTable(cmp) {
  const rows = [];
  for (const cell of (cmp && cmp.cells) || []) {
    for (const m of COMPARE_METRICS) {
      const v = (cell.metrics || []).find((x) => x.metric === m.metric);
      if (!v) continue;
      const spread = compareSpread(cmp, cell, m.metric);
      const digits = benchDigits(m.unit);
      const d = v.delta_pct;
      rows.push(h("tr",
        h("td", compareCellName(cell)),
        h("td", m.label),
        h("td.num", benchNum(v.base, digits) + " " + m.unit),
        h("td.num", benchNum(v.cand, digits) + " " + m.unit),
        h("td.num", (d > 0 ? "+" : d < 0 ? "−" : "") + benchNum(Math.abs(d), 1) + " %"),
        h("td", verdictChip(v.verdict, [compareBandText(v), compareSpreadText(spread)]))));
    }
  }
  if (!rows.length) return null;
  return h("details.bench-fallback",
    h("summary", "The numbers of the comparison"),
    h("div.scroll", h("table.bench-table",
      h("thead", h("tr",
        h("th", "CELL"), h("th", "METRIC"),
        h("th.num", "BASELINE"), h("th.num", "CANDIDATE"), h("th.num", "CHANGE"), h("th", "VERDICT"))),
      h("tbody", rows))));
}

/** compareChart draws the comparison of two runs: one block per cell, the
 *  five metric groups of that cell in each block, and the numbers in a
 *  table under the chart. */
function compareChart(cmp) {
  const blocks = [];
  const noVerdict = [];
  const scale = compareScale(cmp);
  for (const cell of (cmp && cmp.cells) || []) {
    const svg = compareBlock(cmp, cell, scale);
    if (!svg) continue;
    blocks.push(h("div.bench-compare-cell",
      h("p.bench-compare-cellhead", compareCellName(cell)),
      svg));
    for (const m of COMPARE_METRICS) {
      const v = (cell.metrics || []).find((x) => x.metric === m.metric);
      if (v && !VERDICTS[v.verdict]) noVerdict.push(m.label);
    }
  }
  if (!blocks.length) return emptyState("Pick two results to compare.",
    "Two saved results side by side, with a verdict on every cell.", "bernie bench list");
  const legend = h("div.bench-legend",
    h("span", h("i.key.s1"), "baseline"),
    h("span", h("i.key.s2"), "candidate"),
    h("span", "indexed: the baseline is 100, the scale ends at " + scale));
  const note = noVerdict.length
    ? h("p.small.muted", NO_VERDICT + ": " + noVerdict.join(", ") + ".")
    : null;
  return h("div.bench-compare", legend, blocks, note, compareTable(cmp));
}

/* ---------- the curve of the prompt length ---------- */

/** curveSmallMultiples draws the prompt length as two stacked charts that
 *  share the log x-axis: the prefill speed above, the time to the first
 *  token below. One measure per chart, so the two lines cannot be read as
 *  one curve. */
function curveSmallMultiples(cells) {
  const points = ladderPoints(cells);
  if (points.length < 2) return null;
  const width = 700, panelH = 138, gap = 48, left = 82, right = 26, top = 16, bottom = 56;
  const plotW = width - left - right;
  const height = top + panelH * 2 + gap + bottom;
  const lens = points.map((p) => p.input_len);
  const minL = Math.min(...lens), maxL = Math.max(...lens);
  const logMin = Math.log(minL), logSpan = Math.log(maxL) - logMin || 1;
  const x = (len) => left + plotW * ((Math.log(len) - logMin) / logSpan);
  const series = [
    {
      label: "prefill (tok/s)", unit: "tok/s", digits: 0, cls: "s1",
      values: points.map((p) => (p.prefill && p.prefill.agg_tok_s) || 0),
    },
    {
      label: "time to the first token (s)", unit: "s", digits: 2, cls: "s2",
      values: points.map((p) => (p.ttft_s && p.ttft_s.p50) || 0),
    },
  ];
  const svg = svgEl("svg", {
    class: "bench-curve", viewBox: "0 0 " + width + " " + height, role: "img",
    "aria-label": "The prefill speed and the time to the first token against the prompt length",
  });
  series.forEach((s, si) => {
    const y0 = top + si * (panelH + gap);
    const max = Math.max(...s.values, 0.0001);
    const y = (v) => y0 + panelH - (panelH * v) / max;
    svg.appendChild(svgEl("line", { class: "axis", x1: left, x2: left, y1: y0, y2: y0 + panelH }));
    svg.appendChild(svgEl("line", { class: "axis", x1: left, x2: left + plotW, y1: y0 + panelH, y2: y0 + panelH }));
    for (const f of [0, 0.5, 1]) {
      const gy = y0 + panelH - panelH * f;
      svg.appendChild(svgEl("line", { class: "grid", x1: left, x2: left + plotW, y1: gy, y2: gy }));
      svg.appendChild(svgEl("text", {
        class: "axis-label", x: left - 6, y: gy + 3, "text-anchor": "end",
      }, benchNum(max * f, s.digits)));
    }
    svg.appendChild(svgEl("text", { class: "axis-title", x: left, y: y0 - 7 }, s.label));
    svg.appendChild(svgEl("polyline", {
      class: "curve-line " + s.cls,
      points: s.values.map((v, i) => x(lens[i]) + "," + y(v)).join(" "),
    }));
    points.forEach((p, i) => {
      svg.appendChild(svgEl("circle", { class: "curve-dot " + s.cls, cx: x(lens[i]), cy: y(s.values[i]), r: 4 }));
    });
    // The first and the last point carry their number; the numbers of the
    // other points are in the table under the chart.
    const last = points.length - 1;
    svg.appendChild(svgEl("text", {
      class: "point-label " + s.cls, x: x(lens[0]) + 7, y: y(s.values[0]) + 15,
    }, benchNum(s.values[0], s.digits) + " " + s.unit));
    svg.appendChild(svgEl("text", {
      class: "point-label " + s.cls, x: x(lens[last]) - 7, y: y(s.values[last]) - 9, "text-anchor": "end",
    }, benchNum(s.values[last], s.digits) + " " + s.unit));
  });
  const yx = top + panelH * 2 + gap;
  for (const len of lens) {
    svg.appendChild(svgEl("text", {
      class: "axis-label", x: x(len), y: yx + 17, "text-anchor": "middle",
    }, len >= 1024 ? (len / 1024) + "k" : String(len)));
  }
  svg.appendChild(svgEl("text", {
    class: "axis-title", x: left + plotW, y: yx + 36, "text-anchor": "end",
  }, "prompt tokens (log scale)"));
  return h("div.bench-curve-wrap", svg, curveTable(points));
}

/** curveTable is the text form of the curve: every point of the ladder with
 *  its two numbers. */
function curveTable(points) {
  const rows = points.map((p) => h("tr",
    h("td.num", String(p.input_len)),
    h("td.num", benchNum((p.prefill && p.prefill.agg_tok_s) || 0, 1) + " tok/s"),
    h("td.num", benchNum((p.ttft_s && p.ttft_s.p50) || 0, 3) + " s")));
  return h("details.bench-fallback",
    h("summary", "The numbers of the curve"),
    h("div.scroll", h("table.bench-table",
      h("thead", h("tr",
        h("th.num", "PROMPT TOKENS"), h("th.num", "PREFILL"), h("th.num", "TIME TO FIRST TOKEN"))),
      h("tbody", rows))));
}
