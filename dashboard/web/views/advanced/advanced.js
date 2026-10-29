"use strict";
// ---------- Advanced ----------
// The Advanced tab asks one question: is this box serving, and how fast?
//
// Every number comes from an endpoint of spec §11. The live numbers come from
// one stream (GET /api/telemetry at 1 Hz) and the charts of the last minutes
// from GET /api/gpus/history. The view never opens a fetch or a timer of its
// own: the stream lives in js/data.js.
//
// Adapted from vllm-rdna-adv by CrispyGinger, used with permission.

const adv = {
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
  subscribed: false, // the repaint of this tab is registered on the stream
  grids: [],     // the grids that follow the width of the window
};

// advGrid keeps the columns of a block: the wide ones until the page is
// narrow, the narrow ones below. The shell stacks the columns of every tab
// at 959 px; the hero of this tab keeps its number on the left longer.
function advGrid(el, wide, narrow) {
  const apply = () => {
    el.style.gridTemplateColumns = window.innerWidth <= 959 ? narrow : wide;
  };
  apply();
  adv.grids.push(apply);
}

// advSection is one block of the tab: a rule on top, the name of the block
// and what it holds. The name is a heading the router can focus.
function advSection(label, id, ...kids) {
  return h("section.section", { id: "advanced-" + id, "data-section": id },
    h("h2.section-label", { tabindex: "-1" }, label),
    ...kids);
}

// advInfo is the small mark that opens the explanation of a number. The
// explanation is never the only copy of the fact: the tile says it too.
function advInfo(text) {
  if (typeof tooltip === "function") return tooltip(text, "What this number means");
  return h("span.tip", h("button", { type: "button", title: text, "aria-label": "What this number means" }, "i"));
}

// advGroup writes a number with a thin space between the thousands.
function advGroup(n) {
  const s = String(Math.round(n));
  return s.replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

// advPill is a status: an icon and a word, never the colour alone.
const ADV_PILL_ICON = { ok: "✓", warn: "!", serious: "▲", fail: "✕", skip: "–", info: "i", running: "◐" };
function advPill(kind, word) {
  return h("span.pill." + kind, h("span.pill-icon", ADV_PILL_ICON[kind] || "·"),
    h("span", word || kind));
}

// The stream lives in js/data.js; the shell calls advRenderGPUs() on every
// telemetry message. This view opens no fetch and no timer of its own.

// advPush keeps one compact sample of the stream and the fleet numbers it
// carries. The buffer holds one hour, so the 1 h window is always full.
function advPush(doc) {
  adv.tele = doc;
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
    if (g.temp_c != null && (hot == null || g.temp_c > hot)) { hot = g.temp_c; hotName = advCardLabel(g.hip_index); }
    if (g.sclk_mhz != null && (clk == null || g.sclk_mhz > clk)) { clk = g.sclk_mhz; clkName = advCardLabel(g.hip_index); }
    return { hip: g.hip_index, busy: g.busy_pct, pow: g.power_w, mem: g.vram_used,
      tmp: g.temp_c, jtmp: g.temp_junction_c, mtmp: g.temp_mem_c,
      clk: g.sclk_mhz, mclk: g.mclk_mhz, cap: g.power_cap_w, total: g.vram_total };
  });
  const stats = live && live.stats ? live.stats : null;
  adv.samples.push({
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
      prompt: live ? live.prompt_tok_s : null,
      running: stats ? stats.requests_running : null,
      waiting: stats ? stats.requests_waiting : null,
      kv: stats ? stats.kv_cache_usage : null,
    },
  });
  if (adv.samples.length > 3600) adv.samples.shift();
}

// advSeries is the last minute of one fleet number: the sparkline of a tile.
function advSeries(key, count) {
  const n = count || 60;
  return adv.samples.slice(-n).map((s) => (s.agg ? s.agg[key] : null));
}

// advCardSeries is the last minute of one card.
function advCardSeries(hip, key) {
  return adv.samples.slice(-60).map((s) => {
    const c = s.cards.find((x) => x.hip === hip);
    return c ? c[key] : null;
  });
}

// advCardName is the name of a card: the one of /api/hw, or its index.
function advCardName(hip) {
  const g = ((adv.hw && adv.hw.gpus) || []).find((x) => x.hip_index === hip);
  if (!g) return "GPU " + hip;
  return g.name || g.gfx || ("GPU " + hip);
}

// advCardLabel names a card for a line that says which card a number comes
// from: the index of the card and its name. Eight cards of the same model
// share a name, so the index is what tells them apart.
function advCardLabel(hip) {
  const name = advCardName(hip);
  return name === "GPU " + hip ? name : "GPU " + hip + " · " + name;
}

// advLiveRun is the run the tab talks about.
function advLiveRun() {
  const runs = adv.runs || [];
  return runs.find((r) => r.state !== "exited") || runs[runs.length - 1] || null;
}

// advPhase is the phase sentence of a run: what the log says it is doing.
function advPhase(run) {
  if (!run) return "";
  if (run.phase && run.phase.sentence) return run.phase.sentence;
  if (run.state === "starting") return "The server is loading the model.";
  return "";
}

// advSetHead writes the state headline: one sentence about the situation and
// one line about what to do next.
function advSetHead() {
  const head = adv.head;
  if (!head) return;
  const gpus = (adv.hw && adv.hw.gpus) || [];
  const run = advLiveRun();
  const live = adv.tele && (adv.tele.runs || []).find((r) => run && r.name === r.name);
  const serving = !!run && run.state !== "exited";
  if (adv.live.headStop) adv.live.headStop.disabled = !serving;
  if (adv.live.headRestart) adv.live.headRestart.disabled = !serving;
  if (!gpus.length) {
    setHead(head, "NO AMD GPU ON THIS MACHINE.",
      "bernie reads the cards from /sys/class/kfd. The amdgpu driver has to be loaded and /dev/kfd has to exist.");
    if (adv.live.modprobe) adv.live.modprobe.hidden = false;
    return;
  }
  if (adv.live.modprobe) adv.live.modprobe.hidden = true;
  if (!run) {
    setHead(head, "NOTHING IS SERVING.",
      "Start a model server on this host, then this tab fills with its numbers.");
  } else if (run.state === "starting" || (live && live.state === "starting")) {
    setHead(head, "THE SERVER IS LOADING THE MODEL.",
      advPhase(run) || "The server is loading the model.");
  } else if (live && !live.stats) {
    setHead(head, "THE SERVER DOES NOT ANSWER.",
      "The process is alive but /health stays silent. The log says why.");
  } else {
    const tok = live && live.gen_tok_s != null ? " at " + num(live.gen_tok_s) + " tok/s" : "";
    setHead(head, "THIS BOX IS SERVING.",
      "The model answers on port " + (run.port || "–") + tok + ".");
  }
}

// advAdvanced is the panel with the raw documents and the commands that
// make them.
function advAdvanced() {
  const docs = [
    ["/api/hw", adv.hw, "bernie hw"],
    ["/api/topo", adv.topo, "bernie topo"],
    ["/api/topo/check", adv.check, "bernie topo --check"],
    ["/api/doctor", adv.doctor, "bernie doctor"],
    ["/api/runs", adv.runs, "bernie ps"],
    ["/api/power", adv.power, "bernie tune power status"],
    ["/api/telemetry", adv.tele, "bernie status --json"],
  ];
  const body = h("div.advanced-body");
  for (const [path, doc, command] of docs) {
    body.append(h("h3", path), cmdBlock(command, "Copy the command"));
    const pre = h("pre", { class: "adv-raw-json" });
    pre.textContent = doc == null ? "not read yet" : JSON.stringify(doc, null, 2);
    body.append(pre);
  }
  return h("details.advanced", h("summary", "Advanced"), body);
}

// advLoad reads the documents the tab needs. One failure does not stop the
// others: the card that failed says so.
async function advLoad() {
  const one = async (path, key) => {
    try { adv[key] = await api(path); }
    catch (e) { adv[key] = null; adv.errors = adv.errors || {}; adv.errors[path] = e.message || String(e); }
  };
  await Promise.all([
    one("/api/hw", "hw"),
    one("/api/runs", "runs"),
    one("/api/doctor", "doctor"),
    one("/api/power", "power"),
    one("/api/topo", "topo"),
    one("/api/topo/check", "check"),
  ]);
  if (!Array.isArray(adv.runs)) adv.runs = [];
  advSetHead();
  if (adv.live.tiles) adv.live.tiles.update(adv.samples);
  if (adv.live.cards) adv.live.cards.update();
  if (adv.live.run) adv.live.run.update();
  if (adv.live.power) adv.live.power.update();
  if (adv.live.topo) adv.live.topo.update();
  if (adv.live.problems) adv.live.problems.update();
}

// advHistory reads the history of the cards for the window. The endpoint
// answers one series per card; the fleet numbers are the sum or the mean of
// the cards.
async function advHistory() {
  try {
    const doc = await api("/api/gpus/history?window=" + adv.window);
    adv.history = doc;
    adv.historyOK = true;
  } catch (e) {
    adv.history = null;
    adv.historyOK = false;
  }
  if (adv.live.charts) adv.live.charts.update();
}

// advWindowPrompt is the prefill rate of the live run over the window: the
// prompt tokens per second. The rate is a property of the server, so every
// card carries the same number; the first series of the history holds it.
function advWindowPrompt() {
  const mins = adv.window === "1h" ? 60 : adv.window === "15m" ? 15 : 5;
  if (adv.historyOK && adv.history && adv.history.series && adv.history.series.length) {
    const first = adv.history.series[0];
    return (first.prompt_tok_s || []).slice();
  }
  const want = mins * 60;
  return adv.samples.slice(-want).map((x) => (x.agg ? x.agg.prompt : null));
}

// advWindowSeries is the fleet series of one measure over the window: the
// history of the server when it has one, the ring buffer of the page when it
// does not.
function advWindowSeries(key, unit) {
  const mins = adv.window === "1h" ? 60 : adv.window === "15m" ? 15 : 5;
  if (adv.historyOK && adv.history && adv.history.series && adv.history.series.length) {
    const series = adv.history.series;
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
  const cut = adv.samples.slice(-want);
  return cut.map((s) => (s.agg ? s.agg[key] : null));
}

async function viewAdvanced(root) {
  put(root, loading());
  adv.grids = [];
  const stop = h("button", { type: "button", onclick: () => advStopRun() }, "Stop");
  const restart = h("button", { type: "button", onclick: () => advRestartRun() }, "Restart");
  const head = stateHead("Advanced", h("div", { class: "adv-head-actions" }, stop, restart));
  adv.head = head;
  adv.live.headStop = stop;
  adv.live.headRestart = restart;
  const modprobe = cmdBlock("sudo modprobe amdgpu", "Copy the command");
  modprobe.hidden = true;
  adv.live.modprobe = modprobe;
  head.append(modprobe);

  const hero = advHero();
  const cards = advSection("The cards", "gpus");
  const minutes = advSection("The last minutes", "minutes");
  const run = advSection("The run", "runs");
  const power = advSection("The power caps", "power");
  const topo = advSection("The topology", "topology");
  const problems = advSection("Known problems", "problems");
  put(root, head, hero.el, cards, minutes, run, power, topo, problems, advAdvanced());

  adv.live.cards = advCards(cards);
  adv.live.charts = advCharts(minutes);
  adv.live.run = advRunCard(run);
  adv.live.power = advPower(power);
  adv.live.topo = advTopology(topo);
  adv.live.problems = advProblems(problems);
  adv.live.tiles = hero.tiles;

  await advLoad();
  await advHistory();
  advSubscribe();
  advRenderGPUs();
}

// advSample is the stream: it keeps the sample and repaints the live parts.
function advSample(doc) {
  advPush(doc);
  advSetHead();
  if (adv.live.tiles) adv.live.tiles.update(adv.samples);
  if (adv.live.cards) adv.live.cards.update();
  if (adv.live.run) adv.live.run.update();
  if (adv.live.charts) adv.live.charts.update();
}
