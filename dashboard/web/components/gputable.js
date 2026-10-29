// The exact per-GPU table: every measured value of every AMD GPU on this
// host. One function serves the Dashboard's details and the Advanced tab,
// so the two places can never drift apart.
"use strict";

/** gpuValueTable is the table of the exact values of every card: the load,
 *  the memory, the power, the three temperatures, the two clocks and the
 *  ECC state. A value that was not measured is a dash, never a zero.
 *
 *  gpus    the cards of /api/hw
 *  samples the samples of the telemetry stream (each one carries a hip_index)
 *  checks  the checks of /api/doctor (the ECC check of a card is gpu.ecc.<hip>)
 *
 *  It returns the element and the function that repaints it, so the stream
 *  rewrites the numbers without building the table again.
 */
function gpuValueTable(gpus, samples, checks) {
  const body = h("tbody");
  const el = h("div.scroll", h("table.gpu-value-table",
    tableCaption("Every measured value of every AMD GPU on this host."),
    tableHead(["GPU", { label: "Load", num: true }, { label: "Memory", num: true },
      { label: "Power", num: true }, { label: "Edge", num: true },
      { label: "Junction", num: true }, { label: "Memory temp", num: true },
      { label: "SCLK", num: true }, { label: "MCLK", num: true }, "ECC"]),
    body));

  // The rows of the cards of the host, kept between the repaints.
  const rows = new Map();
  let shape = "";

  /** row builds the row of one card and keeps its cells. */
  const row = (i, hw) => {
    const name = h("div.small.muted", hw.name || "");
    const cells = {
      name: name,
      load: h("td.num"),
      mem: h("td.num"),
      pow: h("td.num"),
      edge: h("td.num"),
      junction: h("td.num"),
      memtemp: h("td.num"),
      sclk: h("td.num"),
      mclk: h("td.num"),
      ecc: h("td"),
    };
    const tr = h("tr", h("td", h("b", "GPU " + i), name),
      cells.load, cells.mem, cells.pow, cells.edge, cells.junction, cells.memtemp,
      cells.sclk, cells.mclk, cells.ecc);
    rows.set(i, { tr: tr, cells: cells });
    return tr;
  };

  const update = (nextGpus, nextSamples, nextChecks) => {
    const cards = nextGpus || [];
    const byIndex = new Map();
    for (const s of nextSamples || []) byIndex.set(s.hip_index, s);
    const ids = cards.length ? cards.map((g) => g.hip_index) : [...byIndex.keys()];
    const nextShape = ids.join(",");
    if (nextShape !== shape) {
      shape = nextShape;
      for (const i of ids) {
        if (!rows.has(i)) row(i, cards.find((g) => g.hip_index === i) || {});
      }
      for (const i of [...rows.keys()]) {
        if (!ids.includes(i)) rows.delete(i);
      }
      body.replaceChildren(...ids.map((i) => rows.get(i).tr));
    }
    const all = nextChecks || [];
    for (const i of ids) {
      const cells = rows.get(i).cells;
      const s = byIndex.get(i) || null;
      const hw = cards.find((g) => g.hip_index === i) || {};
      const temp = (v) => (v == null ? "–" : num(v) + " °C");
      const clock = (v) => (v == null ? "–" : num(v) + " MHz");
      cells.name.textContent = hw.name || "";
      cells.load.textContent = s && s.busy_pct != null ? num(s.busy_pct) + " %" : "–";
      cells.mem.textContent = s && s.vram_total != null
        ? num(s.vram_used / GiB) + " / " + num(s.vram_total / GiB) + " GiB" : "–";
      cells.pow.textContent = s && s.power_w != null
        ? num(s.power_w) + (s.power_cap_w != null ? " / " + num(s.power_cap_w) : "") + " W" : "–";
      cells.edge.textContent = temp(s && s.temp_c);
      cells.junction.textContent = temp(s && s.temp_junction_c);
      cells.memtemp.textContent = temp(s && s.temp_mem_c);
      cells.sclk.textContent = clock(s && s.sclk_mhz);
      cells.mclk.textContent = clock(s && s.mclk_mhz);
      const check = all.find((c) => c.id === "gpu.ecc." + i);
      put(cells.ecc, check ? badge(check.status) : h("span.badge.b-skip", "unknown"));
    }
  };

  update(gpus, samples, checks);
  return { el: el, update: update };
}
