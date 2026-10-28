"use strict";
// ---------- GPUs ----------

// gpusHead is the headline of the GPUs tab. It is kept between the renders,
// because the telemetry stream repaints the tab every second.
let gpusHead = null;

async function viewGPUs(root) {
  put(root, loading());
  try { await loadHW(); } catch { /* names and PCI info are optional; telemetry still renders */ }
  gpusHead = newGpusHead();
  root.replaceChildren(gpusHead);
  renderGPUs();
  loadPower().then(() => renderPower(root)).catch(() => { /* power status is optional */ });
}

// newGpusHead is the headline of the GPUs tab with its one primary action:
// copy the card list as plain text, one line per card.
function newGpusHead() {
  return stateHead("GPUs", h("button.primary", { type: "button", onclick: copyCardList }, "Copy the card list"));
}

// cardListText writes the cards as plain text: one line per card, so it can
// be pasted into a report or a ticket.
function cardListText() {
  const samples = (state.tele && state.tele.gpus) || [];
  const gpus = (state.hw && state.hw.gpus) || [];
  if (!samples.length && !gpus.length) return "";
  return samples.map((s) => {
    const g = gpus.find((x) => x.hip_index === s.hip_index) || {};
    return ["GPU " + s.hip_index, g.name || g.gfx || "", num(s.busy_pct, 0) + " % busy",
      num(s.temp_c, 0) + " °C", num(s.power_w, 1) + " W",
      (s.vram_used ? bytes(s.vram_used) : "–") + " / " + (s.vram_total ? bytes(s.vram_total) : "–")].join("\t");
  }).join("\n");
}

async function copyCardList() {
  const text = cardListText();
  if (!text) { toast("There are no cards to copy yet."); return; }
  const ok = await copyText(text);
  toast(ok ? "The card list is on the clipboard." : "Copy failed: select the text and copy it.", ok);
}

function renderGPUs() {
  const root = $("#tab-gpus");
  if (!gpusHead) gpusHead = newGpusHead();
  const t = state.tele;
  const gpus = (state.hw && state.hw.gpus) || [];
  const samples = new Map(((t && t.gpus) || []).map((s) => [s.hip_index, s]));
  if (!gpus.length && !samples.size) {
    setHead(gpusHead, "No AMD GPU on this machine", "bernie reads the cards from /sys/class/kfd. The amdgpu driver has to be loaded and /dev/kfd has to exist.");
    put(root, gpusHead, h("div.card", cardHead("GPUs", "/api/hw"),
      emptyState("No AMD GPU is visible to bernie.", "The amdgpu driver has to be loaded and /dev/kfd has to exist. The doctor checks every part of that.", "sudo modprobe amdgpu")));
    renderServers(root);
    renderPower(root);
    return;
  }
  let grid = root.querySelector(".grid.gpus");
  if (!grid) {
    root.replaceChildren();
    root.append(gpusHead, h("div.tiles"), h("div.servers"), grid = h("div.grid.gpus"));
  }
  setHead(gpusHead, gpuHeadline(gpus, samples), "The cards, the PCIe link behind them, and what the telemetry stream measures every second: the load, the memory, the power and the temperature.");
  renderTiles(root);
  renderServers(root);
  const ids = gpus.length ? gpus.map((g) => g.hip_index) : [...samples.keys()];
  for (const i of ids) {
    let card = grid.querySelector(`[data-gpu="${i}"]`);
    if (!card) grid.append(card = gpuCard(i));
    updateGPUCard(card, i, samples.get(i));
  }
  renderPower(root);
}

// gpuHeadline says in one sentence how many cards there are and how busy they are.
function gpuHeadline(gpus, samples) {
  if (!gpus.length) return "AMD GPU found";
  const busy = [...samples.values()].filter((s) => s.busy_pct > 5).length;
  return gpus.length + (gpus.length === 1 ? " GPU" : " GPUs") + (busy ? ", " + busy + " busy" : ", all idle");
}

// renderTiles paints the tiles of the tab: the cards in five numbers, each
// with the last minute of it.
function renderTiles(root) {
  let box = root.querySelector(".tiles");
  if (!box) return;
  const gs = (state.tele && state.tele.gpus) || [];
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
  put(box,
    gpuTile("Busy", Math.round(avg), "%", busy + " of " + n + " cards busy", "busy_avg", 100,
      "The average load of the cards, last 60 seconds"),
    gpuTile("Power", Math.round(power), "W", cap ? "of " + Math.round(cap) + " W of cap" : "no cap published",
      "power_total", undefined, "The power of all cards, last 60 seconds"),
    gpuTile("Memory", vram.toFixed(1), "GiB", "of " + vramTotal.toFixed(1) + " GiB",
      "vram_used", undefined, "The memory in use on all cards, last 60 seconds"),
    gpuTile("Hottest", hot.temp_c == null ? "–" : Math.round(hot.temp_c), "°C",
      hot.hip_index == null ? "no card reports a temperature" : "GPU " + hot.hip_index,
      "temp_max", undefined, "The hottest card, last 60 seconds"),
    gpuTile("Clock", sclk.sclk_mhz == null ? "–" : Math.round(sclk.sclk_mhz), "MHz",
      "the highest shader clock", "sclk_max", undefined, "The highest shader clock, last 60 seconds"));
}

// gpuTile is one number of the tab: the label, the value with its unit, the
// line under it and the last minute of it.
function gpuTile(label, value, unit, sub, hist, max, aria) {
  return h("article.tile",
    h("p.tile-l", label),
    h("p.tile-n", String(value), unit ? h("small", unit) : null),
    h("p.tile-s", sub),
    sparkSvg(state.hist[hist] || [], max, aria));
}

// busyRing draws the load of a card as a ring: the number in the middle and
// the track behind it.
function busyRing() {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("class", "ring");
  svg.setAttribute("viewBox", "0 0 84 84");
  svg.setAttribute("role", "img");
  const bg = document.createElementNS(ns, "circle");
  bg.setAttribute("class", "ring-bg");
  const fg = document.createElementNS(ns, "circle");
  fg.setAttribute("class", "ring-fg");
  for (const c of [bg, fg]) {
    c.setAttribute("cx", "42");
    c.setAttribute("cy", "42");
    c.setAttribute("r", "36");
  }
  const circumference = 2 * Math.PI * 36;
  fg.setAttribute("stroke-dasharray", circumference + " " + circumference);
  fg.setAttribute("stroke-dashoffset", String(circumference));
  const n = document.createElementNS(ns, "text");
  n.setAttribute("class", "ring-n");
  n.setAttribute("x", "42");
  n.setAttribute("y", "44");
  const u = document.createElementNS(ns, "text");
  u.setAttribute("class", "ring-u");
  u.setAttribute("x", "42");
  u.setAttribute("y", "56");
  u.textContent = "%";
  svg.append(bg, fg, n, u);
  return svg;
}

// setRing writes the load of a card into its ring.
function setRing(ring, pct) {
  if (!ring) return;
  const p = Math.max(0, Math.min(100, pct || 0));
  const circumference = 2 * Math.PI * 36;
  ring.querySelector(".ring-fg").setAttribute("stroke-dashoffset", String(circumference * (1 - p / 100)));
  ring.querySelector(".ring-n").textContent = String(Math.round(p));
  ring.setAttribute("aria-label", "The card is busy " + Math.round(p) + " percent");
}

function gpuCard(i) {
  const g = ((state.hw && state.hw.gpus) || []).find((x) => x.hip_index === i) || {};
  const metric = (name, bar = true, title = "", hint = "") =>
    h("div.metric", { "data-m": name }, h("div.lbl", { title: title }, h("span", name), h("b")), bar ? h("div.bar", h("i")) : null, hint ? h("div.small.muted", hint) : null);
  return h("div.card.gpu", { "data-gpu": String(i) },
    h("div.head", h("h3", "GPU " + i + " ", h("span.muted.small", g.name || "")), h("div.ring-wrap", busyRing())),
    h("div.muted.small", [g.gfx, g.pci, g.link && g.link.gen ? `PCIe ${g.link.gen}.0 x${g.link.width}` : "", g.numa_node >= 0 ? "NUMA " + g.numa_node : ""].filter(Boolean).join(" · ")),
    h("canvas.spark", { "data-s": "busy", title: "busy %, last 60 s" }),
    metric("VRAM"),
    metric("Power", true, "the power the card draws and the cap it may draw", "the cap is the power the card may draw"),
    metric("Temp", true, "edge temperature"),
    metric("Junction", true, "memory junction temperature"),
    metric("Mem", true, "memory temperature"),
    metric("sclk", false, "shader clock"),
    metric("mclk", false, "memory clock"),
    h("canvas.spark", { "data-s": "pow", title: "power W, last 60 s" }));
}

// tempCell renders a temperature with its state word first: "hot 100 °C" when
// the card is at or above the limit of the device tables, "–" when the card
// does not publish it. The bar fills up to the limit.
function tempCell(v, limit) {
  if (!v) return { text: "–", frac: 0, hot: false };
  const hot = limit > 0 && v >= limit;
  return { text: (hot ? "hot " : "") + Math.round(v) + " °C", frac: limit > 0 ? v / limit : 0, hot: hot };
}

// clockText renders a clock in MHz, "–" when the card does not publish it.
function clockText(mhz) {
  return mhz == null ? "–" : mhz + " MHz";
}

function updateGPUCard(card, i, s) {
  const set = (m, text, frac, hot) => {
    const el = card.querySelector(`[data-m="${m}"]`);
    el.querySelector("b").textContent = text;
    const bar = el.querySelector(".bar > i");
    if (!bar) return;
    bar.style.width = Math.max(0, Math.min(100, frac * 100)) + "%";
    el.querySelector(".bar").classList.toggle("hot", !!hot);
  };
  if (!s) return;
  const g = ((state.hw && state.hw.gpus) || []).find((x) => x.hip_index === i) || {};
  const limits = g.temp_limits_c || {};
  setRing(card.querySelector(".ring"), s.busy_pct);
  const vfrac = s.vram_total ? s.vram_used / s.vram_total : 0;
  set("VRAM", `${(s.vram_used / GiB).toFixed(1)} / ${(s.vram_total / GiB).toFixed(1)} GiB`, vfrac, vfrac > 0.95);
  const cap = s.power_cap_w || 0;
  set("Power", `${Math.round(s.power_w)} W` + (cap ? ` / ${Math.round(cap)} W cap` : ""), cap ? s.power_w / cap : 0, cap && s.power_w > cap * 0.98);
  const edge = tempCell(s.temp_c, limits.edge);
  set("Temp", edge.text, edge.frac, edge.hot);
  const junction = tempCell(s.temp_junction_c, limits.junction);
  set("Junction", junction.text, junction.frac, junction.hot);
  const mem = tempCell(s.temp_mem_c, limits.mem);
  set("Mem", mem.text, mem.frac, mem.hot);
  set("sclk", clockText(s.sclk_mhz), 0, false);
  set("mclk", clockText(s.mclk_mhz), 0, false);
  spark(card.querySelector('[data-s="busy"]'), state.hist["busy" + i] || [], 100);
  spark(card.querySelector('[data-s="pow"]'), state.hist["pow" + i] || [], cap || undefined);
}

function renderServers(root) {
  const t = state.tele;
  let box = root.querySelector(".servers");
  if (!box) root.prepend(box = h("div.servers"));
  const runs = (t && t.runs) || [];
  if (!runs.length) { box.replaceChildren(); return; }
  let card = box.querySelector(".card");
  if (!card) put(box, card = h("div.card", cardHead("vLLM servers", "/api/telemetry"), h("div.scroll", h("table", h("thead", h("tr",
    ["Run", "State", "Running", "Waiting", "KV cache", "Gen tok/s", "Prompt tok/s", "Spec accept", "Gen tok/s, 60 s"].map((c, j) => h("th", { class: j > 1 && j < 8 ? "num" : null }, c)))), h("tbody")))), h("div.gap"));
  const tb = card.querySelector("tbody");
  const seen = new Set();
  for (const r of runs) {
    seen.add(r.name);
    let tr = tb.querySelector(`tr[data-run="${CSS.escape(r.name)}"]`);
    if (!tr) {
      tr = h("tr", { "data-run": r.name }, h("td", r.name), h("td"), ...Array.from({ length: 6 }, () => h("td.num")), h("td", h("canvas.spark")));
      tb.append(tr);
    }
    const st = r.stats || {};
    const c = tr.children;
    put(c[1], r.error ? h("span.badge.b-fail", { title: r.error }, "metrics error") : badge(r.state));
    c[2].textContent = num(st.requests_running, 0);
    c[3].textContent = num(st.requests_waiting, 0);
    c[4].textContent = st.kv_cache_usage != null ? (st.kv_cache_usage * 100).toFixed(1) + "%" : "–";
    c[5].textContent = num(r.gen_tok_s);
    c[6].textContent = num(r.prompt_tok_s);
    c[7].textContent = st.spec_drafts_total ? (st.spec_accept_rate * 100).toFixed(0) + "% · " + num(st.spec_mean_accept_len, 2) : "–";
    spark(c[8].firstChild, state.hist["gen:" + r.name] || []);
  }
  for (const tr of [...tb.children]) if (!seen.has(tr.dataset.run)) tr.remove();
}
