// Shared chart primitives: the sparkline, the area chart, the busy ring and
// the meter. Every drawing is an inline SVG built with createElementNS; there
// is no chart library, no canvas and no image.
//
// The measured geometry of the drawings (the 120 x 34 sparkline, the 84 px
// ring, the axis of the area chart) follows the reference UI.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
//
// Accessibility: every chart says its numbers in words (the aria-label and the
// text beside it), the two charts with a readout can be reached with the
// keyboard and the arrow keys move that readout, and a number that was not
// measured is a dash, never a zero.
"use strict";

// The one helper of the drawings lives in core.js: the charts of the
// Dashboard tab and the charts of the Classic tab share it.

/** downsample keeps at most max points of an array, evenly spaced. The
 *  chart fills after the first minute; older samples are dropped. */
function downsample(pts, max) {
  if (pts.length <= max) return pts;
  const out = new Array(max);
  const step = (pts.length - 1) / (max - 1);
  for (let i = 0; i < max; i++) out[i] = pts[Math.round(i * step)];
  return out;
}

/** chartNumbers keeps the numbers of a series that can be drawn. */
function chartNumbers(values) {
  const out = [];
  for (const v of values || []) if (typeof v === "number" && isFinite(v)) out.push(v);
  return out;
}

/** chartPaths builds the line and the area of a series. A missing point
 *  breaks the line: the gap in the data is a gap in the chart, never a
 *  zero. points holds plain numbers or {v} objects; px and py map an index
 *  and a value to the canvas; baseY closes the area (null for a line). */
function chartPaths(points, px, py, baseY) {
  let line = "";
  let area = "";
  let run = [];
  const flush = () => {
    if (run.length) {
      let d = "";
      for (let k = 0; k < run.length; k++) {
        d += (k ? "L" : "M") + px(run[k].i).toFixed(1) + " " + py(run[k].v).toFixed(1);
      }
      line += d;
      if (run.length > 1 && baseY != null) {
        area += d + " L" + px(run[run.length - 1].i).toFixed(1) + " " + baseY +
          " L" + px(run[0].i).toFixed(1) + " " + baseY + " Z";
      }
    }
    run = [];
  };
  (points || []).forEach((p, i) => {
    const v = p && typeof p === "object" ? p.v : p;
    if (typeof v !== "number" || !isFinite(v)) {
      flush();
      return;
    }
    run.push({ i: i, v: v });
  });
  flush();
  return { line: line, area: area };
}

/** chartPointer wires the pointer and the keyboard of a chart: the pointer
 *  moves the readout and the arrow keys move it too. The index -1 means that
 *  nothing is pointed at. The count is read when the pointer moves, not when
 *  the chart is built: a chart is built empty and filled later, so the
 *  number of points is only known then. */
function chartPointer(svg, count, onIndex) {
  let index = -1;
  const total = () => {
    const n = typeof count === "function" ? count() : count;
    return typeof n === "number" && n > 0 ? Math.floor(n) : 0;
  };
  const move = (i) => {
    const n = total();
    if (n < 1) return;
    index = Math.max(0, Math.min(n - 1, i));
    onIndex(index);
  };
  const at = (ev) => {
    const n = total();
    if (n < 1) return;
    const box = svg.getBoundingClientRect();
    if (!box.width) return;
    move(Math.round(((ev.clientX - box.left) / box.width) * (n - 1)));
  };
  svg.addEventListener("pointermove", at);
  svg.addEventListener("pointerleave", () => { index = -1; onIndex(-1); });
  svg.addEventListener("focus", () => move(total() - 1));
  svg.addEventListener("blur", () => { index = -1; onIndex(-1); });
  svg.addEventListener("keydown", (ev) => {
    if (ev.key === "ArrowRight") { move(index + 1); ev.preventDefault(); }
    else if (ev.key === "ArrowLeft") { move(index - 1); ev.preventDefault(); }
    else if (ev.key === "Home") { move(0); ev.preventDefault(); }
    else if (ev.key === "End") { move(total() - 1); ev.preventDefault(); }
  });
}

/** chartNice rounds a number up to 1, 2 or 5 times a power of ten: the top
 *  of a scale is a round number, so two charts can be compared by eye. */
function chartNice(v) {
  if (!(v > 0)) return 1;
  const e = Math.pow(10, Math.floor(Math.log10(v)));
  const f = v / e;
  const m = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10;
  return m * e;
}

/** sparkDomain is the scale of one sparkline: the top of the scale is the
 *  limit of the measure when the caller names one, else the largest number
 *  that was measured, rounded up to a round number. The paint and the
 *  pointer read the same domain, so the dot sits on the line. */
function sparkDomain(values, max) {
  let top = 0;
  for (const v of values || []) {
    if (typeof v === "number" && isFinite(v) && v > top) top = v;
  }
  const fixed = typeof max === "number" && max > 0;
  if (fixed) top = max;
  else top = chartNice(top);
  return { min: 0, max: top > 0 ? top : 1, fixed: fixed };
}

/** spark draws the last minute of one number as a small line under a tile.
 *  values is the series, max the top of the scale and label the text a screen
 *  reader reads. The element carries an update method: the caller repaints it
 *  with the next sample instead of building it again. */
function spark(values, max, label, opts) {
  const W = 120, H = 34, PAD = 2;
  const options = opts || {};
  const wrap = h("div.spark-wrap");
  const svg = chartSvg("svg", {
    class: "spark-svg", viewBox: "0 0 " + W + " " + H, preserveAspectRatio: "none",
    role: "img", tabindex: "0", "aria-label": label || "the last minute",
  });
  const base = chartSvg("line", { x1: 0, x2: W, y1: H - 1, y2: H - 1,
    class: "chart-base" });
  // The spark draws the line only: a filled block under a line that never
  // moves reads as a solid bar, not as a number.
  const line = chartSvg("path", { class: "chart-line" });
  const mark = chartSvg("circle", { r: 2.4, class: "chart-pointer" });
  svg.append(base, line, mark);
  const read = h("span.chart-read", { class: "chart-readout" });
  wrap.append(svg, read);

  let values2 = values || [];
  // The top of the scale is the limit of the measure. A caller that names
  // one is obeyed; a percentage is of a hundred and needs no other limit.
  const limit = () => (typeof max === "number" && max > 0)
    ? max : (options.unit === "%" ? 100 : null);
  let domain = sparkDomain(values2, limit());
  const paint = (vals) => {
    const n = vals.length;
    const top = domain.max;
    const step = n > 1 ? W / (n - 1) : 0;
    const px = (i) => (n > 1 ? i * step : W / 2);
    const py = (v) => H - PAD - (Math.min(v, top) / top) * (H - 2 * PAD);
    if (!n) {
      line.setAttribute("d", "");
      mark.setAttribute("opacity", "0");
      return;
    }
    // A missing point breaks the line: the gap in the data is a gap in
    // the chart.
    line.setAttribute("d", chartPaths(vals, px, py, null).line);
    mark.setAttribute("opacity", "0");
  };
  const show = (i) => {
    if (i < 0) { read.style.opacity = "0"; mark.setAttribute("opacity", "0"); return; }
    const v = values2[i];
    if (typeof v !== "number" || !isFinite(v)) {
      read.textContent = "–" + (options.unit ? " " + options.unit : "");
      read.style.opacity = "1";
      mark.setAttribute("opacity", "0");
      return;
    }
    read.textContent = (options.format ? options.format(v) : num(v)) + (options.unit ? " " + options.unit : "");
    read.style.opacity = "1";
  };
  paint(values2);
  chartPointer(svg, () => values2.length, (i) => {
    if (i < 0) { read.style.opacity = "0"; mark.setAttribute("opacity", "0"); return; }
    const v = values2[i];
    if (typeof v !== "number" || !isFinite(v)) {
      show(i);
      return;
    }
    const top = domain.max;
    const step = values2.length > 1 ? W / (values2.length - 1) : 0;
    const x = values2.length > 1 ? i * step : W / 2;
    const y = H - PAD - (Math.min(v, top) / top) * (H - 2 * PAD);
    mark.setAttribute("cx", x.toFixed(1));
    mark.setAttribute("cy", y.toFixed(1));
    mark.setAttribute("opacity", "1");
    show(i);
  });
  wrap.update = (next, nextMax) => {
    values2 = next || [];
    if (nextMax) max = nextMax;
    domain = sparkDomain(values2, limit());
    paint(values2);
    const nums = chartNumbers(values2);
    const last = nums.length ? nums[nums.length - 1] : null;
    // A spark without a fixed limit says the range it draws, so the reader
    // knows what the height of the line means.
    const range = domain.fixed ? ""
      : ", 0 to " + num(domain.max) + (options.unit ? " " + options.unit : "");
    svg.setAttribute("aria-label", (label || "the last minute") + ": " +
      (last == null ? "no sample yet" : (options.format ? options.format(last) : num(last)) + (options.unit ? " " + options.unit : "")) + range);
  };
  wrap.update(values);
  return wrap;
}

/** areaChart draws one measure over a window: the load, the power, the memory
 *  or the tokens per second. series is the list of values or the list of
 *  {t, v} points; opts carries the title, the unit, the window and the empty
 *  copy. The element carries an update method. */
function areaChart(input, opts) {
  const options = opts || {};
  const W = 320, H = 170, L = 34, R = 312, T = 8, B = 128;
  const box = h("figure.chart", { class: "chart-figure" });
  const head = h("figcaption", { class: "chart-caption" });
  const title = h("span", { class: "chart-label" }, options.title || "");
  const read = h("span.chart-read", { class: "chart-value" }, "");
  head.append(title, read);
  const svg = chartSvg("svg", {
    viewBox: "0 0 " + W + " " + H, role: "img", tabindex: "0",
    "aria-label": options.title || "the measure", class: "chart-svg",
  });
  const grid = chartSvg("g", { class: "chart-grid" });
  const ylab = chartSvg("g", { class: "chart-axis-end" });
  const xlab = chartSvg("g", { class: "chart-axis" });
  const area = chartSvg("path", { class: "chart-area" });
  const line = chartSvg("path", { class: "chart-line-strong" });
  const extra = chartSvg("g", { class: "chart-extra" });
  const cross = chartSvg("line", { y1: T, y2: B, class: "chart-cross" });
  const dot = chartSvg("circle", { r: 3.5, class: "chart-dot" });
  svg.append(grid, area, line, extra, cross, dot, ylab, xlab);
  box.append(head, svg);
  const empty = h("div.chart-empty");
  box.append(empty);

  const norm = (input) => {
    const out = [];
    for (const p of input || []) {
      if (Array.isArray(p)) out.push({ t: null, v: p[1] });
      else if (p && typeof p === "object") out.push({ t: p.t || p.time || null, v: typeof p.v === "number" ? p.v : p.value });
      else if (typeof p === "number") out.push({ t: null, v: p });
      else out.push({ t: null, v: null });
    }
    // A missing point stays in the series: the chart breaks the line there
    // instead of drawing a zero.
    return downsample(out, 240);
  };
  // The chart takes one series (a list of values or points) or a list of
  // series: {values, label, dash}. A second series gets its own line, its
  // own dash pattern and its own word on the chart, so a reader who cannot
  // see the colours still knows which line is which.
  const seriesOf = (input) => {
    const many = Array.isArray(input) && input.length && input[0] && typeof input[0] === "object" &&
      !Array.isArray(input[0]) && input[0].values !== undefined;
    const list = many ? input : [null];
    return list.map((one) => ({
      points: norm(one ? one.values : input),
      label: one && one.label ? one.label : "",
      dash: !!(one && one.dash),
    }));
  };

  let series = [];
  // The scale of the chart: the fixed maximum of the measure when there is
  // one, else the largest number that was measured. The paint and the
  // pointer read it, so the dot sits on the line.
  let maxV = 1;
  const paint = () => {
    const n = Math.max(1, ...series.map((one) => one.points.length));
    const values = chartNumbers(series.flatMap((one) => one.points.map((p) => p.v)));
    maxV = options.max && options.max > 0 ? options.max : Math.max(1, ...values);
    const px = (i) => (n > 1 ? L + (i * (R - L)) / (n - 1) : (L + R) / 2);
    const py = (v) => B - (Math.min(v, maxV) / maxV) * (B - T);
    grid.replaceChildren();
    ylab.replaceChildren();
    xlab.replaceChildren();
    extra.replaceChildren();
    for (const f of [0, 0.5, 1]) {
      const y = B - f * (B - T);
      grid.append(chartSvg("line", { x1: L, x2: R, y1: y, y2: y }));
      const t = chartSvg("text", { x: L - 6, y: y + 3 });
      t.textContent = f === 1 ? num(maxV, maxV < 10 ? 1 : 0) + (options.unit ? " " + options.unit : "") : f === 0 ? "0" : num(maxV / 2, maxV < 10 ? 1 : 0);
      ylab.append(t);
    }
    const first = series[0];
    if (!first || !first.points.length) {
      line.setAttribute("d", "");
      area.setAttribute("d", "");
      empty.hidden = false;
      empty.textContent = "";
      empty.append(h("b", { class: "chart-strong" }, "No samples yet."),
        h("span", { class: "chart-sub" }, options.hint || "The chart fills after the first minute."));
      return;
    }
    empty.hidden = true;
    const used = [];
    series.forEach((one, idx) => {
      if (!one.points.length) return;
      // A missing point breaks the line: the gap in the data is a gap in
      // the chart, never a zero.
      const seg = chartPaths(one.points, px, py, idx === 0 ? B : null);
      if (idx === 0) {
        line.setAttribute("d", seg.line);
        area.setAttribute("d", seg.area);
      } else {
        extra.append(chartSvg("path", { class: one.dash ? "chart-line-dash" : "chart-line-alt", d: seg.line }));
      }
      if (!one.label) return;
      const nums = chartNumbers(one.points.map((q) => q.v));
      const last = nums.length ? nums[nums.length - 1] : 0;
      let y = py(last) - 5;
      while (used.some((u) => Math.abs(u - y) < 11)) y -= 11;
      used.push(y);
      const t = chartSvg("text", { class: "chart-series-label", x: R - 2, y: y, "text-anchor": "end" });
      t.textContent = one.label;
      extra.append(t);
    });
    const mins = options.windowMinutes || 5;
    const marks = [[L, "- " + mins + " min", "start"], [(L + R) / 2, "- " + (mins / 2) + " min", "middle"], [R, "now", "end"]];
    for (const [x, text, anchor] of marks) {
      const t = chartSvg("text", { x: x, y: B + 18, "text-anchor": anchor });
      t.textContent = text;
      xlab.append(t);
    }
  };
  const fmt = (p) => (options.format ? options.format(p.v) : num(p.v)) + (options.unit ? " " + options.unit : "");
  const at = (i) => series.map((one) => {
    const p = one.points[i];
    if (!p) return null;
    return (one.label ? one.label + " " : "") + fmt(p);
  }).filter(Boolean).join(" · ");
  const show = (i) => {
    if (i < 0 || !series.length || !series[0].points.length) {
      read.textContent = series[0] && series[0].points.length ? at(series[0].points.length - 1) : "";
      cross.setAttribute("opacity", "0");
      dot.setAttribute("opacity", "0");
      return;
    }
    const p = series[0].points[i];
    const n = Math.max(1, ...series.map((one) => one.points.length));
    const px = (n > 1 ? L + (i * (R - L)) / (n - 1) : (L + R) / 2);
    const py = B - (Math.min(p.v, maxV) / maxV) * (B - T);
    cross.setAttribute("x1", px.toFixed(1));
    cross.setAttribute("x2", px.toFixed(1));
    cross.setAttribute("opacity", "1");
    dot.setAttribute("cx", px.toFixed(1));
    dot.setAttribute("cy", py.toFixed(1));
    dot.setAttribute("opacity", "1");
    read.textContent = at(i);
  };
  series = seriesOf(input);
  paint();
  chartPointer(svg, () => (series[0] ? series[0].points.length : 0), show);
  box.update = (next, mins) => {
    if (mins) options.windowMinutes = mins;
    series = seriesOf(next);
    paint();
    const last = series[0] && series[0].points.length ? series[0].points.length - 1 : -1;
    svg.setAttribute("aria-label", (options.title || "the measure") + ": " + (last >= 0 ? at(last) : "no sample yet"));
    show(-1);
  };
  box.update(input);
  return box;
}

/** ring draws the load of one card: the track, the arc, the number in the
 *  middle and the name of the card under it. A card that does not report its
 *  load shows a dash, never a zero. The element carries an update method. */
function ring(pct, label, opts) {
  // A horizontal 0-100% bar. The old donut sat at 84 px and crowded the card;
  // the bar reads the same number with the same words.
  const box = h("div.ring-wrap", { class: "ring-wrap" });
  const num = h("div", { class: "ring-n" }, "–");
  const track = h("div", { class: "ring-track", role: "img" });
  const fill = h("i", { class: "ring-fill" });
  const cap = h("div", { class: "ring-caption" }, label || "");
  track.append(fill);
  box.append(num, track, cap);
  const paint = (v) => {
    if (v == null || !isFinite(v)) {
      fill.style.width = "0%";
      num.textContent = "–";
      track.setAttribute("aria-label", "the load of " + (label || "the card") + " is not published");
      return;
    }
    const p = Math.max(0, Math.min(100, v));
    fill.style.width = p.toFixed(1) + "%";
    num.textContent = String(Math.round(p));
    track.setAttribute("aria-label", "the load of " + (label || "the card") + ": " + Math.round(p) + " %");
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
  const box = h("div.metric");
  const label = h("span", options.label || "");
  const amount = h("b", "");
  const head = h("div.lbl", label, amount);
  const track = h("div.bar", { role: "img" });
  const fill = h("i");
  track.append(fill);
  const tick = h("span", { class: "meter-tick" });
  const wrap = h("div", { class: "meter-track-wrap" }, track, tick);
  const note = h("div", { class: "meter-note" }, "");
  box.append(head, wrap, note);

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
    const fmtM = options.format ? options.format(scale) : num(scale);
    amount.textContent = scale ? fmtV + sep + fmtM + unit : fmtV + unit;
    fill.style.width = scale ? Math.min(100, (v / scale) * 100).toFixed(1) + "%" : "0%";
    track.classList.toggle("hot", !!over);
    if (cap && scale) {
      tick.style.display = "block";
      tick.style.left = Math.min(100, (cap / scale) * 100).toFixed(1) + "%";
    } else {
      tick.style.display = "none";
    }
    track.setAttribute("aria-label", (options.label || "the measure") + ": " + fmtV + unit +
      (scale ? " of " + fmtM + unit : "") + (over ? ", over the cap" : ""));
    note.textContent = over ? (options.overNote || "over the cap") : (options.note || "");
    if (over) note.style.color = "var(--serious)";
    else note.style.color = "var(--muted)";
  };
  paint(value, max);
  box.update = (v, m) => paint(v, m);
  return box;
}
