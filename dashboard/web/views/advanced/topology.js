"use strict";
// ---------- The topology ----------
// The PCIe tree of the cards, the matrix of the link classes and the
// readiness checklist of `bernie topo --check`.
//
// Adapted from vllm-rdna-adv by CrispyGinger, used with permission.

// ADV_CLASS_COLOR is the colour of one link class. The class is written in
// every cell, so the colour only helps.
const ADV_CLASS_COLOR = {
  PIX: "var(--series-2)",
  PXB: "var(--series-1)",
  PHB: "var(--series-3)",
  SYS: "var(--series-4)",
};

// advTopology is the section with the tree, the matrix and the checklist.
function advTopology(section) {
  const el = h("div.cols");
  section.append(el);
  const tree = h("div.card");
  const matrix = h("div.card");
  const check = h("div.card");
  el.append(tree, matrix, check);
  const api = {
    update: () => {
      const topo = adv.topo;
      put(tree, cardHead("PCIe tree", "/api/topo"), advTree(topo));
      put(matrix, cardHead("Link classes", "/api/topo"), advMatrix(topo));
      put(check, cardHead("P2P readiness", "/api/topo/check"), advChecklist(adv.check));
    },
  };
  api.update();
  return api;
}

// advTree is the tree of the cards: the switch of every card, its PCI
// address, its root port and its root complex.
function advTree(topo) {
  const nodes = (topo && topo.nodes) || [];
  if (!nodes.length) {
    return emptyState("There is no PCIe tree to show.",
      "The tree is the way the cards are wired to each other. It needs at least one GPU.");
  }
  const groups = new Map();
  for (const n of nodes) {
    const key = n.switch || "";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(n);
  }
  const box = h("div");
  for (const [sw, ns] of groups) {
    const block = h("div", { class: "adv-mb12" });
    block.append(h("h3", { class: "adv-topo-switch" }, sw ? "PCIe switch " + sw : "No PCIe switch (direct to the root port)"));
    block.append(h("div.scroll", h("table",
      h("thead", h("tr", ["Card", "Name", "PCI", "Root port", "Root"].map((c) => h("th", c)))),
      h("tbody", ns.map((n) => h("tr", null,
        h("td", "GPU " + n.hip_index),
        h("td", advCardName(n.hip_index)),
        h("td", { class: "adv-mono-cell" }, n.pci || "–"),
        h("td", { class: "adv-mono-cell" }, n.root_port || "–"),
        h("td", { class: "adv-mono-cell" }, n.root || "–")))))));
    box.append(block);
  }
  return box;
}

// advMatrix is the heat map of the link classes: one cell per pair of cards,
// the class in the cell and a dot where a KFD peer link exists.
function advMatrix(topo) {
  const nodes = (topo && topo.nodes) || [];
  const pairs = (topo && topo.pairs) || [];
  if (!nodes.length) {
    return h("div", { class: "adv-note-13" }, "The matrix needs at least one card.");
  }
  const idx = nodes.map((n) => n.hip_index).sort((a, b) => a - b);
  const pairOf = (a, b) => pairs.find((p) => (p.a === a && p.b === b) || (p.a === b && p.b === a));
  const head = (text) => h("th", text);
  const rows = idx.map((a) => h("tr", null,
    head(h("span", { class: "adv-matrix-name" }, "GPU " + a), h("div", { class: "adv-tiny" }, advCardName(a))),
    idx.map((b) => {
      if (a === b) return h("td.self", "—");
      const p = pairOf(a, b);
      if (!p) return h("td", { class: "adv-matrix-cell adv-matrix-cell-unknown", title: a + " and " + b + ": no link class" }, "?");
      return h("td", {
        class: "c-" + p.class + " adv-matrix-cell",
        title: a + " and " + b + ": " + p.class + ", " + (p.hops != null ? p.hops + " hops" : "unknown hops") + ", peer to peer " + (p.p2p ? "yes" : "no"),
      }, p.class, p.p2p ? h("span", { class: "adv-matrix-dot" }, "•") : null,
        p.gbps ? h("span", { class: "adv-matrix-gbps" }, num(p.gbps) + " GB/s") : null);
    })));
  const legend = h("div", { class: "legend" },
    ["PIX same switch", "PXB several switches", "PHB host bridge", "SYS across CPUs"].map((text) => {
      const cls = text.slice(0, 3);
      return h("span", { class: "c-" + cls }, text);
    }),
    h("span", "• = a KFD peer link"));
  return h("div", h("div.scroll", h("table.matrix.adv-matrix-table",
    h("thead", h("tr", head(""), idx.map((i) => head(h("div", { class: "adv-matrix-head" }, advCardName(i)), h("div", { class: "adv-tiny" }, "GPU " + i))))),
    h("tbody", rows))), legend,
    topo && topo.bandwidth ? null : h("div", { class: "adv-muted-12" }, "Run bernie topo --bandwidth to measure the GB/s of every pair."));
}

// advChecklist is the readiness checklist of `bernie topo --check`.
function advChecklist(data) {
  if (!data) {
    const err = (adv.errors || {})["/api/topo/check"];
    return h("div", null,
      h("div", { class: "adv-note-13" }, "The checklist is not available."),
      err ? h("div", { class: "adv-muted-12-note" }, err) : null,
      h("button", { type: "button", onclick: () => advLoad() }, "Try again"));
  }
  const items = data.checklist || [];
  const sum = data.summary || {};
  const failed = sum.fail || 0;
  const box = h("div");
  const verdict = failed
    ? "Multi-GPU copies will not work: " + failed + (failed === 1 ? " check failed." : " checks failed.")
    : "Multi-GPU copies will work on this host.";
  box.append(h("div", { class: "adv-verdict" },
    advPill(failed ? "fail" : "ok", failed ? "not ready" : "ready"),
    h("span", { class: "adv-note-13-text2" }, verdict)));
  if (!items.length) {
    box.append(h("div", { class: "adv-note-13" }, "Nothing to check on this host."));
    return box;
  }
  for (const it of items) {
    const kind = it.status === "ok" ? "ok" : it.status === "warn" ? "warn" : it.status === "fail" ? "fail" : "skip";
    box.append(h("div", { class: "adv-check-row" },
      h("div", { class: "adv-check-head" }, advPill(kind, it.status === "skip" ? "skip" : it.status), h("b", { class: "adv-small-13" }, it.title || it.id)),
      it.detail ? h("div", { class: "adv-check-detail" }, it.detail) : null,
      it.fix ? h("div", { class: "adv-check-fix" }, cmdBlock(it.fix, "Copy the fix")) : null,
      it.doc ? h("a", { href: it.doc, target: "_blank", rel: "noopener noreferrer", class: "adv-check-doc" }, "the rule in the documentation") : null));
  }
  return box;
}
