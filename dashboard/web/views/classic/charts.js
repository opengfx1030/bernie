"use strict";
// The drawing parts of the Classic tab: the line chart, the sparkline, the
// ring and the counting numbers.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
//
// Every shape is a DOM node: the SVG parts are made with createElementNS.
// There is no markup string in this tab.

const CLASSIC_SVG = "http://www.w3.org/2000/svg";

/** csvg makes one SVG element with its attributes. */
function csvg(tag, attrs) {
  const el = document.createElementNS(CLASSIC_SVG, tag);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, String(v));
  return el;
}

/** cfmt writes a number the way the reference does: a dash when there is
 *  none, and a thin space between the thousands. */
function cfmt(v, d = 0) {
  if (v == null || v === "" || isNaN(v)) return "–";
  return Number(v).toLocaleString(undefined, { maximumFractionDigits: d, minimumFractionDigits: d });
}

/** cago writes how long ago a time was, in the words of the reference. */
function cago(t) {
  if (!t) return "";
  const s = Math.max(0, Date.now() / 1000 - t);
  if (s < 60) return Math.round(s) + "s";
  if (s < 3600) return Math.round(s / 60) + " min";
  return (s / 3600).toFixed(1) + " h";
}

/* ---------- the line chart: thin lines, a recessive grid, a crosshair ----- */

/** classicChart draws the chart of the reference.
 *  opts: {label, height, series: [{name, color, pts: [[seconds, value]]}], yFmt}
 *  It returns the element of the chart and keeps its geometry in
 *  classic.charts, so the crosshair can find the point under the pointer. */
function classicChart(opts) {
  const W = 640, H = opts.height || 190, L = 46, R = 12, Tp = 10, B = 26;
  const all = opts.series.flatMap((s) => s.pts.filter((p) => p[1] != null));
  const xs = all.map((p) => p[0]);
  const x0 = xs.length ? Math.min(...xs) : 0;
  const x1 = xs.length ? Math.max(...xs) : x0 + 1;
  let ymax = Math.max(1, ...all.map((p) => p[1]));
  const nice = (v) => {
    const p = Math.pow(10, Math.floor(Math.log10(v)));
    const m = v / p;
    return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p;
  };
  ymax = nice(ymax * 1.08);
  const sx = (x) => L + ((x - x0) / (x1 - x0 || 1)) * (W - L - R);
  const sy = (y) => Tp + (1 - y / ymax) * (H - Tp - B);
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => ymax * f);

  const svg = csvg("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": opts.label || "chart" });
  const grid = csvg("g", { class: "grid" });
  for (const t of ticks) {
    grid.append(csvg("line", { x1: L, x2: W - R, y1: sy(t), y2: sy(t) }));
  }
  svg.append(grid);
  const axis = csvg("g", { class: "axis" });
  for (const t of ticks) {
    axis.append(csvg("text", { x: L - 8, y: sy(t) + 4, "text-anchor": "end" },
      csvg("tspan", {}, opts.yFmt ? opts.yFmt(t) : cfmt(t))));
  }
  // The x axis of the reference: the age of the oldest point and "now".
  const now = x1;
  [0, 0.5, 1].forEach((f) => {
    const x = x0 + (x1 - x0) * f;
    const label = f === 1 ? "now" : "-" + Math.round((now - x) / 60 * 10) / 10 + " min";
    axis.append(csvg("text", { x: sx(x), y: H - 6, "text-anchor": f === 0 ? "start" : f === 1 ? "end" : "middle" },
      csvg("tspan", {}, label)));
  });
  svg.append(axis);

  for (const s of opts.series) {
    const pts = s.pts.filter((p) => p[1] != null);
    if (!pts.length) continue;
    const d = pts.map((p, i) => `${i ? "L" : "M"}${sx(p[0]).toFixed(1)},${sy(p[1]).toFixed(1)}`).join("");
    if (pts.length > 1) {
      svg.append(csvg("path", {
        d: `${d}L${sx(pts[pts.length - 1][0]).toFixed(1)},${sy(0)}L${sx(pts[0][0]).toFixed(1)},${sy(0)}Z`,
        fill: s.color, opacity: ".12",
      }));
    }
    svg.append(csvg("path", {
      d, fill: "none", stroke: s.color, "stroke-width": "2",
      "stroke-linejoin": "round", "stroke-linecap": "round",
    }));
    if (pts.length < 3) {
      for (const p of pts) {
        svg.append(csvg("circle", {
          cx: sx(p[0]), cy: sy(p[1]), r: 4, fill: s.color,
          stroke: "var(--bg)", "stroke-width": "2",
        }));
      }
    }
  }
  const xh = csvg("line", { class: "xh", x1: 0, x2: 0, y1: Tp, y2: H - B, stroke: "var(--border-strong)", visibility: "hidden" });
  svg.append(xh);
  svg.append(csvg("rect", { x: L, y: 0, width: W - L - R, height: H, fill: "transparent", class: "hit" }));

  const box = h("div.chart", { "data-chart": opts.id || opts.label || "chart" }, svg);
  classic.charts.set(box, { opts, sx, x0, x1, L, R, W, svg, xh });
  return box;
}

/* ---------- the sparkline and the ring ---------- */

/** classicSpark draws the little line under a number. */
function classicSpark(vals, color, h = 34) {
  const svg = csvg("svg", { class: "spark", viewBox: `0 0 120 ${h}`, preserveAspectRatio: "none", "aria-hidden": "true" });
  const v = vals.filter((x) => x != null && !isNaN(x));
  if (v.length < 2) return svg;
  const max = Math.max(...v, 1e-9);
  const n = vals.length;
  const pts = vals.map((x, i) => [(i / (n - 1)) * 120, h - 2 - ((x == null ? 0 : x) / max) * (h - 6)]);
  const d = pts.map((p, i) => `${i ? "L" : "M"}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join("");
  svg.append(csvg("path", { d: `${d}L120,${h}L0,${h}Z`, fill: color, opacity: ".14" }));
  svg.append(csvg("path", {
    d, fill: "none", stroke: color, "stroke-width": "1.6",
    "vector-effect": "non-scaling-stroke", "stroke-linejoin": "round",
  }));
  return svg;
}

/** classicRing draws the ring of the reference around a percentage. */
function classicRing(pct, color) {
  const r = 30;
  const c = 2 * Math.PI * r;
  const p = Math.max(0, Math.min(100, pct || 0));
  const svg = csvg("svg", { class: "ring", viewBox: "0 0 76 76", "aria-hidden": "true" });
  svg.append(csvg("circle", { cx: 38, cy: 38, r, style: "stroke:var(--surface-2)", "stroke-width": 7, fill: "none" }));
  svg.append(csvg("circle", {
    cx: 38, cy: 38, r, style: `stroke:${color};transition:stroke-dashoffset .5s`,
    "stroke-width": 7, fill: "none", "stroke-linecap": "round",
    "stroke-dasharray": c.toFixed(1), "stroke-dashoffset": (c * (1 - p / 100)).toFixed(1),
    transform: "rotate(-90 38 38)",
  }));
  return svg;
}

/* ---------- the numbers that count up ---------- */

/** classicCount writes a number into an element. It counts up from the last
 *  value, unless the reader asked for less motion. */
function classicCount(el, value, digits) {
  if (!el) return;
  const key = el.dataset.k || (el.dataset.k = "c" + Math.random().toString(36).slice(2));
  const from = classic.counts[key];
  classic.counts[key] = value;
  if (value == null || isNaN(value)) { el.textContent = "–"; return; }
  const reduced = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (from == null || isNaN(from) || reduced) { el.textContent = cfmt(value, digits); return; }
  const t0 = performance.now();
  const step = (t) => {
    const k = Math.min(1, (t - t0) / 450);
    const e = 1 - Math.pow(1 - k, 3);
    el.textContent = cfmt(from + (value - from) * e, digits);
    if (k < 1 && el.isConnected) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}
