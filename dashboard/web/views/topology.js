"use strict";
// ---------- Topology ----------

async function viewTopology(root) {
  put(root, loading());
  try {
    const [topo, check] = await Promise.all([
      api("/api/topo"),
      api("/api/topo/check").catch(() => null),
      loadHW().catch(() => null),
    ]);
    const nodes = topo.nodes || [];
    const pairs = topo.pairs || [];
    if (!nodes.length) {
      put(root, h("div", p2pCard(check), h("div.gap"),
        h("div.card", h("h2", "Topology"), h("div.empty", "No GPUs found, so there is no PCIe topology to show."))));
      return;
    }
    const groups = new Map();
    for (const n of nodes) {
      const k = n.switch || "";
      if (!groups.has(k)) groups.set(k, []);
      groups.get(k).push(n);
    }
    const tree = [...groups.entries()].map(([sw, ns]) => h("div.switch",
      h("h3", sw ? "PCIe switch " + sw : "No PCIe switch (direct to root port)"),
      h("table", h("thead", h("tr", h("th", "HIP"), h("th", "GPU"), h("th", "PCI"), h("th", "Root port"), h("th", "Root"))),
        h("tbody", ns.map((n) => h("tr", h("td", n.hip_index), h("td", gpuName(n.hip_index)), h("td.mono", n.pci), h("td.mono", n.root_port || "–"), h("td.mono", n.root || "–")))))));

    const idx = nodes.map((n) => n.hip_index).sort((a, b) => a - b);
    const pairOf = (a, b) => pairs.find((p) => (p.a === a && p.b === b) || (p.a === b && p.b === a));
    const matrix = h("table.matrix",
      h("thead", h("tr", h("th", ""), idx.map((i) => h("th", i)))),
      h("tbody", idx.map((a) => h("tr", h("th", a), idx.map((b) => {
        if (a === b) return h("td.self", "X");
        const p = pairOf(a, b);
        if (!p) return h("td", "?");
        return h("td", { class: "c-" + p.class, title: `${a}↔${b}: ${p.class}, ${p.hops ?? "?"} hops, P2P ${p.p2p ? "yes" : "no"}` },
          p.class + (p.p2p ? " •" : ""), p.gbps ? h("span.gb", num(p.gbps) + " GB/s") : null);
      })))));
    put(root, h("div", p2pCard(check), h("div.gap"),
      h("div.cols",
        h("div.card", h("h2", "PCIe tree"), tree),
        h("div.card", h("h2", "Link classes", h("span.grow"), topo.bandwidth ? h("span.small.muted", "bandwidth: " + topo.bandwidth) : null),
          h("div.scroll", matrix),
          h("div.legend", ["PIX same switch", "PXB several switches", "PHB host bridge", "SYS across CPUs"].map((s) => h("span", { class: "c-" + s.slice(0, 3) }, s)),
            h("span", "• = KFD P2P link")),
          topo.bandwidth ? null : h("div.small.muted", "Run `bernie topo --bandwidth` to measure GB/s per pair.")))));
  } catch (e) {
    put(root, failed(e));
  }
}

// p2pCard shows the readiness checklist of `bernie topo --check`.
function p2pCard(data) {
  if (!data) {
    return h("div.card", h("h2", "P2P readiness"), h("div.empty", "The P2P readiness checklist is not available."));
  }
  const items = data.checklist || [];
  const sum = data.summary || {};
  const counts = ["ok", "warn", "fail", "skip"].map((s) => `${sum[s] || 0} ${s}`).join(" · ");
  let label = "ready", cls = "b-ok";
  if (sum.fail) { label = "not ready"; cls = "b-fail"; }
  else if (!sum.ok && !sum.warn) { label = "nothing to check"; cls = "b-skip"; }
  return h("div.card",
    h("h2", "P2P readiness", h("span.grow"),
      h("span.small.muted", counts),
      h("span.badge", { class: cls }, label)),
    items.length ? items.map(p2pRow) : h("div.empty", "No checklist items."));
}

// p2pRow shows one checklist item: its state, the cause and the fix.
function p2pRow(it) {
  return h("div.check",
    h("div.t", badge(it.status), h("b", it.title)),
    it.detail ? h("div.muted.small", it.detail) : null,
    it.fix ? h("div.row", h("code", it.fix), copyBtn(it.fix)) : null,
    it.doc ? h("a.small", { href: it.doc, target: "_blank", rel: "noopener noreferrer" }, "docs") : null);
}
