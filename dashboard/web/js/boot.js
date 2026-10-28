"use strict";
// Router and start-up. Loaded last: every view function exists by now.

const views = {
  setup: viewSetup,
  dashboard: viewDashboard,
  benchmark: viewBenchmark,
  logs: viewLogs,
};

// The old tab names still open their new home. The page keeps the old links
// and bookmarks working while the four tabs replace the old six.
const aliases = {
  overview: "setup",
  home: "setup",
  recipes: "setup",
  gpus: "dashboard",
  topology: "dashboard",
  bench: "benchmark",
  runs: "logs",
};

function route() {
  const wanted = (location.hash || "#setup").slice(1);
  const tab = views[wanted] ? wanted : (views[aliases[wanted]] ? aliases[wanted] : "setup");
  state.tab = tab;
  if (state.tab !== "logs") stopLogs();
  for (const link of document.querySelectorAll("[data-tab]")) {
    link.setAttribute("aria-selected", String(link.dataset.tab === state.tab));
  }
  for (const panel of document.querySelectorAll("[data-tab-panel]")) {
    panel.hidden = panel.dataset.tabPanel !== state.tab;
  }
  views[state.tab](document.querySelector('[data-tab-panel="' + state.tab + '"]'));
}

window.addEventListener("hashchange", route);
window.addEventListener("resize", () => state.tab === "dashboard" && renderGPUs());
startTelemetry();
loadHW().catch(() => {}); // header hostname, GPU names
route();
