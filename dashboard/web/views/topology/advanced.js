"use strict";
// ---------- the advanced panel of the Topology tab ----------
// The raw documents behind the tab and the commands that make them.

/** topoBandwidth is the state of the bandwidth of this host. The dashboard
 *  never measures it: `bernie topo --bandwidth` does. The tab says so
 *  instead of showing a number it does not have. */
function topoBandwidth(data) {
  const pairs = (data && data.pairs) || [];
  const measured = pairs.filter((p) => p.gbps);
  if (!measured.length) {
    return emptyState("The bandwidth of this host is not measured.",
      "The dashboard never measures the bandwidth of the cards. The command below measures the GB/s of every pair.",
      "bernie topo --bandwidth");
  }
  return h("div",
    h("p.small.muted", "The GB/s of every pair, as the last measurement found it."),
    topoPairTable(data));
}

/** topoAdvanced is the panel with the raw documents and the commands that
 *  make them. */
function topoAdvanced() {
  const docs = [
    ["/api/topo", topo.data, "bernie topo"],
    ["/api/topo/check", topo.check, "bernie topo --check"],
  ];
  const body = h("div.advanced-body");
  for (const [path, doc, command] of docs) {
    body.append(h("h3", path), cmdBlock(command, "Copy the command"));
    const pre = h("pre", { class: "topo-raw-json" });
    pre.textContent = doc == null ? "not read yet" : JSON.stringify(doc, null, 2);
    body.append(pre);
  }
  return h("details.advanced", h("summary", "Advanced"), body);
}
