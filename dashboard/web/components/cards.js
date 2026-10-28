// Shared dashboard cards and state headlines.
"use strict";

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

/** stateHead is the block that opens a tab: the label of the tab, the
 *  headline that says what the situation is and the line under it. The
 *  headline is read out when it changes. */
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

function loading() { return h("div.empty", h("span.spin"), " loading…"); }
function failed(e) { return h("div.err", String(e.message || e)); }
