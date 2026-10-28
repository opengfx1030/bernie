"use strict";
// ---------- GPUs ----------

async function viewGPUs(root) {
  put(root, loading());
  try { await loadHW(); } catch { /* names and PCI info are optional; telemetry still renders */ }
  root.replaceChildren();
  renderGPUs();
  loadPower().then(() => renderPower(root)).catch(() => { /* power status is optional */ });
}

function renderGPUs() {
  const root = $("#tab-gpus");
  const t = state.tele;
  const gpus = (state.hw && state.hw.gpus) || [];
  if (!gpus.length && !(t && t.gpus && t.gpus.length)) {
    put(root, h("div.card", cardHead("GPUs", "/api/hw"), h("div.empty", "No AMD GPUs found on this host (KFD topology is empty).")));
    renderServers(root);
    renderPower(root);
    return;
  }
  let grid = root.querySelector(".grid.gpus");
  if (!grid) {
    root.replaceChildren();
    root.append(h("div.servers"), grid = h("div.grid.gpus"));
  }
  renderServers(root);
  const samples = new Map(((t && t.gpus) || []).map((s) => [s.hip_index, s]));
  const ids = gpus.length ? gpus.map((g) => g.hip_index) : [...samples.keys()];
  for (const i of ids) {
    let card = grid.querySelector(`[data-gpu="${i}"]`);
    if (!card) grid.append(card = gpuCard(i));
    updateGPUCard(card, i, samples.get(i));
  }
  renderPower(root);
}

function gpuCard(i) {
  const g = (state.hw.gpus || []).find((x) => x.hip_index === i) || {};
  const metric = (name, bar = true, title = "") =>
    h("div.metric", { "data-m": name }, h("div.lbl", { title: title }, h("span", name), h("b")), bar ? h("div.bar", h("i")) : null);
  return h("div.card.gpu", { "data-gpu": String(i) },
    h("div.head", h("h3", "GPU " + i + " ", h("span.muted.small", g.name || "")), h("span.big", { "data-busy": "" }, "–")),
    h("div.muted.small", [g.gfx, g.pci, g.link && g.link.gen ? `PCIe ${g.link.gen}.0 x${g.link.width}` : "", g.numa_node >= 0 ? "NUMA " + g.numa_node : ""].filter(Boolean).join(" · ")),
    h("canvas.spark", { "data-s": "busy", title: "busy %, last 60 s" }),
    metric("VRAM"), metric("Power"),
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
  const g = (state.hw.gpus || []).find((x) => x.hip_index === i) || {};
  const limits = g.temp_limits_c || {};
  card.querySelector("[data-busy]").textContent = Math.round(s.busy_pct) + "%";
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
