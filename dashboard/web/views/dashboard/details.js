"use strict";
// ---------- Dashboard: Show details ----------
// The disclosure at the end of the Dashboard tab: more about the same thing
// on this tab. The documents are read when the reader opens the disclosure,
// not when the tab opens.
//
// The per-GPU table is the one implementation of components/gputable.js: the
// Advanced tab shows the same table.

// The windows the history of the cards answers with.
const DASH_DETAIL_WINDOWS = ["5m", "15m", "1h"];

/** dashDetails is the disclosure at the end of the Dashboard tab. */
function dashDetails() {
  const body = h("div.dash-details-body", loading());
  const det = h("details.advanced.dash-details", h("summary", "Show details"), body);
  let loaded = false;
  det.addEventListener("toggle", async () => {
    if (!det.open || loaded) return;
    loaded = true;
    await dashLoadDetails(view);
  });
  const view = {
    el: det,
    body: body,
    table: null,
    update: () => {
      // The table follows the stream; the rest of the disclosure is read once.
      if (!view.table) return;
      view.table.update(
        (dash.hw && dash.hw.gpus) || [],
        (dash.tele && dash.tele.gpus) || [],
        (dash.doctor && dash.doctor.checks) || []);
    },
  };
  return view;
}

/** dashLoadDetails reads the documents of the disclosure and paints them. */
async function dashLoadDetails(view) {
  const body = view.body;
  const power = await api("/api/power").catch(() => null);
  const docs = [];
  await dashDetailsRead(docs, "/api/telemetry", dash.tele);
  await dashDetailsRead(docs, "/api/hw", dash.hw);
  await dashDetailsRead(docs, "/api/runs", dash.runs);
  await dashDetailsRead(docs, "/api/gpus/history?window=5m");
  const table = gpuValueTable(
    (dash.hw && dash.hw.gpus) || [],
    (dash.tele && dash.tele.gpus) || [],
    (dash.doctor && dash.doctor.checks) || []);
  view.table = table;
  put(body,
    h("div.card", cardHead("All GPU values", "/api/hw"), table.el),
    dashDetailsWindow(),
    dashDetailsPower(power),
    dashDetailsDocuments(docs));
}

/** dashDetailsRead adds one document of the disclosure. A failed read shows
 *  its message; the other documents still come. */
async function dashDetailsRead(entries, path, doc) {
  if (doc !== undefined) {
    entries.push({ path: path, doc: doc });
    return;
  }
  try {
    entries.push({ path: path, doc: await api(path) });
  } catch (e) {
    entries.push({ path: path, error: String(e.message || e) });
  }
}

/** dashDetailsWindow is the window of the history: the reader picks it and
 *  the chart shows the load of the cards over that window. */
function dashDetailsWindow() {
  const card = h("div.card");
  const seg = h("div.segmented-control", { role: "group", "aria-label": "The window of the history" });
  const note = h("p.small.muted");
  const chart = areaChart([], { title: "The load of the cards", unit: "%", windowMinutes: 5 });
  let win = "5m";
  const buttons = {};
  for (const w of DASH_DETAIL_WINDOWS) {
    const button = h("button", { type: "button", "aria-pressed": String(w === win) }, w);
    button.addEventListener("click", () => {
      win = w;
      for (const [id, other] of Object.entries(buttons)) other.setAttribute("aria-pressed", String(id === win));
      paint();
    });
    buttons[w] = button;
    seg.append(button);
  }
  const paint = async () => {
    put(note, "Reading the history of the cards…");
    let doc = null;
    try {
      doc = await api("/api/gpus/history?window=" + win);
    } catch (e) {
      put(note, "The history could not be read: " + (e.message || e));
      chart.update([], dashWindowMinutes(win));
      return;
    }
    const series = (doc && doc.series) || [];
    const n = series.length ? (series[0].busy_pct || []).length : 0;
    const mean = [];
    for (let i = 0; i < n; i++) {
      const values = series.map((s) => (s.busy_pct || [])[i]).filter((v) => v != null);
      mean.push(values.length ? values.reduce((a, b) => a + b, 0) / values.length : null);
    }
    const minutes = dashWindowMinutes(win);
    chart.update(mean, minutes);
    put(note, "Last " + ((doc && doc.window) || win) + ", one point every "
      + ((doc && doc.interval_s) || "–") + " s, " + ((doc && doc.points) || n) + " points. "
      + "The mean load of the cards.");
  };
  put(card, cardHead("The history window", "/api/gpus/history"), seg, note, chart);
  paint();
  return card;
}

/** dashWindowMinutes is the number of minutes of a window of the history. */
function dashWindowMinutes(win) {
  if (win === "1h") return 60;
  if (win === "15m") return 15;
  return 5;
}

/** dashDetailsPower is the read-out of the power caps: the cap of every card
 *  and the range the service allows. The write lives on the Advanced tab:
 *  this is the reader's view of the host. */
function dashDetailsPower(power) {
  const card = h("div.card");
  if (!power) {
    put(card, cardHead("The power caps", "/api/power"),
      emptyState("The power caps are not available.",
        "The caps of the cards come from the power service of the host.",
        "bernie tune power status"));
    return card;
  }
  const rows = (power.gpus || []).map((g) => h("tr",
    h("td", "GPU " + g.hip_index),
    h("td.num", Math.round(g.cap_w) + " W"),
    h("td.num", Math.round(g.min_w || 1) + "–" + Math.round(g.max_w || g.cap_w || 1) + " W"),
    h("td.num", g.desired_w == null ? "–" : Math.round(g.desired_w) + " W"),
    h("td", g.drift ? h("span.badge.b-warn", "drift") : h("span.badge.b-ok", "in sync"))));
  put(card,
    cardHead("The power caps", "/api/power",
      power.installed ? null : h("span.badge.b-warn", "not installed")),
    rows.length
      ? h("div.scroll", h("table",
        tableHead(["GPU", { label: "Current", num: true }, { label: "Allowed", num: true },
          { label: "Desired", num: true }, "State"]),
        h("tbody", rows)))
      : emptyState("No GPU publishes a hwmon power cap.",
        "The caps come from the power service of the host.",
        "bernie tune power status"));
  return card;
}

/** dashDetailsDocuments are the raw documents behind the tab. */
function dashDetailsDocuments(entries) {
  return h("div.dash-details-documents",
    h("p.small.muted", "The documents behind this tab. An agent reads the same ones from the API."),
    entries.map((entry) => {
      const text = entry.error
        ? JSON.stringify({ error: entry.error }, null, 2)
        : JSON.stringify(entry.doc, null, 2);
      return h("div.dash-details-document",
        h("div.dash-details-head",
          h("span.dash-details-path", entry.path),
          h("span.grow"),
          jsonLink(entry.path),
          copyBtn(text, "Copy the JSON of " + entry.path)),
        h("pre.dash-details-json", text));
    }));
}
