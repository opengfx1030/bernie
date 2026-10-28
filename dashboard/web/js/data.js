// bernie dashboard: the data layer.
// One fetch helper, one state, one poller and the three streams of the
// server. A view never opens a fetch or a timer of its own.
"use strict";

const HISTORY = 60; // seconds of sparkline history
const RUN_POLL_MS = 5000; // how often the run list is read for the pill

const state = {
  hw: null,          // /api/hw
  tele: null,        // the last telemetry event
  hist: {},          // key -> number[] (the last HISTORY samples)
  tab: "setup",      // the tab that is open
  power: null,       // /api/power
  runs: [],          // /api/runs
};

/** request performs one JSON request and returns its status and document.
 *  api below turns a failed request into an Error. */
async function request(path, opts = {}) {
  const init = { method: opts.method || "GET", headers: {}, credentials: "same-origin" };
  if (init.method !== "GET") init.headers["X-Bernie"] = "1";
  if (opts.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(opts.body);
  }
  const response = await fetch(path, init);
  const text = await response.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  return { status: response.status, ok: response.ok, data, response, text };
}

/** api fetches JSON; a mutating method carries the X-Bernie header (the CSRF
 *  guard of spec §11). */
async function api(path, opts = {}) {
  const result = await request(path, opts);
  if (!result.ok) {
    const err = new Error((result.data && result.data.error) || result.response.status + " " + result.response.statusText);
    // The card that shows the error names the endpoint and the raw answer.
    err.path = path;
    err.status = result.status;
    err.raw = result.text;
    throw err;
  }
  return result.data;
}

/* ---------- the host ---------- */

/** loadHW reads the host once and keeps it. The rail shows the name of it. */
async function loadHW(force) {
  if (!state.hw || force) {
    state.hw = await api("/api/hw");
    const name = $("#hostname");
    if (name) name.textContent = (state.hw.host && state.hw.host.hostname) || "";
    // The environment can limit the GPUs bernie uses (spec §7.7): say so.
    const note = $("#gpunote");
    if (note) note.textContent = state.hw.restriction ? "· " + state.hw.restriction : "";
  }
  return state.hw;
}

function gpuName(i) {
  const g = state.hw && (state.hw.gpus || []).find((g) => g.hip_index === i);
  return g ? g.name || g.gfx : "";
}

function runsOf(data) { return Array.isArray(data) ? data : (data && data.runs) || []; }
function runURL(r) { return "http://" + (r.host || "127.0.0.1") + ":" + r.port; }

/* ---------- the history of the live numbers ---------- */

function pushHist(key, v) {
  const a = state.hist[key] || (state.hist[key] = []);
  a.push(v || 0);
  if (a.length > HISTORY) a.shift();
  return a;
}

/* ---------- the streams ---------- */

/** openTelemetry follows the live sample of the host. It is the one stream
 *  that drives every tile, every ring and every sparkline of the page. */
function openTelemetry(handlers = {}) {
  const es = new EventSource("/api/telemetry");
  es.onopen = () => handlers.open && handlers.open();
  es.onerror = () => handlers.error && handlers.error();
  es.onmessage = (ev) => {
    let doc = null;
    try { doc = JSON.parse(ev.data); } catch { return; }
    handlers.message && handlers.message(doc, ev);
  };
  return es;
}

/** openRunLogs follows the log of one run. */
function openRunLogs(name, handlers = {}) {
  const es = new EventSource("/api/runs/" + encodeURIComponent(name) + "/logs");
  es.onopen = () => handlers.open && handlers.open();
  es.onerror = () => handlers.disconnect && handlers.disconnect();
  es.addEventListener("end", (ev) => handlers.end && handlers.end(ev));
  es.addEventListener("error", (ev) => handlers.error && handlers.error(ev));
  es.onmessage = (ev) => handlers.message && handlers.message(ev);
  return es;
}

/** openJobEvents follows the output of one onboarding job. */
function openJobEvents(id, handlers = {}) {
  const es = new EventSource("/api/jobs/" + encodeURIComponent(id) + "/events");
  es.addEventListener("line", (ev) => handlers.line && handlers.line(ev));
  es.addEventListener("done", (ev) => handlers.done && handlers.done(ev));
  es.addEventListener("failed", (ev) => handlers.failed && handlers.failed(ev));
  return es;
}

/* ---------- the one poller of the page ---------- */

let telemetryStream = null;
let runTimer = null;

/** startTelemetry opens the live stream and keeps the history of the page. */
function startTelemetry() {
  if (telemetryStream) return telemetryStream;
  telemetryStream = openTelemetry({
    message: (t) => {
      state.tele = t;
      const gs = t.gpus || [];
      let pw = 0, busy = 0, vram = 0, hot = 0, sclk = 0;
      for (const s of gs) {
        pushHist("busy" + s.hip_index, s.busy_pct);
        pushHist("pow" + s.hip_index, s.power_w);
        pw += s.power_w || 0;
        busy += s.busy_pct || 0;
        vram += s.vram_used || 0;
        if ((s.temp_c || 0) > hot) hot = s.temp_c || 0;
        if ((s.sclk_mhz || 0) > sclk) sclk = s.sclk_mhz || 0;
      }
      if (gs.length) {
        pushHist("busy_avg", busy / gs.length);
        pushHist("power_total", pw);
        pushHist("vram_used", vram / GiB);
        pushHist("temp_max", hot);
        pushHist("sclk_max", sclk);
      }
      for (const r of t.runs || []) pushHist("gen:" + r.name, r.gen_tok_s);
      if (state.tab === "dashboard" && typeof renderGPUs === "function") renderGPUs();
    },
  });
  return telemetryStream;
}

/** pollRuns reads the run list. The pill in the rail answers with it. */
async function pollRuns() {
  try {
    const runs = runsOf(await api("/api/runs"));
    state.runs = runs;
    setRunPill(runs);
  } catch (e) {
    // The pill keeps its last word when the server does not answer.
  }
}

/** startPolling opens the one timer of the page. */
function startPolling() {
  if (runTimer) return;
  pollRuns();
  runTimer = setInterval(pollRuns, RUN_POLL_MS);
}
