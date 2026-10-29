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
  const el = h("div.tile", { class: "dash-tile" });
  const label = h("div.tile-l", spec.label);
  if (spec.tip) label.append(dashInfo(spec.tip));
  const value = h("div.tile-n");
  const amount = h("span", spec.value);
  const unit = h("small", spec.unit || "");
  value.append(amount, unit);
  const sub = h("div.tile-s", spec.sub || "");
  const sparkBox = h("div", { class: "dash-tile-spark" });
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
  const box = h("div.tiles", { class: "dash-tiles" });
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
  box.items = tiles;
  box.update = () => dashTilesPaint(box);
  box.update();
  return box;
}

// dashHero is the block at the top of the tab: the output speed of the live
// run on the left, the tiles of the fleet on the right.
function dashHero() {
  const el = h("div.dashboard-hero", { class: "dash-hero-start" });
  const left = h("div", { class: "dash-min0" });
  const eyebrow = h("div.eyebrow", "Output speed");
  const value = h("div", { class: "dash-hero-num" });
  const amount = h("span", "–");
  const unit = h("small", { class: "dash-hero-unit" }, "tok/s");
  value.append(amount, unit);
  const note = h("div", { class: "dash-hero-note" }, "No run is serving.");
  const chartBox = h("div", { class: "dash-hero-spark" });
  const chart = spark([], null, "the output speed of the live run", { unit: "tok/s" });
  chartBox.append(chart);
  left.append(eyebrow, value, note, chartBox);
  const tiles = dashTiles();
  el.append(left, tiles);
  dashGrid(el, "minmax(190px,320px) minmax(0,1fr)", "minmax(0,1fr)");
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
  tiles.update = () => {
    dashTilesPaint(tiles);
    paint();
  };
  return { el: el, tiles: tiles, paint: paint };
}

// dashTilesPaint repaints the tiles with the samples of the buffer.
function dashTilesPaint(box) {
  const tiles = box.items || [];
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
  const el = h("div.card", { class: "dash-card" });
  const top = h("div.gpu-top", { class: "dash-card-top" });
  const names = h("div", { class: "dash-min0" });
  const name = h("h3", { class: "dash-card-name" }, dashCardName(hip));
  const where = h("div.small-t", { class: "dash-card-where" }, (hw ? hw.pci : "") + (hw && hw.gfx ? " · " + hw.gfx : ""));
  names.append(name, where);
  top.append(names);
  const mid = h("div.gpu-mid", { class: "dash-card-mid" });
  const ringBox = ring(null, "GPU " + hip);
  const stats = h("div.gpu-stats", { class: "dash-card-stats" });
  const mem = meter(null, null, { label: "Memory", unit: "GiB", format: (v) => num(v / (1024 * 1024 * 1024)) });
  const pow = meter(null, null, { label: "Power", unit: "W", sep: " of ", format: (v) => num(Math.round(v)) });
  const temps = h("div.stat-row", { class: "dash-stat-row" });
  const clocks = h("div.stat-row", { class: "dash-stat-row-center" });
  const hot = h("div", { class: "dash-stat-hot" });
  stats.append(mem, pow, temps, hot, clocks);
  mid.append(ringBox, stats);
  const sparkBox = h("div.gpu-spark", { class: "dash-mt12" });
  const chart = spark([], 100, "the load of GPU " + hip, { unit: "%" });
  sparkBox.append(chart);
  el.append(top, mid, sparkBox);

  const pair = (label, value, cls) => h("span", { class: "dash-nowrap" },
    h("span", { class: "dash-muted" }, label + " "),
    h("b", { class: "dash-stat-value" + (cls ? " " + cls : "") }, value));

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
      pair("TEMP", t == null ? "–" : num(t) + " °C", isHot ? "dash-stat-serious" : ""),
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
  const grid = h("div", { class: "dash-cards" });
  const note = h("div", { class: "dash-cards-note" });
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

// renderGPUs subscribes to the shared stream through the onTabFrame seam.
// The stream lives in js/data.js; this view opens no fetch and no timer of
// its own. One stream, one frame per sample, for the whole page. A hidden
// tab paints nothing: data.js pauses the poller and skips the frame.
function renderGPUs() {
  for (const f of dash.grids) f();
  if (dash.live.cards) dash.live.cards.update();
}

// dashSubscribe registers the repaint of this tab on the shared stream. The
// stream lives in js/data.js; this view opens no fetch and no timer of its
// own. One stream, one frame per sample. The subscription happens once: the
// shell calls renderGPUs() on every sample and on a resize.
function dashSubscribe() {
  if (dash.subscribed) return;
  dash.subscribed = true;
  if (typeof onTabFrame !== "function") return;
  onTabFrame("dashboard", (doc) => { if (doc) dashSample(doc); });
}

// dashCharts is the section with the four charts of the last minutes and the
// control that picks the window.
function dashCharts(section) {
  const head = h("div", { class: "dash-section-head" });
  const seg = h("div.segmented-control", { role: "group", "aria-label": "The window of the charts" });
  const buttons = {};
  for (const w of [{ id: "5m", label: "5 min" }, { id: "15m", label: "15 min" }, { id: "1h", label: "1 h" }]) {
    const b = h("button", { type: "button", "aria-pressed": String(dash.window === w.id) }, w.label);
    b.addEventListener("click", () => { dash.window = w.id; paintSeg(); dashHistory(); });
    buttons[w.id] = b;
    seg.append(b);
  }
  const paintSeg = () => {
    for (const [id, b] of Object.entries(buttons)) b.setAttribute("aria-pressed", String(id === dash.window));
  };
  paintSeg();
  head.append(seg);
  const grid = h("div", { class: "dash-cards-grid" });
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
