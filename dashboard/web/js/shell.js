// bernie dashboard: the page-wide behaviour.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
// The toast, the copy button, the tooltip, the theme, the Advanced panels and
// the run status pill. The views only call these.
"use strict";

const THEME_KEY = "bernie.theme";
const ADVANCED_KEY = "bernie.advanced";

/* ---------- the toast ---------- */

let toastTimer;

/** toast says what happened. One toast at a time; a click dismisses it. */
function toast(msg, ok) {
  const t = $("#toast");
  if (!t) return;
  t.textContent = msg;
  t.className = ok === false ? "bad" : (ok ? "ok" : "");
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), 2600);
}

/* ---------- the copy ---------- */

/** copyText puts a text on the clipboard. It falls back to a hidden text area
 *  when the page is not a secure context (a plain http address on the LAN). */
async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (e) { /* fall through to the text area */ }
  try {
    const area = h("textarea", { style: "position:fixed;left:-9999px;top:0" });
    area.value = text;
    document.body.append(area);
    area.select();
    const ok = document.execCommand("copy");
    area.remove();
    return ok;
  } catch (e) { return false; }
}

/** copy puts a text on the clipboard and says so. */
async function copy(text, what) {
  const ok = await copyText(text);
  if (ok) toast(what ? "Copied the " + what + "." : "Copied.");
  else toast("Copy failed — select the text and press Ctrl-C.", false);
  return ok;
}

// One handler for every copy button of the page: the button carries the text
// in data-copy, or takes it from the command block it sits in.
document.addEventListener("click", async (ev) => {
  const target = ev.target instanceof Element ? ev.target : null;
  const btn = target && target.closest("[data-copy]");
  if (!btn) return;
  const text = btn.getAttribute("data-copy") || copySource(btn);
  if (!text) { toast("There is nothing to copy."); return; }
  const ok = await copyText(text);
  toast(ok ? "Copied." : "Copy failed — select the text and press Ctrl-C.", ok);
  if (ok) {
    const label = btn.textContent;
    btn.textContent = "Copied";
    setTimeout(() => { btn.textContent = label; }, 1200);
  }
});

/** copySource reads the text a copy button belongs to: the code of the command
 *  block around it. */
function copySource(btn) {
  const box = btn.closest(".cmd") || btn.parentElement;
  const code = box && box.querySelector("code, pre");
  return code ? code.textContent.trim() : "";
}

/* ---------- the tooltip ---------- */

let tipPanel = null;

/** tooltip is the small mark that opens a panel with an explanation. It opens
 *  on hover and on focus, and closes on Escape and on blur. */
function tooltip(text, label) {
  const mark = h("button", { type: "button", "aria-label": label || "What this means" }, "i");
  const wrap = h("span.tip", mark);
  const open = () => {
    closeTip();
    const panel = h("div.tip-panel", { role: "tooltip", id: "tip-" + (tipPanel ? tipPanel.length : 0) }, text);
    document.body.append(panel);
    tipPanel = panel;
    const box = mark.getBoundingClientRect();
    panel.style.left = Math.max(8, Math.min(window.innerWidth - panel.offsetWidth - 8, box.left)) + "px";
    panel.style.top = (box.bottom + 6) + "px";
    mark.setAttribute("aria-describedby", panel.id);
  };
  const close = () => closeTip();
  mark.addEventListener("mouseenter", open);
  mark.addEventListener("focus", open);
  mark.addEventListener("mouseleave", close);
  mark.addEventListener("blur", close);
  wrap.addEventListener("keydown", (e) => { if (e.key === "Escape") { close(); mark.focus(); } });
  return wrap;
}

function closeTip() {
  if (!tipPanel) return;
  tipPanel.remove();
  tipPanel = null;
}

/* ---------- the theme ---------- */

function themeChoice() {
  try { return localStorage.getItem(THEME_KEY) || ""; } catch (e) { return ""; }
}

/** darkNow says which palette the page shows right now. */
function darkNow() {
  const choice = themeChoice();
  if (choice === "dark") return true;
  if (choice === "light") return false;
  return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
}

/** applyTheme shows the palette the reader picked. An empty choice follows the
 *  system. */
function applyTheme(choice) {
  const root = document.documentElement;
  if (choice === "dark" || choice === "light") root.dataset.theme = choice;
  else delete root.dataset.theme;
  const btn = $("#themeBtn");
  if (btn) {
    const dark = darkNow();
    btn.textContent = dark ? "Light" : "Dark";
    btn.setAttribute("aria-pressed", String(dark));
    btn.setAttribute("aria-label", dark ? "Switch to the light palette" : "Switch to the dark palette");
  }
}

/** toggleTheme flips the palette and remembers the choice. */
function toggleTheme() {
  const next = darkNow() ? "light" : "dark";
  try { localStorage.setItem(THEME_KEY, next); } catch (e) { /* the page still works */ }
  applyTheme(next);
}

/* ---------- the advanced panels ---------- */

/** advancedOn says whether the Advanced panels are open. */
function advancedOn() {
  try { return localStorage.getItem(ADVANCED_KEY) === "1"; } catch (e) { return false; }
}

/** applyAdvanced opens or closes every Advanced panel of every tab. */
function applyAdvanced(on) {
  document.body.dataset.advanced = on ? "on" : "off";
  const btn = $("#advancedBtn");
  if (btn) {
    btn.setAttribute("aria-pressed", String(!!on));
    btn.setAttribute("aria-label", on ? "Hide the advanced panels" : "Show the advanced panels");
  }
  for (const det of document.querySelectorAll("details.advanced")) det.open = !!on;
}

function toggleAdvanced() {
  const next = !advancedOn();
  try { localStorage.setItem(ADVANCED_KEY, next ? "1" : "0"); } catch (e) { /* the page still works */ }
  applyAdvanced(next);
}

/* ---------- the run status pill ---------- */

/** setRunPill paints the answer to "is something serving?" in the footer of
 *  the rail. The word carries the meaning; the colour only helps. */
function setRunPill(runs) {
  const pill = $("#runpill");
  if (!pill) return;
  const list = Array.isArray(runs) ? runs : [];
  const live = list.filter((r) => r.state === "running");
  const starting = list.filter((r) => r.state === "starting");
  const exited = list.filter((r) => r.state === "exited");
  let word = "idle", icon = "–", tone = "skip", sentence = "No run is serving on this host.";
  if (starting.length) {
    word = "starting"; icon = "◐"; tone = "info";
    sentence = phaseSentence(starting[0], "The server is loading the model.");
  } else if (live.length) {
    const answering = live.some((r) => r.answering !== false);
    if (answering) {
      word = "serving"; icon = "●"; tone = "ok";
      sentence = phaseSentence(live[0], "The server answers requests.");
    } else {
      word = "not answering"; icon = "!"; tone = "warn";
      sentence = phaseSentence(live[0], "The run exists but the server does not answer.");
    }
  } else if (exited.length) {
    word = "stopped"; icon = "✕"; tone = "fail";
    sentence = phaseSentence(exited[0], "The run has stopped.");
  }
  pill.className = "pill " + tone;
  pill.querySelector(".pill-icon").textContent = icon;
  pill.querySelector(".pill-word").textContent = word;
  pill.setAttribute("title", word + " — " + sentence);
  pill.setAttribute("aria-label", word + " — " + sentence);
}

/** phaseSentence is the sentence of the phase of a run. */
function phaseSentence(run, fallback) {
  if (!run) return fallback;
  if (run.phase) {
    if (typeof run.phase === "string") return run.phase;
    if (run.phase.sentence) return run.phase.sentence;
  }
  if (run.state_note) return run.state_note;
  return fallback;
}
