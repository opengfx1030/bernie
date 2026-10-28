"use strict";
// Router and start-up. Loaded last: every view function exists by now.

const views = { home: viewHome, gpus: viewGPUs, topology: viewTopology, recipes: viewRecipes, runs: viewRuns, bench: viewBench };

// The Overview tab became the Home tab. The old link still opens it.
const aliases = { overview: "home" };

function route() {
  const wanted = (location.hash || "#home").slice(1);
  const tab = views[wanted] ? wanted : (views[aliases[wanted]] ? aliases[wanted] : "home");
  state.tab = tab;
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
