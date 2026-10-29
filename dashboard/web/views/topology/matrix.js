"use strict";
// ---------- the matrix of the link classes ----------
// One cell per pair of cards. The class is written in the cell and the
// shape marks the peer link, so the matrix is readable without the colours.
//
// The classes are the ones of spec §4.2: PIX (both cards under the same
// PCIe switch), PXB (the path crosses more than one switch but no root
// complex), PHB (the path crosses one root complex, the host bridge) and
// SYS (the path crosses the CPU interconnect).

// TOPO_CLASS_WORD is what a link class means in words.
const TOPO_CLASS_WORD = {
  PIX: "same PCIe switch",
  PXB: "across PCIe switches",
  PHB: "through the CPU host bridge",
  SYS: "across CPU sockets",
};

// TOPO_CLASS_ORDER is the order of the classes in the legend: the closest
// path first.
const TOPO_CLASS_ORDER = ["PIX", "PXB", "PHB", "SYS"];

/** topoClassWords is the meaning of one class, in one sentence. */
function topoClassWords(cls) {
  switch (cls) {
    case "PIX": return "Both cards hang under the same PCIe switch. This is the fastest path.";
    case "PXB": return "The path crosses more than one PCIe switch, but no root complex.";
    case "PHB": return "The path crosses one root complex: the host bridge of the CPU.";
    case "SYS": return "The path crosses the CPU interconnect. A direct copy is not reachable here.";
    default: return "The class of this path is not known.";
  }
}

/** topoMatrix is the matrix of the link classes: one cell per pair of
 *  cards, the class in the cell and the shape of the peer link. */
function topoMatrix(data) {
  const nodes = (data && data.nodes) || [];
  const pairs = (data && data.pairs) || [];
  if (nodes.length < 2) {
    return emptyState("There is no matrix to show.",
      "The matrix compares every pair of cards. It needs at least two cards on this host.",
      "bernie topo");
  }
  const idx = nodes.map((n) => n.hip_index).sort((a, b) => a - b);
  const pairOf = (a, b) => pairs.find((p) => (p.a === a && p.b === b) || (p.a === b && p.b === a));
  const head = (text) => h("th", { scope: "col" }, text);
  const rows = idx.map((a) => h("tr", null,
    head(h("span.topo-matrix-name", "GPU " + a)),
    idx.map((b) => {
      if (a === b) return h("td.self", { scope: "row" }, "—");
      const p = pairOf(a, b);
      if (!p) {
        return h("td", { class: "topo-cell topo-cell-unknown",
          title: "GPU " + a + " and GPU " + b + ": the path is not known" }, "?");
      }
      const word = TOPO_CLASS_WORD[p.class] || "the path is not known";
      const title = "GPU " + a + " and GPU " + b + ": " + p.class + ", " + word +
        ", " + (p.hops != null ? p.hops + " hops" : "the number of hops is not known") +
        ", " + (p.p2p ? "a direct copy is possible" : "the copy goes through the host");
      return h("td", { class: "topo-cell topo-c-" + p.class, title: title },
        h("span.topo-cell-code", p.class),
        h("span.topo-cell-mark", p.p2p ? "●" : "○"),
        p.gbps ? h("span.topo-cell-gbps", num(p.gbps) + " GB/s") : null);
    })));
  const table = h("div.scroll", h("table.matrix.topo-matrix",
    h("thead", h("tr", head(""), idx.map((i) => head(h("div.topo-matrix-head", "GPU " + i))))),
    h("tbody", rows)));
  return table;
}

/** topoPairTable is the table of every pair: its path, its number of hops
 *  and whether the two cards can copy to each other. */
function topoPairTable(data) {
  const pairs = (data && data.pairs) || [];
  if (!pairs.length) {
    return emptyState("There are no pairs to show.",
      "A pair is two cards of this host. There are none yet.", "bernie topo");
  }
  const cols = tableHead(["Pair", "Path", "Hops", "Peer link", "Measured"]);
  const rows = pairs.map((p) => h("tr",
    h("td", "GPU " + p.a + " and GPU " + p.b),
    h("td", h("span", { class: "topo-pair-class topo-c-" + p.class }, p.class),
      h("span.topo-pair-word", TOPO_CLASS_WORD[p.class] || "the path is not known")),
    h("td.num", p.hops == null ? "–" : String(p.hops)),
    h("td", p.p2p ? statePill("ok", "direct copy") : statePill("skip", "through the host")),
    h("td.num", p.gbps ? num(p.gbps) + " GB/s" : "not measured")));
  return h("div.scroll", h("table", cols, h("tbody", rows)));
}
