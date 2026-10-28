// Shared metric components: tiles, rings, meters and status pills.
"use strict";

/** tile is one number with its unit, its context and a small chart. */
function tile(label, value, unit, sub, values, max, aria) {
  return h("article.tile",
    h("p.tile-l", label),
    h("p.tile-n", String(value), unit ? h("small", unit) : null),
    h("p.tile-s", sub),
    sparkSvg(values || [], max, aria));
}

/** busyRing draws the load of a card as a ring: the number in the middle and
 *  the track behind it. */
function busyRing() {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("class", "ring");
  svg.setAttribute("viewBox", "0 0 84 84");
  svg.setAttribute("role", "img");
  const bg = document.createElementNS(ns, "circle");
  bg.setAttribute("class", "ring-bg");
  const fg = document.createElementNS(ns, "circle");
  fg.setAttribute("class", "ring-fg");
  for (const c of [bg, fg]) {
    c.setAttribute("cx", "42");
    c.setAttribute("cy", "42");
    c.setAttribute("r", "36");
  }
  const circumference = 2 * Math.PI * 36;
  fg.setAttribute("stroke-dasharray", circumference + " " + circumference);
  fg.setAttribute("stroke-dashoffset", String(circumference));
  const n = document.createElementNS(ns, "text");
  n.setAttribute("class", "ring-n");
  n.setAttribute("x", "42");
  n.setAttribute("y", "44");
  const u = document.createElementNS(ns, "text");
  u.setAttribute("class", "ring-u");
  u.setAttribute("x", "42");
  u.setAttribute("y", "56");
  u.textContent = "%";
  svg.append(bg, fg, n, u);
  return svg;
}

/** setRing writes the load of a card into its ring. */
function setRing(ring, pct) {
  if (!ring) return;
  const p = Math.max(0, Math.min(100, pct || 0));
  const circumference = 2 * Math.PI * 36;
  ring.querySelector(".ring-fg").setAttribute("stroke-dashoffset", String(circumference * (1 - p / 100)));
  ring.querySelector(".ring-n").textContent = String(Math.round(p));
  ring.setAttribute("aria-label", "The card is busy " + Math.round(p) + " percent");
}

/** metricMeter is one labelled value with an optional bar. */
function metricMeter(name, bar = true, title = "", hint = "") {
  return h("div.metric", { "data-m": name },
    h("div.lbl", { title: title }, h("span", name), h("b")),
    bar ? h("div.bar", h("i")) : null,
    hint ? h("div.small.muted", hint) : null);
}

/** statusPill is a compact state label. The word carries the meaning. */
function statusPill(text, state) {
  return h("span.pill", { class: "pill " + state }, text);
}
