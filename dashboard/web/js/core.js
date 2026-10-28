// bernie dashboard: the DOM and the formatting primitives.
// Every element of the page is built here. There is no innerHTML anywhere:
// the server sends text, and the text goes into the page as text.
"use strict";

/** h("div.card", {onclick}, child, ...) builds a DOM node without markup. */
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

/** add appends children to a node: nodes go in as they are, text as text. */
function add(el, kids) {
  for (const k of kids.flat(Infinity)) {
    if (k == null || k === false) continue;
    el.append(k instanceof Node ? k : String(k));
  }
}

/** put empties a node and fills it again. */
function put(el, ...kids) { el.replaceChildren(); add(el, kids); return el; }

const $ = (sel) => document.querySelector(sel);

const GiB = 1024 ** 3;

/** bytes writes a size in the unit a person reads: 24 GiB. */
function bytes(n) {
  if (!n) return "0";
  const u = ["B", "KiB", "MiB", "GiB", "TiB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (n >= 100 || i === 0 ? n.toFixed(0) : n.toFixed(1)) + " " + u[i];
}

/** num writes a number with one decimal, or a dash when there is none. */
function num(v, d = 1) {
  if (v == null || v === "") return "–";
  if (typeof v !== "number") return String(v);
  if (Number.isInteger(v)) return String(v);
  return v.toFixed(d);
}

/** ago writes how long ago a time was. */
function ago(t) {
  const s = (Date.now() - new Date(t).getTime()) / 1000;
  if (!isFinite(s)) return "";
  if (s < 60) return Math.round(s) + " s ago";
  if (s < 3600) return Math.round(s / 60) + " min ago";
  if (s < 86400) return Math.round(s / 3600) + " h ago";
  return Math.round(s / 86400) + " d ago";
}

/** badge is a word in a colour: the state of a run or of a check. */
function badge(status) { return h("span.badge", { class: "b-" + status }, status || "?"); }

/** sh quotes one word of a command for a shell. */
function sh(a) { return /^[\w@%+=:,./-]+$/.test(a) ? a : "'" + a.replace(/'/g, "'\\''") + "'"; }
