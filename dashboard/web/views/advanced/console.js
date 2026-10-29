"use strict";
// ---------- Advanced: the operator's console ----------
// The Advanced tab answers one question: what can the operator inspect and
// change on this host? It holds the exact per-GPU table, the power caps
// (the read and the one write of the page), the doctor report in full, the
// jobs of the host, the run record and the raw documents of every endpoint
// the page reads.
//
// The old first-dashboard layout is gone: the tiles and the cards are on the
// Dashboard tab, the PCIe tree and the P2P checklist are on the Topology
// tab. The per-GPU table is the one implementation of components/gputable.js:
// the details of the Dashboard tab and this tab share it.
//
// The stream is the one of the page: js/data.js owns it and this view
// registers its repaint with onTabFrame. It opens no stream of its own.

const adv = {
  hw: null,       // /api/hw
  doctor: null,   // /api/doctor
  runs: [],       // /api/runs
  jobs: [],       // /api/jobs
  tele: null,     // the last document of the telemetry stream
  server: null,   // the live run of the stream
  head: null,     // the headline of the tab
  live: {},       // the parts the tab repaints
  unsub: null,    // the way back from the shared stream
};

/** viewAdvanced opens the tab. The router calls it with the panel of the
 *  tab. */
async function viewAdvanced(root) {
  put(root, loading());
  adv.live = {};
  const head = stateHead("Advanced",
    h("button.primary", { type: "button", onclick: () => advReload() }, "Re-check the host"));
  adv.head = head;
  const table = gpuValueTable([], [], []);
  const power = advPowerCard();
  const doctor = advDoctorReport();
  const jobs = advJobList();
  const runs = advRunsTable();
  const measures = advServerMeasures();
  put(root, head,
    h("section.section", { id: "advanced-gpus", "data-section": "gpus" },
      sectionLabel("The per-GPU values"),
      h("div.card",
        cardHead("All GPU values", "/api/hw",
          h("button.small", { type: "button", onclick: () => advCopyCardList() }, "Copy the card list")),
        table.el)),
    h("section.section", { id: "advanced-power", "data-section": "power" },
      sectionLabel("The power caps"), power.el),
    h("section.section", { id: "advanced-doctor", "data-section": "doctor" },
      sectionLabel("The doctor report"), doctor.el),
    h("section.section", { id: "advanced-jobs", "data-section": "jobs" },
      sectionLabel("The jobs"), jobs.el),
    h("section.section", { id: "advanced-run", "data-section": "run" },
      sectionLabel("The run record"), runs.el),
    h("section.section", { id: "advanced-measures", "data-section": "measures" },
      sectionLabel("The server measures"), measures.el),
    h("section.section", { id: "advanced-documents", "data-section": "documents" },
      sectionLabel("The raw documents"), advDocuments()));
  adv.live = {
    table: table, power: power, doctor: doctor, jobs: jobs, runs: runs, measures: measures,
  };
  await advLoad();
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
    one("/api/doctor", "doctor"),
    one("/api/runs", "runs"),
    one("/api/jobs", "jobs"),
  ]);
  if (!Array.isArray(adv.runs)) adv.runs = [];
  if (!Array.isArray(adv.jobs)) adv.jobs = [];
  advPaint();
  await adv.live.power.reload();
}

/** advReload reads the documents of the host again. The doctor is held for
 *  fifteen seconds by the server; the button asks for a fresh read. */
async function advReload() {
  adv.errors = {};
  await advLoad();
  toast("The host was read again.");
}

/** advPaint repaints every part of the tab with the documents it holds. */
function advPaint() {
  if (!adv.live.table) return;
  advHead(adv.head);
  adv.live.table.update(
    (adv.hw && adv.hw.gpus) || [],
    (adv.tele && adv.tele.gpus) || [],
    (adv.doctor && adv.doctor.checks) || []);
  adv.live.doctor.update(adv.doctor);
  adv.live.jobs.update(adv.jobs);
  adv.live.runs.update(adv.runs);
  adv.live.measures.update(adv.server);
}

/** advHead writes the headline of the tab: the situation of the host in one
 *  sentence, and what the tab holds under it. */
function advHead(head) {
  if (!head) return;
  const sum = (adv.doctor && adv.doctor.summary) || {};
  const fail = sum.fail || 0;
  const warn = sum.warn || 0;
  const title = fail
    ? fail + (fail === 1 ? " check fails" : " checks fail") + " on this host"
    : warn
      ? warn + (warn === 1 ? " check warns" : " checks warn") + " on this host"
      : "Every check of this host passed";
  setHead(head, title,
    "The operator's console: the exact values of every card, the power caps, the whole doctor report, the jobs of the host and the raw documents behind them.");
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
    adv.tele = doc;
    const runs = doc.runs || [];
    adv.server = runs.find((r) => r.state !== "exited") || runs[0] || null;
    if (!adv.live.table) return;
    adv.live.table.update(
      (adv.hw && adv.hw.gpus) || [],
      doc.gpus || [],
      (adv.doctor && adv.doctor.checks) || []);
    adv.live.measures.update(adv.server);
  });
  return adv.unsub;
}

/** advRunsTable is the run record of the host: every run this host knows,
 *  with its state, its recipe, its port and its process. */
function advRunsTable() {
  const card = h("div.card");
  const view = {
    el: card,
    update: (runs) => {
      const list = runs || [];
      if (!list.length) {
        put(card, cardHead("The run record", "/api/runs"),
          emptyState("No run is serving.",
            "A run is a model server on this host: it loads the model and then answers requests.",
            "bernie serve <recipe>"));
        return;
      }
      const rows = list.map((r) => h("tr",
        h("td", r.name || "–"),
        h("td", badge(r.state || "skip")),
        h("td", r.recipe || "–"),
        h("td", r.mode || "host"),
        h("td.num", r.port ? String(r.port) : "–"),
        h("td.num", r.pid ? String(r.pid) : "–"),
        h("td", r.started_at ? ago(r.started_at) : "–")));
      put(card,
        cardHead("The run record", "/api/runs",
          h("span.small.muted", list.length + " runs"),
          h("a.small", { href: "#logs" }, "manage")),
        h("div.scroll", h("table",
          tableHead(["Run", "State", "Recipe", "Mode", { label: "Port", num: true },
            { label: "PID", num: true }, "Since"]),
          h("tbody", rows))));
    },
  };
  return view;
}

/** advCardListText writes the cards as plain text: one line per card, so
 *  it can be pasted into a report or a ticket. */
function advCardListText() {
  const gpus = (adv.hw && adv.hw.gpus) || [];
  const samples = (adv.tele && adv.tele.gpus) || [];
  if (!samples.length && !gpus.length) return "";
  return samples.map((s) => {
    const g = gpus.find((x) => x.hip_index === s.hip_index) || {};
    return ["GPU " + s.hip_index, g.name || g.gfx || "", num(s.busy_pct, 0) + " % busy",
      num(s.temp_c, 0) + " °C", num(s.power_w, 1) + " W",
      (s.vram_used ? bytes(s.vram_used) : "–") + " / " + (s.vram_total ? bytes(s.vram_total) : "–")].join("\t");
  }).join("\n");
}

/** advCopyCardList puts the card list on the clipboard. */
async function advCopyCardList() {
  const text = advCardListText();
  if (!text) {
    toast("There are no cards to copy yet.");
    return;
  }
  const ok = await copyText(text);
  toast(ok ? "The card list is on the clipboard." : "Copy failed: select the text and copy it.", ok);
}

/** advServerMeasures is the table of what the server of the live run
 *  measures: the latencies of the last minute and the counters since the
 *  server started. A measure the server does not publish is a dash. */
function advServerMeasures() {
  const card = h("div.card");
  const lat = (d) => (d ? num(d.p50, 3) + " s p50, " + num(d.p95, 3) + " s p95" : "–");
  const count = (m) => {
    if (!m) return "–";
    const keys = Object.keys(m);
    return keys.length ? keys.map((k) => k + " " + num(m[k], 0)).join(", ") : "0";
  };
  const pair = (a, b) => num(a, 0) + " of " + num(b, 0) + " tokens";
  const view = {
    el: card,
    update: (run) => {
      const st = run && run.stats ? run.stats : null;
      const w = run && run.window ? run.window : null;
      if (!st) {
        put(card, cardHead("The server measures", "/api/telemetry"),
          emptyState("No run is serving.", "A run is a model server on this host.",
            "bernie serve <recipe>"));
        return;
      }
      const rows = [
        ["First token", lat(w && w.ttft), lat(st.ttft)],
        ["Time between tokens", lat(w && w.itl), lat(st.itl)],
        ["Whole request", lat(w && w.e2e), lat(st.e2e)],
        ["Wait in the queue", lat(w && w.queue), lat(st.queue)],
        ["Prompt tokens from the cache", w ? pair(w.prompt_tokens_cached, w.prompt_tokens) : "–",
          st.prefix_cache ? pair(st.prompt_tokens_cached_total, st.prompt_tokens_total) : "–"],
        ["External cache hits", "–",
          st.external_cache ? pair(st.external_prefix_cache_hits_total, st.external_prefix_cache_queries_total) : "–"],
        ["Preemptions", "–", num(st.num_preemptions_total, 0) + " requests"],
        ["Waiting by reason", "–", count(st.waiting_by_reason)],
        ["Finished by reason", "–", count(st.finished_by_reason)],
      ];
      put(card, cardHead("The server measures", "/api/telemetry"),
        h("div.scroll", h("table",
          tableHead(["Measure", "Last minute", "Since the start"]),
          h("tbody", rows.map((r) => h("tr", h("td", r[0]), h("td", r[1]), h("td", r[2])))))));
    },
  };
  return view;
}
