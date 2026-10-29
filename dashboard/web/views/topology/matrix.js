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

/** topoBandwidthMatrix is the matrix of the measured bandwidth: one cell
 *  per pair of cards. The cell carries the number of GB/s in words, the
 *  link class beside it and the shape of the peer link, so the matrix is
 *  readable without the colour. The colour is the heat of the number:
 *  slower pairs are cooler (ia.md §5 R32). */
function topoBandwidthMatrix(data) {
  const nodes = (data && data.nodes) || [];
  const pairs = (data && data.pairs) || [];
  if (nodes.length < 2) {
    return emptyState("There is no matrix to show.",
      "The matrix compares every pair of cards. It needs at least two cards on this host.",
      "bernie topo");
  }
  const measured = pairs.filter((p) => p.gbps);
  if (!measured.length) {
    return emptyState("The bandwidth of this host is not measured.",
      "The measurement runs copies between the cards and takes minutes. The command below measures the GB/s of every pair and saves it, so the page shows it afterwards.",
      "bernie topo --bandwidth");
  }
  const idx = nodes.map((n) => n.hip_index).sort((a, b) => a - b);
  const pairOf = (a, b) => pairs.find((p) => (p.a === a && p.b === b) || (p.a === b && p.b === a));
  const peak = Math.max.apply(null, measured.map((p) => p.gbps));
  // The heat of a cell is one of four steps, so the colour never carries
  // the meaning alone: the number is in the cell.
  const heat = (v) => {
    if (!v || !peak) return "";
    const r = v / peak;
    if (r >= 0.75) return " topo-bw-4";
    if (r >= 0.5) return " topo-bw-3";
    if (r >= 0.25) return " topo-bw-2";
    return " topo-bw-1";
  };
  const head = (text) => h("th", { scope: "col" }, text);
  const rows = idx.map((a) => h("tr",
    head(h("span.topo-matrix-name", "GPU " + a)),
    idx.map((b) => {
      if (a === b) return h("td.self", { scope: "row" }, "—");
      const p = pairOf(a, b);
      if (!p) {
        return h("td", { class: "topo-cell topo-cell-unknown",
          title: "GPU " + a + " and GPU " + b + ": the path is not known" }, "?");
      }
      const gbps = p.gbps || 0;
      const word = gbps ? num(gbps) + " GB/s" : "not measured";
      const title = "GPU " + a + " and GPU " + b + ": " + word +
        ", " + (TOPO_CLASS_WORD[p.class] || "the path is not known") +
        ", " + (p.p2p ? "a direct copy is possible" : "the copy goes through the host");
      return h("td", { class: "topo-cell" + heat(gbps), title: title },
        h("span.topo-cell-gbps", word),
        h("span.topo-cell-code", p.class),
        h("span.topo-cell-mark", p.p2p ? "●" : "○"));
    })));
  const table = h("div.scroll", h("table.matrix.topo-matrix.topo-bw-matrix",
    h("thead", h("tr", head(""), idx.map((i) => head(h("div.topo-matrix-head", "GPU " + i))))),
    h("tbody", rows)));
  return h("div",
    h("p.small.muted", "The GB/s of every pair, as the last measurement found it."),
    table);
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
