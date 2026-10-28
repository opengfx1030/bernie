"use strict";
// ---------- The fleet tiles and the cards of the cards ----------
// The hero (the output speed of the live run), the eight tiles of the fleet
// and one card per card of the host.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.

// dashTile is one KPI tile: the name of the number, the number with its
// unit, one line of context and the last minute of it. A number that was not
// measured is a dash and a greyed unit, never a zero.
function dashTile(spec) {
  const el = h("div.tile", { style: "background:var(--surface-2);border:1px solid var(--border);border-radius:16px;padding:16px 18px 12px;display:flex;flex-direction:column;min-width:0" });
  const label = h("div.tile-l", { style: "font-size:13px;color:var(--text-2);font-weight:500;display:flex;align-items:center;gap:6px" }, spec.label);
  if (spec.tip) label.append(dashInfo(spec.tip));
  const value = h("div.tile-n", { style: "font:900 30px/1.05 var(--font-display);letter-spacing:-.02em;margin:8px 0 2px;font-variant-numeric:tabular-nums;white-space:nowrap" });
  const amount = h("span", spec.value);
  const unit = h("small", { style: "font:700 13px var(--font-display);color:var(--muted);margin-left:5px" }, spec.unit || "");
  value.append(amount, unit);
  const sub = h("div.tile-s", { style: "font-size:12.5px;color:var(--muted)" }, spec.sub || "");
  const sparkBox = h("div.tile-sp", { style: "margin-top:auto;padding-top:10px" });
  const chart = spark(spec.series || [], spec.max, spec.label, { unit: spec.unit, format: spec.format });
  sparkBox.append(chart);
  el.append(label, value, sub, sparkBox);
  el.update = (next) => {
    amount.textContent = next.value;
    unit.textContent = next.unit || "";
    unit.style.opacity = next.value === "–" ? ".55" : "1";
    sub.textContent = next.sub || "";
    chart.update(next.series || [], next.max);
  };
  return el;
}

// dashTiles is the eight tiles of the fleet, in the order of the design.
function dashTiles() {
  const box = h("div.tiles", { style: "display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,132px),1fr));gap:16px" });
  const defs = [
    { key: "busy", label: "Load", unit: "%", tip: "The mean load of the cards, in percent. The load is the share of the time the card was busy." },
    { key: "pow", label: "Power", unit: "W", tip: "The sum of the power of the cards. The card cannot draw more than its cap." },
    { key: "mem", label: "Memory", unit: "GiB", tip: "The sum of the video memory in use on the cards." },
    { key: "hot", label: "Hottest card", unit: "°C", tip: "The highest edge temperature of the cards. The line names the card that is hottest." },
    { key: "clk", label: "Clock", unit: "MHz", tip: "The highest shader clock of the cards. The line names the card that runs fastest." },
    { key: "gen", label: "Tokens per second", unit: "tok/s", tip: "The output speed of the live run: the tokens the server writes every second." },
    { key: "running", label: "Running requests", unit: "", tip: "The requests the server is answering right now. The line counts the ones that wait." },
    { key: "kv", label: "KV cache", unit: "%", tip: "The share of the key-value cache the engine holds. A full cache slows the server down." },
  ];
  const tiles = defs.map((d) => {
    const t = dashTile({ label: d.label, unit: d.unit, tip: d.tip, value: "–", sub: "" });
    t.key = d.key;
    box.append(t);
    return t;
  });
  box.update = () => dashTilesPaint(tiles);
  box.update();
  return box;
}

// dashHero is the block at the top of the tab: the output speed of the live
// run on the left, the tiles of the fleet on the right.
function dashHero() {
  const el = h("div.hero-panel", { style: "display:grid;grid-template-columns:minmax(190px,320px) 1fr;gap:28px;padding:24px 28px;background:var(--surface-2);border:1px solid var(--border);border-radius:18px;align-items:start" });
  const left = h("div", { style: "min-width:0" });
  const eyebrow = h("div.eyebrow", { style: "font:800 12px/1 var(--font-body);letter-spacing:.14em;text-transform:uppercase;color:var(--muted)" }, "Output speed");
  const value = h("div.hero-num", { style: "font:900 64px/1 var(--font-display);letter-spacing:-.03em;color:var(--key);font-variant-numeric:tabular-nums;white-space:nowrap" });
  const amount = h("span", "–");
  const unit = h("small", { style: "font:700 15px var(--font-display);color:var(--muted);margin-left:8px" }, "tok/s");
  value.append(amount, unit);
  const note = h("div.hero-note", { style: "font-size:13px;color:var(--muted);margin-top:6px;min-height:18px" }, "No run is serving.");
  const chartBox = h("div", { style: "margin-top:10px;height:44px" });
  const chart = spark([], null, "the output speed of the live run", { unit: "tok/s" });
  chartBox.append(chart);
  left.append(eyebrow, value, note, chartBox);
  const tiles = dashTiles();
  el.append(left, tiles);
  const paint = () => {
    const last = dash.samples[dash.samples.length - 1];
    const agg = last ? last.agg : null;
    const gen = agg ? agg.gen : null;
    amount.textContent = gen == null ? "–" : num(gen);
    unit.style.opacity = gen == null ? ".55" : "1";
    const series = dashSeries("gen").filter((v) => v != null);
    const avg = series.length ? series.reduce((a, b) => a + b, 0) / series.length : null;
    note.textContent = gen == null ? "No run is serving." : (avg == null ? "the live run" : "1-min avg " + num(avg) + " tok/s");
    chart.update(dashSeries("gen"));
  };
  paint();
  tiles.update = (samples) => {
    dashTilesPaint(tiles, samples);
    paint();
  };
  return { el: el, tiles: tiles, paint: paint };
}

// dashTilesPaint repaints the tiles with the samples of the buffer.
function dashTilesPaint(tiles, samples) {
  const last = dash.samples[dash.samples.length - 1];
  const agg = last ? last.agg : null;
  const count = last ? last.cards.length : 0;
  const gen = dashSeries("gen").filter((v) => v != null);
  const avgGen = gen.length ? gen.reduce((a, b) => a + b, 0) / gen.length : null;
  for (const t of tiles) {
    const k = t.key;
    let value = "–", sub = "", max = null;
    if (agg) {
      if (k === "busy") { value = agg.busy == null ? "–" : num(Math.round(agg.busy)); sub = count ? "the mean of " + count + " cards" : "no card"; max = 100; }
      else if (k === "pow") { value = agg.pow == null ? "–" : dashGroup(agg.pow); sub = agg.cap ? "of " + dashGroup(agg.cap) + " W allowed" : "the power of the cards"; max = agg.cap || null; }
      else if (k === "mem") { value = agg.mem == null ? "–" : num(agg.mem / (1024 * 1024 * 1024)); sub = agg.total ? "of " + num(agg.total / (1024 * 1024 * 1024)) + " GiB" : "the memory of the cards"; max = agg.total || null; }
      else if (k === "hot") { value = agg.hot == null ? "–" : num(agg.hot); sub = agg.hotName || "no card"; }
      else if (k === "clk") { value = agg.clk == null ? "–" : dashGroup(agg.clk); sub = agg.clkName || "no card"; }
      else if (k === "gen") { value = agg.gen == null ? "–" : num(agg.gen); sub = avgGen == null ? "the live run" : "1-min avg " + num(avgGen) + " tok/s"; }
      else if (k === "running") { value = agg.running == null ? "–" : num(agg.running, 0); sub = agg.waiting == null ? "no live run" : num(agg.waiting, 0) + " waiting"; }
      else if (k === "kv") { value = agg.kv == null ? "–" : num(agg.kv * 100, 0); sub = agg.kv == null ? "the server does not publish it" : "of the cache of the engine"; max = 100; }
    } else {
      sub = k === "kv" ? "the server does not publish it" : "waiting for the first sample";
    }
    t.update({ value: value, sub: sub, series: dashSeries(k), max: max });
  }
}

// dashCard is the card of one card of the host: its name, the ring of its
// load, the memory and the power meters, the temperatures, the clocks, the
// ECC state and the last minute of its load.
function dashCard(hip) {
  const hw = ((dash.hw && dash.hw.gpus) || []).find((g) => g.hip_index === hip) || null;
  const el = h("div.gpu-card", { style: "background:var(--surface-2);border:1px solid var(--border);border-radius:18px;padding:18px 18px 12px;min-width:0" });
  const top = h("div.gpu-top", { style: "display:flex;align-items:flex-start;justify-content:space-between;gap:10px" });
  const names = h("div", { style: "min-width:0" });
  const name = h("div.gpu-name", { style: "font:900 20px/1.1 var(--font-display);text-transform:uppercase;overflow-wrap:anywhere" }, dashCardName(hip));
  const where = h("div.small-t", { style: "font-size:12.5px;color:var(--muted);margin-top:3px" }, (hw ? hw.pci : "") + (hw && hw.gfx ? " · " + hw.gfx : ""));
  names.append(name, where);
  top.append(names);
  const mid = h("div.gpu-mid", { style: "display:grid;grid-template-columns:84px 1fr;gap:16px;align-items:center;margin-top:14px" });
  const ringBox = ring(null, "GPU " + hip);
  const stats = h("div.gpu-stats", { style: "display:flex;flex-direction:column;gap:10px;min-width:0" });
  const mem = meter(null, null, { label: "Memory", unit: "GiB", format: (v) => num(v / (1024 * 1024 * 1024)) });
  const pow = meter(null, null, { label: "Power", unit: "W", sep: " of ", format: (v) => num(Math.round(v)) });
  const temps = h("div.stat-row", { style: "display:flex;flex-wrap:wrap;gap:6px 14px;font-size:12.5px;font-variant-numeric:tabular-nums" });
  const clocks = h("div.stat-row", { style: "display:flex;flex-wrap:wrap;gap:6px 14px;font-size:12.5px;font-variant-numeric:tabular-nums;align-items:center" });
  const hot = h("div", { style: "font-size:11.5px;color:var(--serious)" });
  stats.append(mem, pow, temps, hot, clocks);
  mid.append(ringBox, stats);
  const sparkBox = h("div.gpu-spark", { style: "margin-top:12px" });
  const chart = spark([], 100, "the load of GPU " + hip, { unit: "%" });
  sparkBox.append(chart);
  el.append(top, mid, sparkBox);

  const pair = (label, value, color) => h("span", { style: "white-space:nowrap" },
    h("span", { style: "color:var(--muted)" }, label + " "),
    h("b", { style: "font-weight:600;font-variant-numeric:tabular-nums" + (color ? ";color:" + color : "") }, value));

  el.update = () => {
    const sample = dash.tele ? (dash.tele.gpus || []).find((g) => g.hip_index === hip) : null;
    const last = dash.samples[dash.samples.length - 1];
    const card = last ? last.cards.find((c) => c.hip === hip) : null;
    const limits = (hw && hw.temp_limits_c) || {};
    const limit = limits.edge != null ? limits.edge : (hw && hw.power_cap_w ? null : null);
    name.textContent = dashCardName(hip);
    where.textContent = (hw ? hw.pci : "") + (hw && hw.gfx ? " · " + hw.gfx : "");
    ringBox.update(card ? card.busy : null);
    mem.update(card ? card.mem : null, card ? card.total : null);
    pow.update(card ? card.pow : null, card ? card.cap : null);
    const t = card ? card.tmp : null;
    const isHot = t != null && limit != null && t >= limit;
    temps.replaceChildren(
      pair("TEMP", t == null ? "–" : num(t) + " °C", isHot ? "var(--serious)" : ""),
      pair("JUNCTION", card && card.jtmp != null ? num(card.jtmp) + " °C" : "–"),
      pair("MEM", card && card.mtmp != null ? num(card.mtmp) + " °C" : "–"));
    hot.textContent = isHot ? "hot: the limit of this card is " + limit + " °C" : "";
    const ecc = dashEcc(hip);
    clocks.replaceChildren(
      pair("SCLK", card && card.clk != null ? dashGroup(card.clk) + " MHz" : "–"),
      pair("MCLK", card && card.mclk != null ? dashGroup(card.mclk) + " MHz" : "–"),
      ecc.pill);
    chart.update(dashCardSeries(hip, "busy"), 100);
    if (sample && sample.hip_index === hip) { /* the sample is in the card list */ }
  };
  el.update();
  return el;
}

// dashEcc is the ECC state of a card: the check of the doctor whose id is
// gpu.ecc.<hip index>.
function dashEcc(hip) {
  const checks = (dash.doctor && dash.doctor.checks) || [];
  const c = checks.find((x) => x.id === "gpu.ecc." + hip);
  if (!c) {
    return { pill: dashPill("skip", "unknown"), note: "The doctor has not checked the ECC counters yet." };
  }
  const kind = c.status === "ok" ? "ok" : c.status === "warn" ? "warn" : c.status === "fail" ? "fail" : "skip";
  return { pill: dashPill(kind, c.status === "skip" ? "unknown" : c.status), note: c.detail || "" };
}

// dashCards is the section with one card per card of the host.
function dashCards(section) {
  const grid = h("div.gpu-grid", { style: "display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,270px),1fr));gap:16px" });
  const note = h("div", { style: "color:var(--muted);font-size:13px;margin-bottom:14px" });
  section.append(note, grid);
  let built = [];
  const build = () => {
    const hw = (dash.hw && dash.hw.gpus) || [];
    const tele = (dash.tele && dash.tele.gpus) || [];
    const hips = hw.length ? hw.map((g) => g.hip_index) : tele.map((g) => g.hip_index);
    if (hips.length === built.length && hips.every((v, i) => v === built[i])) return false;
    built = hips;
    grid.replaceChildren();
    for (const hip of hips) grid.append(dashCard(hip));
    return true;
  };
  const api = {
    update: () => {
      if (!build()) for (const c of grid.children) if (c.update) c.update();
      const hips = built.length;
      note.textContent = hips ? hips + (hips === 1 ? " card on this host." : " cards on this host.") : "No AMD GPU is visible to bernie.";
    },
  };
  api.update();
  return api;
}

// renderGPUs repaints the cards of the host. The shell calls it when the
// window changes.
function renderGPUs() {
  if (dash.live.cards) dash.live.cards.update();
}

// dashCharts is the section with the four charts of the last minutes and the
// control that picks the window.
function dashCharts(section) {
  const head = h("div", { style: "display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:16px" });
  const seg = h("div", { style: "display:inline-flex;background:var(--surface-3);border-radius:9px;padding:3px", role: "group", "aria-label": "The window of the charts" });
  const buttons = {};
  for (const w of [{ id: "5m", label: "5 min" }, { id: "15m", label: "15 min" }, { id: "1h", label: "1 h" }]) {
    const b = h("button", { type: "button", "aria-pressed": String(dash.window === w.id),
      style: "border:0;background:transparent;padding:5px 12px;border-radius:7px;cursor:pointer;color:var(--text-2);font-size:13px;font-weight:600" }, w.label);
    b.addEventListener("click", () => { dash.window = w.id; paintSeg(); dashHistory(); });
    buttons[w.id] = b;
    seg.append(b);
  }
  const paintSeg = () => {
    for (const [id, b] of Object.entries(buttons)) {
      const on = id === dash.window;
      b.setAttribute("aria-pressed", String(on));
      b.style.background = on ? "var(--surface)" : "transparent";
      b.style.color = on ? "var(--text)" : "var(--text-2)";
      b.style.boxShadow = on ? "0 1px 2px rgba(0,0,0,.25)" : "none";
    }
  };
  paintSeg();
  head.append(seg);
  const grid = h("div", { style: "display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,260px),1fr));gap:16px" });
  const charts = {
    busy: areaChart([], { title: "Utilisation", unit: "%", windowMinutes: 5 }),
    pow: areaChart([], { title: "Power", unit: "W", windowMinutes: 5 }),
    mem: areaChart([], { title: "Memory", unit: "GiB", windowMinutes: 5 }),
    gen: areaChart([], { title: "Tokens per second", unit: "tok/s", windowMinutes: 5 }),
  };
  for (const c of Object.values(charts)) grid.append(c);
  section.append(head, grid);
  const api = {
    update: () => {
      const mins = dash.window === "1h" ? 60 : dash.window === "15m" ? 15 : 5;
      charts.busy.update(dashWindowSeries("busy"), mins);
      charts.pow.update(dashWindowSeries("pow"), mins);
      charts.mem.update(dashWindowSeries("mem"), mins);
      charts.gen.update(dashWindowSeries("gen"), mins);
    },
  };
  api.update();
  return api;
}
