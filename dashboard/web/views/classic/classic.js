"use strict";
// ---------- Classic ----------
// The Classic tab is the original dashboard design of the reference web UI,
// recovered as it is: the same hero, the same six tiles, the same two charts,
// the same GPU cards with their rings and meters, the same host panel and the
// same connect panel. It is here so that the owner of bernie and the author of
// the reference can put the two side by side and pick one.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
//
// The data is the data of this host: the shared telemetry stream of
// js/data.js (the stream the Dashboard tab reads too), GET /api/hw,
// GET /api/runs and GET /api/gpus/history. This tab opens no timer of its
// own. A number this host cannot measure shows a dash and says so: nothing
// here is invented.

// The tab is three files: this one holds the state and the model, panels.js
// holds the DOM and paint.js holds the update. The bundle is named in
// js/boot.js.

const classic = {
  built: false,
  panel: null,
  unsub: null,     // the way back from the one telemetry stream of the page
  hist: [],        // [{t, gen, prompt, running, waiting, kv, spec}] the last minutes
  snip: "curl",    // the connect snippet: curl or python
  charts: new Map(),
  counts: {},      // the last value of every counted number
  el: {},          // the parts the page updates
  tip: null,       // the tooltip of the charts and of the marks
};

const CLASSIC_DOCS = "https://github.com/opengfx1030/bernie/blob/main/docs/design-dashboard.md";
const CLASSIC_HIST = 240;   // the points of a chart: four minutes at one per second
const CLASSIC_TAIL = 60;    // the samples under a tile

/* ---------- the view ---------- */

/** viewClassic opens the tab. The router calls it with the panel of the tab. */
function viewClassic(panel) {
  classic.panel = panel;
  panel.classList.add("classic");
  if (!classic.built) {
    classicBuild();
    classic.built = true;
  }
  classicUpdate(state.tele);
  classicSubscribe();
  classicHistory();
}

/** classicSubscribe rides the one telemetry stream of the page. The view
 *  opens no stream of its own: it registers its repaint with the registry
 *  of js/data.js, and the page keeps one connection to /api/telemetry. The
 *  registry calls the callbacks of the open tab after every sample, in one
 *  frame, with the document of that sample. */
function classicSubscribe() {
  if (classic.unsub) return classic.unsub;
  if (typeof onTabFrame !== "function") return null;
  classic.unsub = onTabFrame("classic", (doc) => {
    if (!doc) return;
    classicSample(doc);
    classicUpdate(doc);
  });
  return classic.unsub;
}

/** classicHistory reads the five minutes of the server's output and prefill
 *  rates. The ring buffer of the server holds them (tokens_per_s and
 *  prompt_tok_s of every series); the live samples then keep the charts
 *  moving on top of that history. */
async function classicHistory() {
  try {
    const doc = await api("/api/gpus/history?window=5m");
    const series = (doc && doc.series) || [];
    if (!series.length) return;
    const vals = series[0].tokens_per_s || [];
    const prompts = series[0].prompt_tok_s || [];
    const step = (doc.interval_s || 2) * 1000;
    const now = Date.now();
    const seeded = vals.map((v, i) => ({
      t: now - (vals.length - 1 - i) * step,
      gen: v == null ? null : v,
      prompt: i < prompts.length && prompts[i] != null ? prompts[i] : null,
    }));
    // The seeded points are older than the live samples; the buffer stays in
    // the order of the clock, so the charts read from the old to the new.
    const oldest = classic.hist.length ? classic.hist[0].t : Infinity;
    const merged = classic.hist.concat(seeded.filter((p) => p.t < oldest));
    merged.sort((a, b) => a.t - b.t);
    classic.hist = merged.slice(-CLASSIC_HIST);
    if (typeof state !== "undefined" && state.tab === "classic") classicUpdate(state.tele);
  } catch (e) {
    // The chart starts empty and fills from the live stream.
  }
}

/** classicSample keeps the last minutes of the run in the buffer. */
function classicSample(doc) {
  const run = (doc.runs || []).find((r) => r.state === "running") || (doc.runs || [])[0];
  const stats = run && run.stats ? run.stats : null;
  classic.hist.push({
    t: doc.time || Date.now(),
    gen: run ? run.gen_tok_s : null,
    prompt: run ? run.prompt_tok_s : null,
    running: stats ? stats.requests_running : null,
    waiting: stats ? stats.requests_waiting : null,
    kv: stats ? stats.kv_cache_usage * 100 : null,
    spec: stats && stats.spec_draft_tokens_total > 0 ? stats.spec_accept_rate * 100 : null,
  });
  if (classic.hist.length > CLASSIC_HIST) classic.hist.shift();
}

/* ---------- the model: bernie's data in the shape of the reference -------- */

/** classicModel reads the state of the host and answers the questions the
 *  reference's dashboard view asks. The document is the one of the sample
 *  that arrived; without one the last document of the shared state is
 *  used. */
function classicModel(doc) {
  const tele = doc || (typeof state !== "undefined" ? state.tele : null) || { gpus: [], runs: [] };
  const hw = (typeof state !== "undefined" && state.hw) || null;
  const list = (typeof state !== "undefined" && state.runs) || [];
  const runs = tele.runs || [];
  const live = runs.find((r) => r.state === "running") || null;
  const starting = runs.find((r) => r.state === "starting") || null;
  const record = live || starting || null;
  const stats = live && live.stats ? live.stats : null;
  const runList = record ? (list.find((r) => r.name === record.name) || null) : null;

  // the word of the state, the way the reference words it
  let word = "Off";
  let dot = "idle";
  if (live) { word = "Serving"; dot = "ok"; }
  else if (starting) { word = "Starting"; dot = "run"; }

  // what is running: the recipe, the mode and the port
  const layout = record
    ? [record.name, runList && runList.mode, runList && runList.port ? "port " + runList.port : null]
      .filter(Boolean).join(" · ")
    : "no run on this host";

  // the address other machines use
  let url = "";
  if (record) {
    url = record.url || (runList ? runURL(runList) : "");
    if (url && !/\/v1$/.test(url)) url = url.replace(/\/$/, "") + "/v1";
  }

  const hostInfo = (hw && hw.host) || null;
  const hwGpus = (hw && hw.gpus) || [];
  const names = {};
  for (const g of hwGpus) names[g.hip_index] = g.name || g.gfx || "";

  return {
    word: word,
    dot: dot,
    live: !!live,
    starting: !!starting,
    record: record,
    runList: runList,
    stats: stats,
    // the line under the state: what the run is doing right now. The words
    // of the reference for the case where nothing serves.
    sub: live ? "is answering requests" : (starting ? "is loading the model" : "The model server is not running."),
    layout: layout,
    url: url,
    model: record ? record.name : "",
    gpus: (tele.gpus || []).map((g) => ({
      index: g.hip_index,
      name: names[g.hip_index] || "",
      util: g.busy_pct,
      vramUsed: g.vram_used,
      vramTotal: g.vram_total,
      power: g.power_w,
      powerCap: g.power_cap_w,
      temp: g.temp_c,
      tempMem: g.temp_mem_c,
      sclk: g.sclk_mhz,
    })),
    host: hostInfo,
    hostname: hostInfo ? hostInfo.hostname : "",
    ramTotal: hostInfo ? hostInfo.ram_bytes : null,
  };
}

/** classicLive is the number of the reference: the live value, or the last
 *  real one while the server is idle, with the note that says which. */
function classicLive(key, digits) {
  const hist = classic.hist;
  const cur = hist.length ? hist[hist.length - 1][key] : null;
  if (cur != null && cur > 0) {
    const now = Date.now() / 1000;
    const busy = hist.filter((s) => s.t / 1000 > now - 60 && s[key] != null && s[key] > 0).map((s) => s[key]);
    const avg = busy.length ? busy.reduce((a, b) => a + b, 0) / busy.length : null;
    return { v: cur, note: avg != null ? "1-min avg " + cfmt(avg, digits) : null, idle: false };
  }
  for (let i = hist.length - 1; i >= 0; i--) {
    const v = hist[i][key];
    if (v != null && v > 0) return { v, note: "idle · last " + cago(hist[i].t / 1000) + " ago", idle: true };
  }
  return { v: cur, note: null, idle: false };
}

/** classicTail is the last samples of one number: the line under a tile. */
function classicTail(key) {
  return classic.hist.slice(-CLASSIC_TAIL).map((s) => s[key]);
}
