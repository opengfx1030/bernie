// Shared chart primitives: the sparkline, the area chart, the busy ring and
// the meter. Every drawing is an inline SVG built with createElementNS; there
// is no chart library, no canvas and no image.
//
// Accessibility: every chart says its numbers in words (the aria-label and the
// text beside it), the two charts with a readout can be reached with the
// keyboard and the arrow keys move that readout, and a number that was not
// measured is a dash, never a zero.
"use strict";

const SVG_NS = "http://www.w3.org/2000/svg";

/** chartSvg builds one node of an inline SVG. */
function chartSvg(tag, attrs) {
  const el = document.createElementNS(SVG_NS, tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    el.setAttribute(k, String(v));
  }
  return el;
}

/** chartNumbers keeps the numbers of a series that can be drawn. */
function chartNumbers(values) {
  const out = [];
  for (const v of values || []) if (typeof v === "number" && isFinite(v)) out.push(v);
  return out;
}

/** chartPointer wires the pointer and the keyboard of a chart: the pointer
 *  moves the readout and the arrow keys move it too. The index -1 means that
 *  nothing is pointed at. */
function chartPointer(svg, count, onIndex) {
  let index = -1;
  const move = (i) => {
    if (count < 1) return;
    index = Math.max(0, Math.min(count - 1, i));
    onIndex(index);
  };
  const at = (ev) => {
    const box = svg.getBoundingClientRect();
    if (!box.width) return;
    move(Math.round(((ev.clientX - box.left) / box.width) * (count - 1)));
  };
  svg.addEventListener("pointermove", at);
  svg.addEventListener("pointerleave", () => { index = -1; onIndex(-1); });
  svg.addEventListener("focus", () => move(count - 1));
  svg.addEventListener("blur", () => { index = -1; onIndex(-1); });
  svg.addEventListener("keydown", (ev) => {
    if (ev.key === "ArrowRight") { move(index + 1); ev.preventDefault(); }
    else if (ev.key === "ArrowLeft") { move(index - 1); ev.preventDefault(); }
    else if (ev.key === "Home") { move(0); ev.preventDefault(); }
    else if (ev.key === "End") { move(count - 1); ev.preventDefault(); }
  });
}

/** spark draws the last minute of one number as a small line under a tile.
 *  values is the series, max the top of the scale and label the text a screen
 *  reader reads. The element carries an update method: the caller repaints it
 *  with the next sample instead of building it again. */
function spark(values, max, label, opts) {
  const W = 120, H = 34, PAD = 2;
  const options = opts || {};
  const wrap = h("div.spark-wrap", { style: "position:relative;min-width:0" });
  const svg = chartSvg("svg", {
    class: "spark-svg", viewBox: "0 0 " + W + " " + H, preserveAspectRatio: "none",
    role: "img", tabindex: "0", "aria-label": label || "the last minute",
  });
  const base = chartSvg("line", { x1: 0, x2: W, y1: H - 1, y2: H - 1,
    style: "stroke:var(--border);stroke-width:1;stroke-dasharray:3 3;vector-effect:non-scaling-stroke" });
  const area = chartSvg("path", { style: "fill:var(--series-1);opacity:.18;stroke:none" });
  const line = chartSvg("path", { style: "fill:none;stroke:var(--series-1);stroke-width:1.6;stroke-linejoin:round;stroke-linecap:round;vector-effect:non-scaling-stroke" });
  const mark = chartSvg("circle", { r: 2.4, style: "fill:var(--series-1);opacity:0;vector-effect:non-scaling-stroke" });
  svg.append(base, area, line, mark);
  const read = h("span.chart-read", { style: "position:absolute;right:0;top:0;padding:0 4px;border-radius:4px;border:1px solid var(--border);background:var(--surface);color:var(--text-2);font-size:11px;line-height:1.5;opacity:0;pointer-events:none" });
  wrap.append(svg, read);

  const paint = (vals) => {
    const n = vals.length;
    const top = max && max > 0 ? max : Math.max(1, ...vals);
    const step = n > 1 ? W / (n - 1) : 0;
    const px = (i) => (n > 1 ? i * step : W / 2);
    const py = (v) => H - PAD - (Math.min(v, top) / top) * (H - 2 * PAD);
    if (!n) {
      area.setAttribute("d", "");
      line.setAttribute("d", "");
      mark.setAttribute("opacity", "0");
      return;
    }
    let d = "";
    vals.forEach((v, i) => { d += (i ? "L" : "M") + px(i).toFixed(1) + " " + py(v).toFixed(1); });
    line.setAttribute("d", d);
    area.setAttribute("d", d + " L " + W + " " + (H - 1) + " L 0 " + (H - 1) + " Z");
    mark.setAttribute("opacity", "0");
  };
  const show = (i) => {
    if (i < 0) { read.style.opacity = "0"; mark.setAttribute("opacity", "0"); return; }
    const vals = chartNumbers(values);
    read.textContent = (options.format ? options.format(vals[i]) : num(vals[i])) + (options.unit ? " " + options.unit : "");
    read.style.opacity = "1";
  };
  let values2 = chartNumbers(values);
  paint(values2);
  chartPointer(svg, values2.length, (i) => {
    if (i < 0) { read.style.opacity = "0"; mark.setAttribute("opacity", "0"); return; }
    const top = max && max > 0 ? max : Math.max(1, ...values2);
    const step = values2.length > 1 ? W / (values2.length - 1) : 0;
    const x = values2.length > 1 ? i * step : W / 2;
    const y = H - PAD - (Math.min(values2[i], top) / top) * (H - 2 * PAD);
    mark.setAttribute("cx", x.toFixed(1));
    mark.setAttribute("cy", y.toFixed(1));
    mark.setAttribute("opacity", "1");
    show(i);
  });
  wrap.update = (next, nextMax) => {
    values2 = chartNumbers(next);
    if (nextMax) max = nextMax;
    paint(values2);
    const last = values2.length ? values2[values2.length - 1] : null;
    svg.setAttribute("aria-label", (label || "the last minute") + ": " +
      (last == null ? "no sample yet" : (options.format ? options.format(last) : num(last)) + (options.unit ? " " + options.unit : "")));
  };
  wrap.update(values);
  return wrap;
}

/** areaChart draws one measure over a window: the load, the power, the memory
 *  or the tokens per second. series is the list of values or the list of
 *  {t, v} points; opts carries the title, the unit, the window and the empty
 *  copy. The element carries an update method. */
function areaChart(series, opts) {
  const options = opts || {};
  const W = 320, H = 170, L = 34, R = 312, T = 8, B = 128;
  const box = h("figure.chart", { style: "margin:0;min-width:0" });
  const head = h("figcaption", { style: "display:flex;justify-content:space-between;align-items:baseline;gap:8px;margin:0 0 6px" });
  const title = h("span", { style: "font:800 11px/1.2 var(--font-body);letter-spacing:.08em;text-transform:uppercase;color:var(--muted)" }, options.title || "");
  const read = h("span.chart-read", { style: "font-size:12px;color:var(--text-2);font-variant-numeric:tabular-nums" }, "");
  head.append(title, read);
  const svg = chartSvg("svg", {
    viewBox: "0 0 " + W + " " + H, role: "img", tabindex: "0",
    "aria-label": options.title || "the measure", style: "width:100%;height:auto;display:block;overflow:visible",
  });
  const grid = chartSvg("g", { style: "stroke:var(--border);stroke-width:1" });
  const ylab = chartSvg("g", { style: "fill:var(--muted);font:400 10px var(--font-body);text-anchor:end" });
  const xlab = chartSvg("g", { style: "fill:var(--muted);font:400 10px var(--font-body)" });
  const area = chartSvg("path", { style: "fill:var(--series-1);opacity:.18;stroke:none" });
  const line = chartSvg("path", { style: "fill:none;stroke:var(--series-1);stroke-width:2;stroke-linejoin:round;stroke-linecap:round" });
  const cross = chartSvg("line", { y1: T, y2: B, style: "stroke:var(--border-strong);stroke-width:1;stroke-dasharray:3 3;opacity:0" });
  const dot = chartSvg("circle", { r: 3.5, style: "fill:var(--series-1);opacity:0" });
  svg.append(grid, area, line, cross, dot, ylab, xlab);
  box.append(head, svg);
  const empty = h("div.chart-empty", { style: "color:var(--muted);font-size:12px;padding:2px 0" });
  box.append(empty);

  let points = [];
  const norm = (input) => {
    const out = [];
    for (const p of input || []) {
      if (Array.isArray(p)) out.push({ t: null, v: p[1] });
      else if (p && typeof p === "object") out.push({ t: p.t || p.time || null, v: typeof p.v === "number" ? p.v : p.value });
      else if (typeof p === "number") out.push({ t: null, v: p });
    }
    return out.filter((p) => typeof p.v === "number" && isFinite(p.v));
  };
  const paint = () => {
    const n = points.length;
    const maxV = options.max && options.max > 0 ? options.max : Math.max(1, ...points.map((p) => p.v));
    const px = (i) => (n > 1 ? L + (i * (R - L)) / (n - 1) : (L + R) / 2);
    const py = (v) => B - (Math.min(v, maxV) / maxV) * (B - T);
    grid.replaceChildren();
    ylab.replaceChildren();
    xlab.replaceChildren();
    for (const f of [0, 0.5, 1]) {
      const y = B - f * (B - T);
      grid.append(chartSvg("line", { x1: L, x2: R, y1: y, y2: y }));
      const t = chartSvg("text", { x: L - 6, y: y + 3 });
      t.textContent = f === 1 ? num(maxV, maxV < 10 ? 1 : 0) + (options.unit ? " " + options.unit : "") : f === 0 ? "0" : num(maxV / 2, maxV < 10 ? 1 : 0);
      ylab.append(t);
    }
    if (!n) {
      area.setAttribute("d", "");
      line.setAttribute("d", "");
      empty.hidden = false;
      empty.textContent = "";
      empty.append(h("b", { style: "color:var(--text)" }, "No samples yet."),
        h("span", { style: "display:block;margin-top:2px" }, options.hint || "The chart fills after the first minute."));
      return;
    }
    empty.hidden = true;
    let d = "";
    points.forEach((p, i) => { d += (i ? "L" : "M") + px(i).toFixed(1) + " " + py(p.v).toFixed(1); });
    line.setAttribute("d", d);
    area.setAttribute("d", d + " L " + R + " " + B + " L " + L + " " + B + " Z");
    const mins = options.windowMinutes || 5;
    const marks = [[L, "- " + mins + " min", "start"], [(L + R) / 2, "- " + (mins / 2) + " min", "middle"], [R, "now", "end"]];
    for (const [x, text, anchor] of marks) {
      const t = chartSvg("text", { x: x, y: B + 18, "text-anchor": anchor });
      t.textContent = text;
      xlab.append(t);
    }
  };
  const fmt = (p) => (options.format ? options.format(p.v) : num(p.v)) + (options.unit ? " " + options.unit : "");
  const show = (i) => {
    if (i < 0 || !points.length) { read.textContent = points.length ? fmt(points[points.length - 1]) : ""; cross.setAttribute("opacity", "0"); dot.setAttribute("opacity", "0"); return; }
    const p = points[i];
    const maxV = options.max && options.max > 0 ? options.max : Math.max(1, ...points.map((q) => q.v));
    const px = points.length > 1 ? L + (i * (R - L)) / (points.length - 1) : (L + R) / 2;
    const py = B - (Math.min(p.v, maxV) / maxV) * (B - T);
    cross.setAttribute("x1", px.toFixed(1));
    cross.setAttribute("x2", px.toFixed(1));
    cross.setAttribute("opacity", "1");
    dot.setAttribute("cx", px.toFixed(1));
    dot.setAttribute("cy", py.toFixed(1));
    dot.setAttribute("opacity", "1");
    read.textContent = fmt(p) + (p.t ? " · " + chartClock(p.t) : "");
  };
  points = norm(series);
  paint();
  chartPointer(svg, points.length, show);
  box.update = (next, mins) => {
    if (mins) options.windowMinutes = mins;
    points = norm(next);
    paint();
    const last = points.length ? points[points.length - 1] : null;
    svg.setAttribute("aria-label", (options.title || "the measure") + ": " +
      (last ? fmt(last) : "no sample yet"));
    show(-1);
  };
  box.update(series);
  return box;
}

/** chartClock writes the time of a sample as HH:MM:SS. */
function chartClock(t) {
  const d = new Date(t);
  if (isNaN(d.getTime())) return "";
  const p = (n) => String(n).padStart(2, "0");
  return p(d.getHours()) + ":" + p(d.getMinutes()) + ":" + p(d.getSeconds());
}

/** ring draws the load of one card: the track, the arc, the number in the
 *  middle and the name of the card under it. A card that does not report its
 *  load shows a dash, never a zero. The element carries an update method. */
function ring(pct, label, opts) {
  const options = opts || {};
  const R = 36, C = 2 * Math.PI * R;
  const box = h("div.ring-wrap", { style: "flex:none;text-align:center;min-width:84px" });
  const svg = chartSvg("svg", { class: "ring", viewBox: "0 0 84 84", width: 84, height: 84, role: "img", "aria-label": "the load of " + (label || "the card") });
  const track = chartSvg("circle", { cx: 42, cy: 42, r: R, style: "fill:var(--surface-3)" });
  const arc = chartSvg("circle", { cx: 42, cy: 42, r: R, "stroke-dasharray": C.toFixed(2),
    transform: "rotate(-90 42 42)", style: "fill:none;stroke:var(--series-1);stroke-width:8;stroke-linecap:round" });
  const value = chartSvg("text", { x: 42, y: 47, "text-anchor": "middle", style: "fill:var(--text);font:900 18px var(--font-display)" });
  svg.append(track, arc, value);
  const cap = h("div", { style: "font-size:11px;color:var(--muted);margin-top:2px;overflow-wrap:anywhere" }, label || "");
  box.append(svg, cap);
  const paint = (v) => {
    if (v == null || !isFinite(v)) {
      arc.setAttribute("stroke-dashoffset", C.toFixed(2));
      value.textContent = "–";
      svg.setAttribute("aria-label", "the load of " + (label || "the card") + " is not published");
      return;
    }
    const p = Math.max(0, Math.min(100, v));
    arc.setAttribute("stroke-dashoffset", (C * (1 - p / 100)).toFixed(2));
    value.textContent = String(Math.round(p));
    svg.setAttribute("aria-label", "the load of " + (label || "the card") + ": " + Math.round(p) + " %");
  };
  paint(pct);
  box.update = (v) => paint(v);
  return box;
}

/** meter draws one value against its maximum: the name of the measure, the
 *  value with its unit, the track, the fill and the tick of the cap. A value
 *  that was not measured shows a dash and an empty track. The element carries
 *  an update method. */
function meter(value, max, opts) {
  const options = opts || {};
  const box = h("div.metric", { style: "margin-top:8px" });
  const head = h("div", { style: "display:flex;justify-content:space-between;gap:10px;align-items:baseline" });
  const label = h("span", { style: "font:800 11px/1.2 var(--font-body);letter-spacing:.08em;text-transform:uppercase;color:var(--muted)" }, options.label || "");
  const amount = h("span", { style: "font-size:13px;color:var(--text-2);font-variant-numeric:tabular-nums;text-align:right" }, "");
  head.append(label, amount);
  const track = h("div", { role: "img", style: "position:relative;height:8px;border-radius:4px;background:var(--surface-3);margin-top:3px;overflow:hidden" });
  const fill = h("i", { style: "display:block;height:100%;width:0;background:var(--series-1);transition:width .2s" });
  const tick = h("span", { style: "position:absolute;top:-2px;bottom:-2px;width:2px;background:var(--border-strong);display:none" });
  track.append(fill, tick);
  const note = h("div", { style: "font-size:11px;color:var(--muted);margin-top:3px" }, "");
  box.append(head, track, note);

  const paint = (v, m) => {
    const unit = options.unit ? " " + options.unit : "";
    const fmtV = options.format ? options.format(v) : num(v);
    if (v == null || !isFinite(v)) {
      amount.textContent = "–";
      fill.style.width = "0%";
      track.setAttribute("aria-label", (options.label || "the measure") + " is not published");
      note.textContent = options.emptyNote || "";
      return;
    }
    const scale = m && m > 0 ? m : (options.max || 0);
    const cap = options.cap != null ? options.cap : scale;
    const over = cap && v > cap;
    const sep = options.sep || " / ";
    amount.textContent = scale ? fmtV + sep + num(scale) + unit : fmtV + unit;
    fill.style.width = scale ? Math.min(100, (v / scale) * 100).toFixed(1) + "%" : "0%";
    fill.style.background = over ? "var(--serious)" : "var(--series-1)";
    if (cap && scale) {
      tick.style.display = "block";
      tick.style.left = Math.min(100, (cap / scale) * 100).toFixed(1) + "%";
    } else {
      tick.style.display = "none";
    }
    track.setAttribute("aria-label", (options.label || "the measure") + ": " + fmtV + unit +
      (scale ? " of " + num(scale) + unit : "") + (over ? ", over the cap" : ""));
    note.textContent = over ? (options.overNote || "over the cap") : (options.note || "");
    if (over) note.style.color = "var(--serious)";
    else note.style.color = "var(--muted)";
  };
  paint(value, max);
  box.update = (v, m) => paint(v, m);
  return box;
}
