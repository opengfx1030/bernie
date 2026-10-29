"use strict";
// ---------- the PCIe tree and the legend of the link classes ----------
// The tree groups the cards by the PCIe switch they hang under. The legend
// says what the four link classes of spec §4.2 mean in words: the colour of
// a class only helps, the word carries the meaning.

/** topoLegend is the legend of the matrix: the class, its meaning in words
 *  and the shape of the peer link. */
function topoLegend() {
  const rows = TOPO_CLASS_ORDER.map((cls) => h("div.topo-legend-row",
    h("span.topo-legend-code", { class: "topo-c-" + cls }, cls),
    h("span.topo-legend-word", h("b", TOPO_CLASS_WORD[cls]), h("span", topoClassWords(cls)))));
  rows.push(h("div.topo-legend-row",
    h("span.topo-legend-code", "●"), h("span.topo-legend-word", h("b", "direct copy"),
      h("span", "The pair has a KFD peer link: the cards copy to each other without the CPU.")),
    h("span.topo-legend-code", "○"), h("span.topo-legend-word", h("b", "through the host"),
      h("span", "The pair has no peer link: the copy is staged through the CPU."))));
  return h("div.topo-legend", rows);
}

/** topoTree is the PCIe tree of the cards: one block per PCIe switch, with
 *  the cards under it, their PCI address and their root port. */
function topoTree(data) {
  const nodes = (data && data.nodes) || [];
  if (nodes.length < 2) {
    return emptyState("There is no PCIe tree to show.",
      "The tree is the way the cards are wired to each other. It needs at least two cards on this host.",
      "bernie topo");
  }
  const groups = new Map();
  for (const n of nodes) {
    const key = n.switch || "";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(n);
  }
  const box = h("div");
  for (const [sw, ns] of groups) {
    const block = h("div.topo-block");
    block.append(h("h3.topo-switch", sw ? "PCIe switch " + sw : "No PCIe switch (direct to the root port)"));
    block.append(h("div.scroll", h("table",
      h("thead", h("tr", ["Card", "PCI address", "Root port", "Root complex"]
        .map((c) => h("th", { scope: "col" }, c)))),
      h("tbody", ns.map((n) => h("tr",
        h("td", "GPU " + n.hip_index),
        h("td.topo-mono", n.pci || "–"),
        h("td.topo-mono", n.root_port || "–"),
        h("td.topo-mono", n.root || "–")))))));
    box.append(block);
  }
  return box;
}
