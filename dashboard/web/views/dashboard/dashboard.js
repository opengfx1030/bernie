"use strict";
// ---------- Dashboard ----------
// The Dashboard tab asks one question: is this box serving, and how fast?
//
// Every number comes from an endpoint of spec §11. The live numbers come from
// one stream (GET /api/telemetry at 1 Hz) and the charts of the last minutes
// from GET /api/gpus/history. The view never opens a fetch or a timer of its
// own: the stream lives in js/data.js.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.

const dash = {
  samples: [],   // the compact telemetry ring buffer (one hour of samples)
  window: "5m",  // the window of the charts: 5m, 15m or 1h
  history: null, // the answer of GET /api/gpus/history
  historyOK: false,
  hw: null,      // /api/hw
  runs: [],      // /api/runs
  doctor: null,  // /api/doctor
  power: null,   // /api/power
  topo: null,    // /api/topo
  check: null,   // /api/topo/check
  tele: null,    // the last telemetry document
  head: null,    // the state headline of the tab
  live: {},      // the parts the stream repaints
  grids: [],     // the grids that follow the width of the window
};

// dashGrid keeps the columns of a block: the wide ones until the page is
// narrow, the narrow ones below. The shell stacks the columns of every tab
// at 959 px; the hero of this tab keeps its number on the left longer.
function dashGrid(el, wide, narrow) {
  const apply = () => {
    el.style.gridTemplateColumns = window.innerWidth <= 959 ? narrow : wide;
  };
  apply();
  dash.grids.push(apply);
}

// dashSection is one block of the tab: a rule on top, the name of the block
// and what it holds. The name is a heading the router can focus.
function dashSection(label, id, ...kids) {
  return h("section.section", { id: "dashboard-" + id, "data-section": id },
    h("h2.section-label", { tabindex: "-1" }, label),
    ...kids);
}

// dashInfo is the small mark that opens the explanation of a number. The
// explanation is never the only copy of the fact: the tile says it too.
function dashInfo(text) {
  if (typeof tooltip === "function") return tooltip(text, "What this number means");
  return h("span.tip", h("button", { type: "button", title: text, "aria-label": "What this number means" }, "i"));
}

// dashGroup writes a number with a thin space between the thousands.
function dashGroup(n) {
  const s = String(Math.round(n));
  return s.replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

// dashPill is a status: an icon and a word, never the colour alone.
const DASH_PILL_ICON = { ok: "✓", warn: "!", serious: "▲", fail: "✕", skip: "–", info: "i", running: "◐" };
function dashPill(kind, word) {
  return h("span.pill." + kind, h("span.pill-icon", DASH_PILL_ICON[kind] || "·"),
    h("span", word || kind));
}

// The stream lives in js/data.js; the shell calls renderGPUs() on every
// telemetry message. This view opens no fetch and no timer of its own.

// dashPush keeps one compact sample of the stream and the fleet numbers it
// carries. The buffer holds one hour, so the 1 h window is always full.
function dashPush(doc) {
  dash.tele = doc;
  const gpus = doc.gpus || [];
  const runs = doc.runs || [];
  const live = runs.find((r) => r.state !== "exited") || runs[0] || null;
  let busy = 0, pow = 0, mem = 0, cap = 0, total = 0, hot = null, clk = null, hotName = "", clkName = "";
  const cards = gpus.map((g) => {
    busy += g.busy_pct || 0;
    pow += g.power_w || 0;
    mem += g.vram_used || 0;
    cap += g.power_cap_w || 0;
    total += g.vram_total || 0;
    if (g.temp_c != null && (hot == null || g.temp_c > hot)) { hot = g.temp_c; hotName = dashCardName(g.hip_index); }
    if (g.sclk_mhz != null && (clk == null || g.sclk_mhz > clk)) { clk = g.sclk_mhz; clkName = dashCardName(g.hip_index); }
    return { hip: g.hip_index, busy: g.busy_pct, pow: g.power_w, mem: g.vram_used,
      tmp: g.temp_c, jtmp: g.temp_junction_c, mtmp: g.temp_mem_c,
      clk: g.sclk_mhz, mclk: g.mclk_mhz, cap: g.power_cap_w, total: g.vram_total };
  });
  const stats = live && live.stats ? live.stats : null;
  dash.samples.push({
    t: doc.time || Date.now(),
    cards: cards,
    agg: {
      busy: cards.length ? busy / cards.length : null,
      pow: cards.length ? pow : null,
      mem: cards.length ? mem : null,
      cap: cards.length ? cap : null,
      total: cards.length ? total : null,
      hot: hot, hotName: hotName,
      clk: clk, clkName: clkName,
      gen: live ? live.gen_tok_s : null,
      running: stats ? stats.requests_running : null,
      waiting: stats ? stats.requests_waiting : null,
      kv: stats ? stats.kv_cache_usage : null,
    },
  });
  if (dash.samples.length > 3600) dash.samples.shift();
}

// dashSeries is the last minute of one fleet number: the sparkline of a tile.
function dashSeries(key, count) {
  const n = count || 60;
  return dash.samples.slice(-n).map((s) => (s.agg ? s.agg[key] : null));
}

// dashCardSeries is the last minute of one card.
function dashCardSeries(hip, key) {
  return dash.samples.slice(-60).map((s) => {
    const c = s.cards.find((x) => x.hip === hip);
    return c ? c[key] : null;
  });
}

// dashCardName is the name of a card: the one of /api/hw, or its index.
function dashCardName(hip) {
  const g = ((dash.hw && dash.hw.gpus) || []).find((x) => x.hip_index === hip);
  if (!g) return "GPU " + hip;
  return g.name || g.gfx || ("GPU " + hip);
}

// dashLiveRun is the run the tab talks about.
function dashLiveRun() {
  const runs = dash.runs || [];
  return runs.find((r) => r.state !== "exited") || runs[runs.length - 1] || null;
}

// dashPhase is the phase sentence of a run: what the log says it is doing.
function dashPhase(run) {
  if (!run) return "";
  if (run.phase && run.phase.sentence) return run.phase.sentence;
  if (run.state === "starting") return "The server is loading the model.";
  return "";
}

// dashSetHead writes the state headline: one sentence about the situation and
// one line about what to do next.
function dashSetHead() {
  const head = dash.head;
  if (!head) return;
  const gpus = (dash.hw && dash.hw.gpus) || [];
  const run = dashLiveRun();
  const live = dash.tele && (dash.tele.runs || []).find((r) => run && r.name === r.name);
  const serving = !!run && run.state !== "exited";
  if (dash.live.headStop) dash.live.headStop.disabled = !serving;
  if (dash.live.headRestart) dash.live.headRestart.disabled = !serving;
  if (!gpus.length) {
    setHead(head, "NO AMD GPU ON THIS MACHINE.",
      "bernie reads the cards from /sys/class/kfd. The amdgpu driver has to be loaded and /dev/kfd has to exist.");
    if (dash.live.modprobe) dash.live.modprobe.hidden = false;
    return;
  }
  if (dash.live.modprobe) dash.live.modprobe.hidden = true;
  if (!run) {
    setHead(head, "NOTHING IS SERVING.",
      "Start a model server on this host, then this tab fills with its numbers.");
  } else if (run.state === "starting" || (live && live.state === "starting")) {
    setHead(head, "THE SERVER IS LOADING THE MODEL.",
      dashPhase(run) || "The server is loading the model.");
  } else if (live && !live.stats) {
    setHead(head, "THE SERVER DOES NOT ANSWER.",
      "The process is alive but /health stays silent. The log says why.");
  } else {
    const tok = live && live.gen_tok_s != null ? " at " + num(live.gen_tok_s) + " tok/s" : "";
    setHead(head, "THIS BOX IS SERVING.",
      "The model answers on port " + (run.port || "–") + tok + ".");
  }
}

// dashAdvanced is the panel with the raw documents and the commands that
// make them.
function dashAdvanced() {
  const docs = [
    ["/api/hw", dash.hw, "bernie hw"],
    ["/api/topo", dash.topo, "bernie topo"],
    ["/api/topo/check", dash.check, "bernie topo --check"],
    ["/api/doctor", dash.doctor, "bernie doctor"],
    ["/api/runs", dash.runs, "bernie ps"],
    ["/api/power", dash.power, "bernie tune power status"],
    ["/api/telemetry", dash.tele, "bernie status --json"],
  ];
  const body = h("div.advanced-body");
  for (const [path, doc, command] of docs) {
    body.append(h("h3", path), cmdBlock(command, "Copy the command"));
    const pre = h("pre", { class: "dash-raw-json" });
    pre.textContent = doc == null ? "not read yet" : JSON.stringify(doc, null, 2);
    body.append(pre);
  }
  return h("details.advanced", h("summary", "Advanced"), body);
}

// dashLoad reads the documents the tab needs. One failure does not stop the
// others: the card that failed says so.
async function dashLoad() {
  const one = async (path, key) => {
    try { dash[key] = await api(path); }
    catch (e) { dash[key] = null; dash.errors = dash.errors || {}; dash.errors[path] = e.message || String(e); }
  };
  await Promise.all([
    one("/api/hw", "hw"),
    one("/api/runs", "runs"),
    one("/api/doctor", "doctor"),
    one("/api/power", "power"),
    one("/api/topo", "topo"),
    one("/api/topo/check", "check"),
  ]);
  if (!Array.isArray(dash.runs)) dash.runs = [];
  dashSetHead();
  if (dash.live.tiles) dash.live.tiles.update(dash.samples);
  if (dash.live.cards) dash.live.cards.update();
  if (dash.live.run) dash.live.run.update();
  if (dash.live.power) dash.live.power.update();
  if (dash.live.topo) dash.live.topo.update();
  if (dash.live.problems) dash.live.problems.update();
}

// dashHistory reads the history of the cards for the window. The endpoint
// answers one series per card; the fleet numbers are the sum or the mean of
// the cards.
async function dashHistory() {
  try {
    const doc = await api("/api/gpus/history?window=" + dash.window);
    dash.history = doc;
    dash.historyOK = true;
  } catch (e) {
    dash.history = null;
    dash.historyOK = false;
  }
  if (dash.live.charts) dash.live.charts.update();
}

// dashWindowSeries is the fleet series of one measure over the window: the
// history of the server when it has one, the ring buffer of the page when it
// does not.
function dashWindowSeries(key, unit) {
  const mins = dash.window === "1h" ? 60 : dash.window === "15m" ? 15 : 5;
  if (dash.historyOK && dash.history && dash.history.series && dash.history.series.length) {
    const series = dash.history.series;
    const n = series[0].busy_pct ? series[0].busy_pct.length : 0;
    const out = [];
    for (let i = 0; i < n; i++) {
      if (key === "busy") {
        let sum = 0;
        for (const s of series) sum += (s.busy_pct && s.busy_pct[i]) || 0;
        out.push(series.length ? sum / series.length : null);
      } else if (key === "pow") {
        let sum = 0;
        for (const s of series) sum += (s.power_w && s.power_w[i]) || 0;
        out.push(series.length ? sum : null);
      } else if (key === "mem") {
        let sum = 0;
        for (const s of series) sum += (s.vram_used && s.vram_used[i]) || 0;
        out.push(series.length ? sum / (1024 * 1024 * 1024) : null);
      } else {
        out.push((series[0].tokens_per_s && series[0].tokens_per_s[i]) || 0);
      }
    }
    return out;
  }
  // The server has no history yet: the samples the page kept itself.
  const want = mins * 60;
  const cut = dash.samples.slice(-want);
  return cut.map((s) => (s.agg ? s.agg[key] : null));
}

async function viewDashboard(root) {
  put(root, loading());
  dash.grids = [];
  const stop = h("button", { type: "button", onclick: () => dashStopRun() }, "Stop");
  const restart = h("button", { type: "button", onclick: () => dashRestartRun() }, "Restart");
  const head = stateHead("Dashboard", h("div", { class: "dash-head-actions" }, stop, restart));
  dash.head = head;
  dash.live.headStop = stop;
  dash.live.headRestart = restart;
  const modprobe = cmdBlock("sudo modprobe amdgpu", "Copy the command");
  modprobe.hidden = true;
  dash.live.modprobe = modprobe;
  head.append(modprobe);

  const hero = dashHero();
  const cards = dashSection("The cards", "gpus");
  const minutes = dashSection("The last minutes", "minutes");
  const run = dashSection("The run", "runs");
  const power = dashSection("The power caps", "power");
  const topo = dashSection("The topology", "topology");
  const problems = dashSection("Known problems", "problems");
  put(root, head, hero.el, cards, minutes, run, power, topo, problems, dashAdvanced());

  dash.live.cards = dashCards(cards);
  dash.live.charts = dashCharts(minutes);
  dash.live.run = dashRunCard(run);
  dash.live.power = dashPower(power);
  dash.live.topo = dashTopology(topo);
  dash.live.problems = dashProblems(problems);
  dash.live.tiles = hero.tiles;

  await dashLoad();
  await dashHistory();
  renderGPUs();
}

// dashSample is the stream: it keeps the sample and repaints the live parts.
function dashSample(doc) {
  dashPush(doc);
  dashSetHead();
  if (dash.live.tiles) dash.live.tiles.update(dash.samples);
  if (dash.live.cards) dash.live.cards.update();
  if (dash.live.run) dash.live.run.update();
  if (dash.live.charts) dash.live.charts.update();
}
