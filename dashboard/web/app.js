// bernie dashboard. Vanilla JS, no build step, no dependencies.
// Every view calls the JSON API of spec §11; the API calls the same Go code as the CLI.
"use strict";

// ---------- helpers ----------

/** h("div.card", {onclick}, child, ...) builds DOM without innerHTML (no XSS). */
function h(spec, attrs, ...kids) {
  const [tag, ...cls] = spec.split(".");
  const el = document.createElement(tag || "div");
  if (cls.length) el.className = cls.join(" ");
  if (attrs != null && (typeof attrs !== "object" || attrs instanceof Node || Array.isArray(attrs))) {
    kids.unshift(attrs);
    attrs = null;
  }
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "class") el.className += (el.className ? " " : "") + v;
    else if (k in el && k !== "list" && typeof v !== "string") el[k] = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  add(el, kids);
  return el;
}
function add(el, kids) {
  for (const k of kids.flat(Infinity)) {
    if (k == null || k === false) continue;
    el.append(k instanceof Node ? k : String(k));
  }
}
function put(el, ...kids) { el.replaceChildren(); add(el, kids); return el; }
const $ = (sel) => document.querySelector(sel);

const GiB = 1024 ** 3;
function bytes(n) {
  if (!n) return "0";
  const u = ["B", "KiB", "MiB", "GiB", "TiB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (n >= 100 || i === 0 ? n.toFixed(0) : n.toFixed(1)) + " " + u[i];
}
function num(v, d = 1) {
  if (v == null || v === "") return "–";
  if (typeof v !== "number") return String(v);
  if (Number.isInteger(v)) return String(v);
  return v.toFixed(d);
}
function ago(t) {
  const s = (Date.now() - new Date(t).getTime()) / 1000;
  if (!isFinite(s)) return "";
  if (s < 60) return Math.round(s) + " s ago";
  if (s < 3600) return Math.round(s / 60) + " min ago";
  if (s < 86400) return Math.round(s / 3600) + " h ago";
  return Math.round(s / 86400) + " d ago";
}
function badge(status) { return h("span.badge", { class: "b-" + status }, status || "?"); }
function sh(a) { return /^[\w@%+=:,./-]+$/.test(a) ? a : "'" + a.replace(/'/g, "'\\''") + "'"; }

// ---------- copy and raw JSON ----------

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

/** copySource reads the text a copy button belongs to: the code of the command
 *  block around it. */
function copySource(btn) {
  const box = btn.closest(".cmd") || btn.parentElement;
  const code = box && box.querySelector("code, pre");
  return code ? code.textContent.trim() : "";
}

// One handler for every copy button on the page: the button carries the text
// in data-copy, or takes it from the command block it sits in.
document.addEventListener("click", async (ev) => {
  const target = ev.target instanceof Element ? ev.target : null;
  const btn = target && target.closest("[data-copy]");
  if (!btn) return;
  const text = btn.getAttribute("data-copy") || copySource(btn);
  if (!text) { toast("There is nothing to copy."); return; }
  const ok = await copyText(text);
  toast(ok ? "Copied to the clipboard." : "Copy failed: select the text and copy it.", ok);
  if (ok) {
    const label = btn.textContent;
    btn.textContent = "Copied";
    setTimeout(() => { btn.textContent = label; }, 1200);
  }
});

// copyBtn is a copy button for a command shown inline (a fix, a flag).
function copyBtn(text) {
  return h("button.small.copy", { type: "button", "data-copy": text, "aria-label": "Copy the command" }, "Copy");
}

/** cmdBlock shows a command with its copy button. The text is never markup. */
function cmdBlock(text, label) {
  if (!text) return null;
  return h("div.cmd", h("code", text),
    h("button.copy", { type: "button", "data-copy": text, "aria-label": label || "Copy the command" }, "Copy"));
}

/** jsonLink opens the raw JSON of an endpoint in a new tab. Every card that
 *  shows data links to the document behind it. */
function jsonLink(path) {
  return h("a.json-link", { href: path, target: "_blank", rel: "noopener noreferrer", title: "Open the raw JSON: " + path }, "json");
}

/** cardHead is the header of a card: the title, the raw JSON behind it and
 *  whatever else the card shows next to the title. */
function cardHead(title, path, ...rest) {
  return h("h2", title, h("span.grow"), ...rest, path ? jsonLink(path) : null);
}

// ---------- the state headline ----------

/** stateHead is the block that opens a tab: the label of the tab, the
 *  headline that says what the situation is and the line under it. The
 *  headline is read out when it changes. The action is the one primary
 *  button of the tab; it sits under the headline. */
function stateHead(eyebrow, action) {
  return h("section.state-head",
    h("p.eyebrow", eyebrow),
    h("h1.display", { "aria-live": "polite" }, "…"),
    h("p.lede", ""),
    action ? h("div.head-actions", action) : null);
}

/** setHead writes the headline of a tab: the situation in one sentence, and
 *  the reason under it. */
function setHead(head, title, lede) {
  const t = head.querySelector(".display");
  const l = head.querySelector(".lede");
  t.textContent = title;
  l.textContent = lede || "";
  l.hidden = !lede;
}

/** emptyState teaches: what the thing is, and what to do next. */
function emptyState(what, next, command) {
  return h("div.empty", h("b", what),
    next ? h("span.next", next) : null,
    command ? cmdBlock(command) : null);
}

let toastTimer;
function toast(msg, ok) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = ok ? "ok" : "";
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), ok ? 3000 : 8000);
}

/** api() fetches JSON; mutating methods carry the X-Bernie header (CSRF guard). */
async function api(path, opts = {}) {
  const init = { method: opts.method || "GET", headers: {}, credentials: "same-origin" };
  if (init.method !== "GET") init.headers["X-Bernie"] = "1";
  if (opts.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(opts.body);
  }
  const r = await fetch(path, init);
  const text = await r.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  if (!r.ok) throw new Error((data && data.error) || r.status + " " + r.statusText);
  return data;
}

function loading() { return h("div.empty", h("span.spin"), " loading…"); }
function failed(e) { return h("div.err", String(e.message || e)); }

/** Small canvas sparkline, no chart library. */
function spark(canvas, values, max) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth, hgt = canvas.clientHeight;
  if (!w || !hgt) return;
  if (canvas.width !== w * dpr || canvas.height !== hgt * dpr) {
    canvas.width = w * dpr;
    canvas.height = hgt * dpr;
  }
  const c = canvas.getContext("2d");
  c.setTransform(dpr, 0, 0, dpr, 0, 0);
  c.clearRect(0, 0, w, hgt);
  if (values.length < 2) return;
  const top = max || Math.max(1, ...values);
  const step = w / (HISTORY - 1);
  const x0 = w - (values.length - 1) * step;
  const color = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#b3122c";
  c.beginPath();
  values.forEach((v, i) => {
    const y = hgt - 1 - (Math.min(v, top) / top) * (hgt - 2);
    i ? c.lineTo(x0 + i * step, y) : c.moveTo(x0, y);
  });
  c.strokeStyle = color;
  c.lineWidth = 1.5;
  c.stroke();
  c.lineTo(w, hgt);
  c.lineTo(x0, hgt);
  c.closePath();
  c.globalAlpha = 0.15;
  c.fillStyle = color;
  c.fill();
  c.globalAlpha = 1;
}

// ---------- shared state ----------

const HISTORY = 60; // seconds of sparkline history
const state = {
  hw: null,          // /api/hw
  tele: null,        // last telemetry event
  hist: {},          // key -> number[] (last HISTORY samples)
  tab: "overview",
};
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

// ---------- telemetry stream (shared by tabs) ----------

let es = null;
function startTelemetry() {
  if (es) return;
  es = new EventSource("/api/telemetry");
  es.onopen = () => $("#live").classList.add("on");
  es.onerror = () => $("#live").classList.remove("on");
  es.onmessage = (ev) => {
    const t = JSON.parse(ev.data);
    state.tele = t;
    for (const s of t.gpus || []) {
      pushHist("busy" + s.hip_index, s.busy_pct);
      pushHist("pow" + s.hip_index, s.power_w);
    }
    for (const r of t.runs || []) pushHist("gen:" + r.name, r.gen_tok_s);
    if (state.tab === "gpus") renderGPUs();
  };
}
