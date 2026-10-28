"use strict";
// Router and start-up. Loaded last: every view function exists by now.

const views = { overview: viewOverview, gpus: viewGPUs, topology: viewTopology, recipes: viewRecipes, runs: viewRuns, bench: viewBench };

function route() {
  const tab = (location.hash || "#overview").slice(1);
  state.tab = views[tab] ? tab : "overview";
  if (state.tab !== "runs") stopLogs();
  for (const a of document.querySelectorAll("#tabs a")) a.setAttribute("aria-selected", String(a.dataset.tab === state.tab));
  for (const s of document.querySelectorAll("main > .tab")) s.hidden = s.id !== "tab-" + state.tab;
  views[state.tab]($("#tab-" + state.tab));
}

window.addEventListener("hashchange", route);
window.addEventListener("resize", () => state.tab === "gpus" && renderGPUs());
startTelemetry();
loadHW().catch(() => {}); // header hostname, GPU names
route();
