"use strict";
// ---------- Advanced ----------
// The Advanced tab is the first bernie dashboard, kept as it is: the cards
// with their tiles, the PCIe topology with the P2P checklist, and the runs
// that are serving. It is drawn with the current design - the tokens, the
// fonts and the shared components of the shell - and it reads the same
// answers as every other tab.
//
// The stream is the one of the page: js/data.js owns it and this view
// registers its repaint with onTabFrame. It opens no stream of its own.

const adv = {
  samples: [],   // the compact telemetry buffer (the sparklines of the tiles)
  hw: null,      // /api/hw
  power: null,   // /api/power
  doctor: null,  // /api/doctor
  server: null,  // the live run of the stream and what its server measures
  unsub: null,   // the way back from the shared stream
  live: {},      // the parts the stream repaints
};

/** viewAdvanced opens the tab. The router calls it with the panel of the
 *  tab. */
async function viewAdvanced(root) {
  put(root, loading());
  const gpus = h("section.gpus-section");
  const topology = h("section.topology-section");
  const overview = h("section.runs-overview");
  const measures = h("section.runs-overview");
  put(root, gpus, topology, overview, measures);
  adv.live.gpus = advViewGPUs(gpus);
  adv.live.topo = advViewTopology(topology);
  adv.live.runs = advRunsOverview(overview);
  adv.live.measures = advServerMeasures(measures);
  advLoad();
  advSubscribe();
}

/** advLoad reads the documents the tab needs. One failure does not stop the
 *  others: the card that failed says so. */
async function advLoad() {
  const one = async (path, key) => {
    try { adv[key] = await api(path); }
    catch (e) { adv[key] = null; adv.errors = adv.errors || {}; adv.errors[path] = e.message || String(e); }
  };
  await Promise.all([
    one("/api/hw", "hw"),
    one("/api/power", "power"),
    one("/api/doctor", "doctor"),
  ]);
  if (adv.live.gpus) adv.live.gpus.update();
}

/** advSubscribe registers the repaint of this tab on the shared stream. The
 *  stream lives in js/data.js; this view opens no stream of its own.
 *  A hidden tab paints nothing: data.js pauses the poller and skips the
 *  frame. */
function advSubscribe() {
  if (adv.unsub) return adv.unsub;
  if (typeof onTabFrame !== "function") return null;
  adv.unsub = onTabFrame("advanced", (doc) => {
    if (!doc) return;
    adv.samples.push({ t: doc.time || Date.now(), gpus: doc.gpus || [] });
    if (adv.samples.length > 3600) adv.samples.shift();
    const runs = doc.runs || [];
    adv.server = runs.find((r) => r.state !== "exited") || runs[0] || null;
    if (adv.live.gpus) adv.live.gpus.update();
    if (adv.live.measures) adv.live.measures.update();
  });
  return adv.unsub;
}

/** advRunsOverview is the compact run list of the tab: the runs that serve,
 *  with a link to the tab that manages them. */
function advRunsOverview(root) {
  const card = h("div.card", cardHead("Runs", "/api/runs", h("a.small", { href: "#logs" }, "manage")), loading());
  put(root, card);
  const view = {
    update: async () => {
      try {
        const runs = runsOf(await api("/api/runs"));
        put(card, cardHead("Runs", "/api/runs", h("a.small", { href: "#logs" }, "manage")), advRunsTable(runs));
      } catch (e) {
        put(card, cardHead("Runs", "/api/runs"), failed(e));
      }
    },
  };
  view.update();
  return view;
}

/** advRunsTable is the table of the runs: the name, the state and the
 *  recipe of every run this host knows. */
function advRunsTable(runs) {
  if (!runs.length) {
    return emptyState("No run is serving.", "A run is a model server on this host: it loads the model and then answers requests.",
      "bernie serve <recipe>");
  }
  const cols = tableHead([{ label: "Run" }, { label: "State" }, { label: "Recipe" }, { label: "Since" }]);
  const rows = runs.map((r) => h("tr",
    h("td", r.name || "–"),
    h("td", badge(r.state || "skip")),
    h("td", r.recipe || "–"),
    h("td", r.started_at ? ago(r.started_at) : "–")));
  return h("div.scroll", h("table", cols, h("tbody", rows)));
}

/** advServerMeasures is the table of what the server of the live run
 *  measures: the latencies of the last minute and the counters since the
 *  server started. A measure the server does not publish is a dash. */
function advServerMeasures(root) {
  const card = h("div.card", cardHead("Server measures", "/api/telemetry"), loading());
  put(root, card);
  const lat = (d) => (d ? num(d.p50, 3) + " s p50, " + num(d.p95, 3) + " s p95" : "\u2013");
  const count = (m) => {
    if (!m) return "\u2013";
    const keys = Object.keys(m);
    return keys.length ? keys.map((k) => k + " " + num(m[k], 0)).join(", ") : "0";
  };
  const pair = (a, b) => num(a, 0) + " of " + num(b, 0) + " tokens";
  const view = {
    update: () => {
      const run = adv.server;
      const st = run && run.stats ? run.stats : null;
      const w = run && run.window ? run.window : null;
      if (!st) {
        put(card, cardHead("Server measures", "/api/telemetry"),
          emptyState("No run is serving.", "A run is a model server on this host.", "bernie serve <recipe>"));
        return;
      }
      const rows = [
        ["First token", lat(w && w.ttft), lat(st.ttft)],
        ["Time between tokens", lat(w && w.itl), lat(st.itl)],
        ["Whole request", lat(w && w.e2e), lat(st.e2e)],
        ["Wait in the queue", lat(w && w.queue), lat(st.queue)],
        ["Prompt tokens from the cache", w ? pair(w.prompt_tokens_cached, w.prompt_tokens) : "\u2013",
          st.prefix_cache ? pair(st.prompt_tokens_cached_total, st.prompt_tokens_total) : "\u2013"],
        ["External cache hits", "\u2013",
          st.external_cache ? pair(st.external_prefix_cache_hits_total, st.external_prefix_cache_queries_total) : "\u2013"],
        ["Preemptions", "\u2013", num(st.num_preemptions_total, 0) + " requests"],
        ["Waiting by reason", "\u2013", count(st.waiting_by_reason)],
        ["Finished by reason", "\u2013", count(st.finished_by_reason)],
      ];
      const cols = tableHead([{ label: "Measure" }, { label: "Last minute" }, { label: "Since the start" }]);
      put(card, cardHead("Server measures", "/api/telemetry"),
        h("div.scroll", h("table", cols, h("tbody", rows.map((r) => h("tr", h("td", r[0]), h("td", r[1]), h("td", r[2])))))));
    },
  };
  view.update();
  return view;
}
