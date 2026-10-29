"use strict";
// ---------- The cards ----------
// The GPU section of the first dashboard: the headline with the one action
// of the section, the five tiles that hold the numbers of every card, one
// card per card of the host and the power caps.

/** advViewGPUs builds the section. It keeps its parts, so the stream can
 *  repaint them without rebuilding the section. */
function advViewGPUs(root) {
  const head = stateHead("GPUs", h("button.primary", { type: "button", onclick: advCopyCardList }, "Copy the card list"));
  const tiles = h("div.adv-tiles");
  const grid = h("div.grid.gpus");
  put(root, head, tiles, grid);
  const api = {
    update: () => {
      const gpus = (adv.hw && adv.hw.gpus) || [];
      const samples = new Map((adv.samples.length ? adv.samples[adv.samples.length - 1].gpus : []).map((s) => [s.hip_index, s]));
      if (!gpus.length && !samples.size) {
        setHead(head, "No AMD GPU on this machine",
          "bernie reads the cards from /sys/class/kfd. The amdgpu driver has to be loaded and /dev/kfd has to exist.");
        put(root, head, h("div.card", cardHead("GPUs", "/api/hw"),
          emptyState("No AMD GPU is visible to bernie.", "The amdgpu driver has to be loaded and /dev/kfd has to exist. The doctor checks every part of that.", "sudo modprobe amdgpu")));
        return;
      }
      setHead(head, advGpuHeadline(gpus, samples),
        "The cards, the PCIe link behind them, and what the telemetry stream measures every second: the load, the memory, the power and the temperature.");
      put(root, head, tiles, grid);
      advRenderTiles(tiles, gpus, samples);
      const ids = gpus.length ? gpus.map((g) => g.hip_index) : [...samples.keys()];
      for (const i of ids) {
        let card = grid.querySelector("[data-gpu=\"" + i + "\"]");
        if (!card) grid.append(card = advGpuCard(i));
        advUpdateGpuCard(card, i, samples.get(i));
      }
      advRenderPower(root);
    },
  };
  api.update();
  return api;
}

/** advGpuHeadline says in one sentence how many cards there are and how busy
 *  they are. */
function advGpuHeadline(gpus, samples) {
  if (!gpus.length) return "AMD GPU found";
  const busy = [...samples.values()].filter((s) => s.busy_pct > 5).length;
  return gpus.length + (gpus.length === 1 ? " GPU" : " GPUs") + (busy ? ", " + busy + " busy" : ", all idle");
}

/** advRenderTiles paints the five tiles: the cards in five numbers, each
 *  with the last minute of it. */
function advRenderTiles(box, gpus, samples) {
  const gs = [...samples.values()];
  if (!gs.length) { box.replaceChildren(); return; }
  const n = gs.length;
  const busy = gs.filter((s) => (s.busy_pct || 0) > 5).length;
  const avg = gs.reduce((a, s) => a + (s.busy_pct || 0), 0) / n;
  const power = gs.reduce((a, s) => a + (s.power_w || 0), 0);
  const cap = gs.reduce((a, s) => a + (s.power_cap_w || 0), 0);
  const vram = gs.reduce((a, s) => a + (s.vram_used || 0), 0) / GiB;
  const vramTotal = gs.reduce((a, s) => a + (s.vram_total || 0), 0) / GiB;
  const hot = gs.reduce((a, s) => ((s.temp_c || 0) > (a.temp_c || 0) ? s : a), {});
  const sclk = gs.reduce((a, s) => ((s.sclk_mhz || 0) > (a.sclk_mhz || 0) ? s : a), {});
  const series = (key) => adv.samples.slice(-60).map((s) => {
    const g = s.gpus[0];
    return g ? g[key] : null;
  });
  const tile = (label, value, unit, sub, values, max, tip) => {
    const chart = spark(values, max, tip, { unit: unit });
    return h("div.card.adv-tile",
      h("div.small.muted", label),
      h("div.adv-tile-n", value, h("span", unit)),
      h("div.small.muted", sub),
      chart);
  };
  put(box,
    tile("Busy", String(Math.round(avg)), "%", busy + " of " + n + " cards busy", series("busy_pct"), 100,
      "The average load of the cards, last 60 seconds"),
    tile("Power", String(Math.round(power)), "W", cap ? "of " + Math.round(cap) + " W of cap" : "no cap published", series("power_w"), undefined,
      "The power of all cards, last 60 seconds"),
    tile("Memory", vram.toFixed(1), "GiB", "of " + vramTotal.toFixed(1) + " GiB", series("vram_used"), undefined,
      "The memory in use on all cards, last 60 seconds"),
    tile("Hottest", hot.temp_c == null ? "–" : String(Math.round(hot.temp_c)), "°C",
      hot.hip_index == null ? "no card reports a temperature" : "GPU " + hot.hip_index, series("temp_c"), undefined,
      "The hottest card, last 60 seconds"),
    tile("Clock", sclk.sclk_mhz == null ? "–" : String(Math.round(sclk.sclk_mhz)), "MHz",
      "the highest shader clock", series("sclk_mhz"), undefined,
      "The highest shader clock, last 60 seconds"));
}

/** advGpuCard is the card of one card of the host: its name, the ring of
 *  its load, the memory and the power, and the last minute of its load. */
function advGpuCard(i) {
  const g = ((adv.hw && adv.hw.gpus) || []).find((x) => x.hip_index === i) || {};
  const box = h("div.card.gpu", { "data-gpu": String(i) });
  const ringBox = ring(null, "GPU " + i);
  const chart = spark([], 100, "busy %, last 60 s", { unit: "%" });
  const facts = h("div.small.muted");
  const mem = meter(null, null, { label: "Memory", unit: "GiB", format: (v) => num(v / (1024 * 1024 * 1024)) });
  const pow = meter(null, null, { label: "Power", unit: "W", sep: " of ", format: (v) => num(Math.round(v)) });
  put(box,
    h("div.head", h("h3", "GPU " + i + " ", h("span.muted.small", g.name || "")), h("div.ring-wrap", ringBox)),
    facts,
    h("div.gpu-stats", mem, pow),
    h("div.gpu-spark", chart));
  return box;
}

/** advUpdateGpuCard paints one card with the sample of the stream. */
function advUpdateGpuCard(card, i, sample) {
  const g = ((adv.hw && adv.hw.gpus) || []).find((x) => x.hip_index === i) || {};
  const facts = card.querySelector(".small.muted");
  if (facts) {
    facts.textContent = [g.gfx, g.pci, g.link && g.link.gen ? "PCIe " + g.link.gen + ".0 x" + g.link.width : "",
      g.numa_node >= 0 ? "NUMA " + g.numa_node : ""].filter(Boolean).join(" · ");
  }
  const ringBox = card.querySelector(".ring-wrap");
  if (ringBox && ringBox.firstElementChild && ringBox.firstElementChild.update) {
    ringBox.firstElementChild.update(sample ? sample.busy_pct : null);
  }
  const meters = card.querySelectorAll(".meter");
  if (meters[0] && meters[0].update) meters[0].update(sample ? sample.vram_used : null, sample ? sample.vram_total : null);
  if (meters[1] && meters[1].update) meters[1].update(sample ? sample.power_w : null, sample ? sample.power_cap_w : null);
  const chart = card.querySelector(".gpu-spark");
  if (chart && chart.firstElementChild && chart.firstElementChild.update) {
    chart.firstElementChild.update(adv.samples.slice(-60).map((s) => {
      const one = s.gpus.find((x) => x.hip_index === i);
      return one ? one.busy_pct : null;
    }), 100);
  }
}

/** advCardListText writes the cards as plain text: one line per card, so it
 *  can be pasted into a report or a ticket. */
function advCardListText() {
  const samples = (adv.samples.length ? adv.samples[adv.samples.length - 1].gpus : []);
  const gpus = (adv.hw && adv.hw.gpus) || [];
  if (!samples.length && !gpus.length) return "";
  return samples.map((s) => {
    const g = gpus.find((x) => x.hip_index === s.hip_index) || {};
    return ["GPU " + s.hip_index, g.name || g.gfx || "", num(s.busy_pct, 0) + " % busy",
      num(s.temp_c, 0) + " °C", num(s.power_w, 1) + " W",
      (s.vram_used ? bytes(s.vram_used) : "–") + " / " + (s.vram_total ? bytes(s.vram_total) : "–")].join("\t");
  }).join("\n");
}

/** advCopyCardList puts the card list on the clipboard. */
async function advCopyCardList() {
  const text = advCardListText();
  if (!text) { toast("There are no cards to copy yet."); return; }
  const ok = await copyText(text);
  toast(ok ? "The card list is on the clipboard." : "Copy failed: select the text and copy it.", ok);
}

/* ---------- The power caps ---------- */

/** advRenderPower paints the power caps card: the state of the service and
 *  the cap of every card that publishes one. */
function advRenderPower(root) {
  const p = adv.power;
  if (!p) return;
  let card = root.querySelector(".card.power-card");
  if (!card) root.append(card = h("div.card.power-card"));
  const service = p.installed
    ? "installed, " + (p.enabled ? "enabled" : "not enabled") + ", " + (p.active ? "watching changes" : "not watching changes")
    : "not installed";
  const children = [
    cardHead("Power caps", "/api/power", badge(p.state || "skip")),
    h("div.muted", "Service: " + service),
  ];
  if (!p.installed) {
    children.push(h("div.err", "The power service is not installed. Install it before changing a cap; the controls stay disabled."));
    for (const command of p.install_commands || []) children.push(cmdBlock(command, "Copy the install command"));
  } else if (!p.enabled || !p.active) {
    children.push(h("div.err", "The power service is not enabled or is not watching the desired-state file."));
  }
  const rows = (p.gpus || []).map((g) => advPowerRow(g, p.installed));
  if (rows.length) {
    children.push(h("div.scroll", h("table", tableHead(["GPU", "Current", "Allowed", "Desired", "Set", "State"]),
      h("tbody", rows))));
  } else {
    children.push(h("div.empty", "No GPU publishes a hwmon power cap."));
  }
  put(card, ...children);
}

/** advPowerRow is one row of the power table: the cap of the card, the
 *  range the service allows and the control that sets it. */
function advPowerRow(g, installed) {
  const wanted = g.effective_w != null ? g.effective_w : (g.desired_w != null ? g.desired_w : g.cap_w);
  const min = g.min_w || 1;
  const max = g.max_w || Math.max(min, g.cap_w || min);
  const value = Math.max(min, Math.min(max, wanted || min));
  const range = h("input", { type: "range", min: String(min), max: String(max), step: "1", value: String(value), disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts" });
  const number = h("input", { type: "number", min: String(min), max: String(max), step: "1", value: String(value), disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts" });
  range.addEventListener("input", () => { number.value = range.value; });
  const apply = async () => {
    let watts = Number(number.value);
    if (!Number.isFinite(watts)) return toast("Enter a power cap in watts.");
    watts = Math.max(min, Math.min(max, watts));
    number.value = String(watts);
    range.value = String(watts);
    await advSavePower(g.hip_index, watts);
  };
  range.addEventListener("change", apply);
  number.addEventListener("change", apply);
  const drift = g.drift ? h("span.badge.b-warn", "drift") : h("span.badge.b-ok", "in sync");
  const desired = g.desired_w == null ? "–" : Math.round(g.desired_w) + " W";
  return h("tr", { "data-power-gpu": String(g.hip_index) },
    h("td", "GPU " + g.hip_index),
    h("td", Math.round(g.cap_w) + " W"),
    h("td", Math.round(min) + "–" + Math.round(max) + " W"),
    h("td", desired),
    h("td", h("div.row", range, number)),
    h("td", drift));
}

/** advSavePower saves the desired cap of one card. */
async function advSavePower(gpu, watts) {
  try {
    await api("/api/power", { method: "PUT", body: { gpu, watts } });
    toast("Desired power cap saved. The service will apply it.", true);
    adv.power = await api("/api/power");
    if (adv.live.gpus) adv.live.gpus.update();
  } catch (e) {
    toast("Power cap not saved: " + e.message);
  }
}
