"use strict";
// ---------- The topology ----------
// The PCIe tree of the cards, the matrix of the link classes and the
// readiness checklist of `bernie topo --check`.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.

// DASH_CLASS_COLOR is the colour of one link class. The class is written in
// every cell, so the colour only helps.
const DASH_CLASS_COLOR = {
  PIX: "var(--series-2)",
  PXB: "var(--series-1)",
  PHB: "var(--series-3)",
  SYS: "var(--series-4)",
};

// dashTopology is the section with the tree, the matrix and the checklist.
function dashTopology(section) {
  const el = h("div.cols");
  section.append(el);
  const tree = h("div.card");
  const matrix = h("div.card");
  const check = h("div.card");
  el.append(tree, matrix, check);
  const api = {
    update: () => {
      const topo = dash.topo;
      put(tree, cardHead("PCIe tree", "/api/topo"), dashTree(topo));
      put(matrix, cardHead("Link classes", "/api/topo"), dashMatrix(topo));
      put(check, cardHead("P2P readiness", "/api/topo/check"), dashChecklist(dash.check));
    },
  };
  api.update();
  return api;
}

// dashTree is the tree of the cards: the switch of every card, its PCI
// address, its root port and its root complex.
function dashTree(topo) {
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
    const block = h("div", { style: "margin-bottom:12px" });
    block.append(h("h3", { style: "font:700 13px/1.3 var(--font-mono);margin:0 0 6px" }, sw ? "PCIe switch " + sw : "No PCIe switch (direct to the root port)"));
    block.append(h("div.scroll", h("table",
      h("thead", h("tr", ["Card", "Name", "PCI", "Root port", "Root"].map((c) => h("th", c)))),
      h("tbody", ns.map((n) => h("tr", null,
        h("td", "GPU " + n.hip_index),
        h("td", dashCardName(n.hip_index)),
        h("td", { style: "font-family:var(--font-mono);font-size:12px" }, n.pci || "–"),
        h("td", { style: "font-family:var(--font-mono);font-size:12px" }, n.root_port || "–"),
        h("td", { style: "font-family:var(--font-mono);font-size:12px" }, n.root || "–")))))));
    box.append(block);
  }
  return box;
}

// dashMatrix is the heat map of the link classes: one cell per pair of cards,
// the class in the cell and a dot where a KFD peer link exists.
function dashMatrix(topo) {
  const nodes = (topo && topo.nodes) || [];
  const pairs = (topo && topo.pairs) || [];
  if (!nodes.length) {
    return h("div", { style: "color:var(--muted);font-size:13px" }, "The matrix needs at least one card.");
  }
  const idx = nodes.map((n) => n.hip_index).sort((a, b) => a - b);
  const pairOf = (a, b) => pairs.find((p) => (p.a === a && p.b === b) || (p.a === b && p.b === a));
  const head = (text) => h("th", text);
  const rows = idx.map((a) => h("tr", null,
    head(h("span", { style: "font-weight:600;color:var(--text)" }, "GPU " + a), h("div", { style: "font-weight:400;font-size:11px" }, dashCardName(a))),
    idx.map((b) => {
      if (a === b) return h("td.self", "—");
      const p = pairOf(a, b);
      const cellStyle = "border:2px solid var(--bg);border-radius:6px;font-weight:600";
      if (!p) return h("td", { style: cellStyle + ";color:var(--muted)", title: a + " and " + b + ": no link class" }, "?");
      return h("td", {
        class: "c-" + p.class, style: cellStyle,
        title: a + " and " + b + ": " + p.class + ", " + (p.hops != null ? p.hops + " hops" : "unknown hops") + ", peer to peer " + (p.p2p ? "yes" : "no"),
      }, p.class, p.p2p ? h("span", { style: "color:var(--text);margin-left:3px" }, "•") : null,
        p.gbps ? h("span", { style: "display:block;font-size:10.5px;color:var(--muted);font-weight:400" }, num(p.gbps) + " GB/s") : null);
    })));
  const legend = h("div", { style: "display:flex;gap:10px;flex-wrap:wrap;font-size:12px;margin-top:10px;color:var(--text-2)" },
    ["PIX same switch", "PXB several switches", "PHB host bridge", "SYS across CPUs"].map((text) => {
      const cls = text.slice(0, 3);
      return h("span", { class: "c-" + cls }, text);
    }),
    h("span", "• = a KFD peer link"));
  return h("div", h("div.scroll", h("table.matrix", { style: "border-collapse:separate;border-spacing:2px" },
    h("thead", h("tr", head(""), idx.map((i) => head(h("div", { style: "font-weight:600;color:var(--text);max-width:96px;overflow-wrap:anywhere" }, dashCardName(i)), h("div", { style: "font-weight:400;font-size:11px" }, "GPU " + i))))),
    h("tbody", rows))), legend,
    topo && topo.bandwidth ? null : h("div", { style: "font-size:12px;color:var(--muted);margin-top:8px" }, "Run bernie topo --bandwidth to measure the GB/s of every pair."));
}

// dashChecklist is the readiness checklist of `bernie topo --check`.
function dashChecklist(data) {
  if (!data) {
    const err = (dash.errors || {})["/api/topo/check"];
    return h("div", null,
      h("div", { style: "color:var(--muted);font-size:13px" }, "The checklist is not available."),
      err ? h("div", { style: "color:var(--muted);font-size:12px;margin-top:4px" }, err) : null,
      h("button", { type: "button", onclick: () => dashLoad() }, "Try again"));
  }
  const items = data.checklist || [];
  const sum = data.summary || {};
  const failed = sum.fail || 0;
  const box = h("div");
  const verdict = failed
    ? "Multi-GPU copies will not work: " + failed + (failed === 1 ? " check failed." : " checks failed.")
    : "Multi-GPU copies will work on this host.";
  box.append(h("div", { style: "display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:10px" },
    dashPill(failed ? "fail" : "ok", failed ? "not ready" : "ready"),
    h("span", { style: "font-size:13px;color:var(--text-2)" }, verdict)));
  if (!items.length) {
    box.append(h("div", { style: "color:var(--muted);font-size:13px" }, "Nothing to check on this host."));
    return box;
  }
  for (const it of items) {
    const kind = it.status === "ok" ? "ok" : it.status === "warn" ? "warn" : it.status === "fail" ? "fail" : "skip";
    box.append(h("div", { style: "display:grid;grid-template-columns:auto 1fr;gap:4px 12px;padding:10px 0;border-bottom:1px solid var(--border)" },
      h("div", { style: "display:flex;gap:8px;align-items:baseline" }, dashPill(kind, it.status === "skip" ? "skip" : it.status), h("b", { style: "font-size:13px" }, it.title || it.id)),
      it.detail ? h("div", { style: "grid-column:2;color:var(--text-2);font-size:13px" }, it.detail) : null,
      it.fix ? h("div", { style: "grid-column:2;margin-top:6px" }, cmdBlock(it.fix, "Copy the fix")) : null,
      it.doc ? h("a", { href: it.doc, target: "_blank", rel: "noopener noreferrer", style: "grid-column:2;font-size:12px;margin-top:4px" }, "the rule in the documentation") : null));
  }
  return box;
}
