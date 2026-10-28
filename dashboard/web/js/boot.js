"use strict";
// The router and the start-up. Loaded last: every view function exists by now.
//
// The page has four tabs: #setup, #dashboard, #benchmark and #logs. The old
// links keep working: the router rewrites them and replaces the history entry,
// so the back button never walks through them.

const VIEWS = {
  setup: "viewSetup",
  dashboard: "viewDashboard",
  benchmark: "viewBenchmark",
  logs: "viewLogs",
  classic: "viewClassic",
};

// The old routes and where they land now.
const OLD_ROUTES = {
  home: ["setup", ""],
  overview: ["setup", ""],
  recipes: ["setup", "recipes"],
  gpus: ["dashboard", "gpus"],
  topology: ["dashboard", "topology"],
  runs: ["dashboard", "runs"],
  bench: ["benchmark", "suites"],
};

/** parseHash reads the route: the tab and the section inside it. */
function parseHash() {
  const raw = (location.hash || "").replace(/^#/, "");
  const [name, section = ""] = raw.split("/");
  if (OLD_ROUTES[name]) return { tab: OLD_ROUTES[name][0], section: OLD_ROUTES[name][1], old: name };
  if (VIEWS[name]) return { tab: name, section, old: "" };
  return { tab: "setup", section: "", old: "" };
}

/** route shows the tab of the address bar and paints it. */
function route() {
  const r = parseHash();
  if (r.old) {
    // The old link opens its new home and disappears from the history.
    const next = "#" + r.tab + (r.section ? "/" + r.section : "");
    history.replaceState(null, "", next);
  }
  state.tab = r.tab;

  for (const link of document.querySelectorAll("[data-tab]")) {
    const on = link.dataset.tab === r.tab;
    link.setAttribute("aria-selected", String(on));
    link.setAttribute("tabindex", on ? "0" : "-1");
  }
  for (const panel of document.querySelectorAll("[data-tab-panel]")) {
    panel.hidden = panel.dataset.tabPanel !== r.tab;
  }

  const panel = document.querySelector('[data-tab-panel="' + r.tab + '"]');
  const fn = window[VIEWS[r.tab]];
  if (panel && typeof fn === "function") fn(panel);

  if (r.section) focusSection(r.tab, r.section);
}

/** focusSection scrolls to a section of a tab and puts the focus on its
 *  heading. */
function focusSection(tab, section) {
  const panel = document.querySelector('[data-tab-panel="' + tab + '"]');
  if (!panel) return;
  const target = panel.querySelector('[data-section="' + section + '"]') || panel.querySelector("#" + tab + "-" + section);
  if (!target) return;
  target.scrollIntoView({ block: "start" });
  const head = target.querySelector("h2, h3") || target;
  if (head.setAttribute) {
    head.setAttribute("tabindex", "-1");
    head.focus({ preventScroll: true });
  }
}

/** tabsKey moves the focus in the tab bar: Left, Right, Home and End. */
function tabsKey(ev) {
  const tabs = Array.from(document.querySelectorAll('[role="tab"]'));
  if (!tabs.length) return;
  const i = tabs.indexOf(document.activeElement);
  if (i < 0) return;
  let next = -1;
  if (ev.key === "ArrowRight" || ev.key === "ArrowDown") next = (i + 1) % tabs.length;
  else if (ev.key === "ArrowLeft" || ev.key === "ArrowUp") next = (i - 1 + tabs.length) % tabs.length;
  else if (ev.key === "Home") next = 0;
  else if (ev.key === "End") next = tabs.length - 1;
  if (next < 0) return;
  ev.preventDefault();
  const link = tabs[next];
  link.focus();
  if (link.dataset.tab && link.dataset.tab !== state.tab) location.hash = "#" + link.dataset.tab;
}

/** shortcutsKey moves between the tabs with the keyboard: g, then the letter
 *  of the tab. */
let pendingG = false;

function shortcutsKey(ev) {
  const t = ev.target;
  if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
  if (ev.metaKey || ev.ctrlKey || ev.altKey) return;
  const k = (ev.key || "").toLowerCase();
  if (pendingG) {
    pendingG = false;
    const tab = { s: "setup", d: "dashboard", b: "benchmark", l: "logs", c: "classic" }[k];
    if (tab) {
      ev.preventDefault();
      location.hash = "#" + tab;
    }
    return;
  }
  if (k === "g") {
    pendingG = true;
    toast("Now press s, d, b, l or c.");
    setTimeout(() => (pendingG = false), 1500);
    return;
  }
  if (k === "escape") {
    pendingG = false;
    closeTip();
  }
}

/* ---------- the start-up ---------- */

function boot() {
  applyTheme(themeChoice());
  applyAdvanced(advancedOn());

  const themeBtn = $("#themeBtn");
  if (themeBtn) themeBtn.addEventListener("click", toggleTheme);
  const advancedBtn = $("#advancedBtn");
  if (advancedBtn) advancedBtn.addEventListener("click", toggleAdvanced);
  const helpBtn = $("#helpBtn");
  if (helpBtn) {
    helpBtn.addEventListener("click", () => {
      if (tipPanel) { closeTip(); return; }
      openTip(helpBtn, shortcutsText());
    });
  }

  const toastEl = $("#toast");
  if (toastEl) toastEl.addEventListener("click", () => (toastEl.hidden = true));

  const list = $("#tabs");
  if (list) list.addEventListener("keydown", tabsKey);
  document.addEventListener("keydown", shortcutsKey);

  // A panel that appears later follows the choice of the reader.
  const main = document.querySelector("main");
  if (main && window.MutationObserver) {
    new MutationObserver(() => applyAdvanced(advancedOn())).observe(main, { childList: true, subtree: true });
  }
  window.addEventListener("hashchange", route);
  window.addEventListener("resize", () => state.tab === "dashboard" && typeof renderGPUs === "function" && renderGPUs());

  startTelemetry();
  startPolling();
  loadHW().catch(() => {}); // the rail shows the name of the host
  route();
}

boot();
