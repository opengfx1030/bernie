"use strict";
// The runs overview is the compact run list on the Dashboard tab.

async function renderDashboardOverview(root) {
  const card = h("div.card", cardHead("Runs", "/api/runs",
    h("a.small", { href: "#logs" }, "manage")), loading());
  put(root, card);
  try {
    const runs = runsOf(await api("/api/runs"));
    put(card, cardHead("Runs", "/api/runs", h("a.small", { href: "#logs" }, "manage")),
      runsTable(runs, true));
  } catch (e) {
    put(card, cardHead("Runs", "/api/runs"), failed(e));
  }
}
