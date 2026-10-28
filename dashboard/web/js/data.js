// Dashboard data layer: one fetch helper, one shared state and one place for
// every server stream. Views never open fetch or EventSource themselves.
"use strict";

const HISTORY = 60; // seconds of sparkline history

const state = {
  hw: null,          // /api/hw
  tele: null,        // last telemetry event
  hist: {},          // key -> number[] (last HISTORY samples)
  tab: "home",
  power: null,       // /api/power
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
  return { status: response.status, ok: response.ok, data, response };
}

/** api fetches JSON; mutating methods carry the X-Bernie header (CSRF guard). */
async function api(path, opts = {}) {
  const result = await request(path, opts);
  if (!result.ok) {
    throw new Error((result.data && result.data.error) || result.response.status + " " + result.response.statusText);
  }
  return result.data;
}

function pushHist(key, v) {
  const a = state.hist[key] || (state.hist[key] = []);
  a.push(v || 0);
  if (a.length > HISTORY) a.shift();
  return a;
}

async function loadHW(force) {
  if (!state.hw || force) {
    state.hw = await api("/api/hw");
    $("#hostname").textContent = state.hw.host && state.hw.host.hostname || "";
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

/** openJobEvents follows the output of one onboarding job. */
function openJobEvents(id, handlers = {}) {
  const es = new EventSource("/api/jobs/" + encodeURIComponent(id) + "/events");
  es.addEventListener("line", (ev) => handlers.line && handlers.line(ev));
  es.addEventListener("done", (ev) => handlers.done && handlers.done(ev));
  es.addEventListener("failed", (ev) => handlers.failed && handlers.failed(ev));
  return es;
}

/** openRunLogs follows the log of one run. */
function openRunLogs(name, handlers = {}) {
  const es = new EventSource(`/api/runs/${encodeURIComponent(name)}/logs`);
  es.onopen = () => handlers.open && handlers.open();
  es.onerror = () => handlers.disconnect && handlers.disconnect();
  es.addEventListener("end", (ev) => handlers.end && handlers.end(ev));
  es.addEventListener("error", (ev) => handlers.error && handlers.error(ev));
  es.onmessage = (ev) => handlers.message && handlers.message(ev);
  return es;
}

let telemetryStream = null;

function startTelemetry() {
  if (telemetryStream) return;
  telemetryStream = new EventSource("/api/telemetry");
  telemetryStream.onopen = () => $("#live").classList.add("on");
  telemetryStream.onerror = () => $("#live").classList.remove("on");
  telemetryStream.onmessage = (ev) => {
    const t = JSON.parse(ev.data);
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
    if (state.tab === "dashboard") renderGPUs();
  };
}
