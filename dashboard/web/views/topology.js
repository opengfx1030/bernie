"use strict";
// ---------- Topology ----------

async function viewTopology(root) {
  put(root, loading());
  try {
    const [topo] = await Promise.all([api("/api/topo"), loadHW().catch(() => null)]);
    const nodes = topo.nodes || [];
    const pairs = topo.pairs || [];
    if (!nodes.length) {
      put(root, h("div.card", h("h2", "Topology"), h("div.empty", "No GPUs found, so there is no PCIe topology to show.")));
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
    put(root, h("div.cols",
      h("div.card", h("h2", "PCIe tree"), tree),
      h("div.card", h("h2", "Link classes", h("span.grow"), topo.bandwidth ? h("span.small.muted", "bandwidth: " + topo.bandwidth) : null),
        h("div.scroll", matrix),
        h("div.legend", ["PIX same switch", "PXB several switches", "PHB host bridge", "SYS across CPUs"].map((s) => h("span", { class: "c-" + s.slice(0, 3) }, s)),
          h("span", "• = KFD P2P link")),
        topo.bandwidth ? null : h("div.small.muted", "Run `bernie topo --bandwidth` to measure GB/s per pair."))));
  } catch (e) {
    put(root, failed(e));
  }
}
