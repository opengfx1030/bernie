// rdnactl dashboard. Vanilla JS, no build step, no dependencies.
// Every view calls the JSON API of spec §11; the API calls the same Go code as the CLI.
"use strict";

// ---------- helpers ----------

/** h("div.card", {onclick}, child, ...) builds DOM without innerHTML (no XSS). */
function h(spec, attrs, ...kids) {
  const [tag, ...cls] = spec.split(".");
  const el = document.createElement(tag || "div");
  if (cls.length) el.className = cls.join(" ");
  if (attrs != null && (typeof attrs !== "object" || attrs instanceof Node || Array.isArray(attrs))) {
    kids.unshift(attrs);
    attrs = null;
  }
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "class") el.className += (el.className ? " " : "") + v;
    else if (k in el && k !== "list" && typeof v !== "string") el[k] = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  add(el, kids);
  return el;
}
function add(el, kids) {
  for (const k of kids.flat(Infinity)) {
    if (k == null || k === false) continue;
    el.append(k instanceof Node ? k : String(k));
  }
}
function put(el, ...kids) { el.replaceChildren(); add(el, kids); return el; }
const $ = (sel) => document.querySelector(sel);

const GiB = 1024 ** 3;
function bytes(n) {
  if (!n) return "0";
  const u = ["B", "KiB", "MiB", "GiB", "TiB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (n >= 100 || i === 0 ? n.toFixed(0) : n.toFixed(1)) + " " + u[i];
}
function num(v, d = 1) {
  if (v == null || v === "") return "–";
  if (typeof v !== "number") return String(v);
  if (Number.isInteger(v)) return String(v);
  return v.toFixed(d);
}
function ago(t) {
  const s = (Date.now() - new Date(t).getTime()) / 1000;
  if (!isFinite(s)) return "";
  if (s < 60) return Math.round(s) + " s ago";
  if (s < 3600) return Math.round(s / 60) + " min ago";
  if (s < 86400) return Math.round(s / 3600) + " h ago";
  return Math.round(s / 86400) + " d ago";
}
function badge(status) { return h("span.badge", { class: "b-" + status }, status || "?"); }
function sh(a) { return /^[\w@%+=:,./-]+$/.test(a) ? a : "'" + a.replace(/'/g, "'\\''") + "'"; }

let toastTimer;
function toast(msg, ok) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = ok ? "ok" : "";
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), ok ? 3000 : 8000);
}

/** api() fetches JSON; mutating methods carry the X-Rdnactl header (CSRF guard). */
async function api(path, opts = {}) {
  const init = { method: opts.method || "GET", headers: {}, credentials: "same-origin" };
  if (init.method !== "GET") init.headers["X-Rdnactl"] = "1";
  if (opts.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(opts.body);
  }
  const r = await fetch(path, init);
  const text = await r.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  if (!r.ok) throw new Error((data && data.error) || r.status + " " + r.statusText);
  return data;
}

function loading() { return h("div.empty", h("span.spin"), " loading…"); }
function failed(e) { return h("div.err", String(e.message || e)); }

/** Small canvas sparkline, no chart library. */
function spark(canvas, values, max) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth, hgt = canvas.clientHeight;
  if (!w || !hgt) return;
  if (canvas.width !== w * dpr || canvas.height !== hgt * dpr) {
    canvas.width = w * dpr;
    canvas.height = hgt * dpr;
  }
  const c = canvas.getContext("2d");
  c.setTransform(dpr, 0, 0, dpr, 0, 0);
  c.clearRect(0, 0, w, hgt);
  if (values.length < 2) return;
  const top = max || Math.max(1, ...values);
  const step = w / (HISTORY - 1);
  const x0 = w - (values.length - 1) * step;
  const color = getComputedStyle(document.documentElement).getPropertyValue("--spark").trim() || "#c8102e";
  c.beginPath();
  values.forEach((v, i) => {
    const y = hgt - 1 - (Math.min(v, top) / top) * (hgt - 2);
    i ? c.lineTo(x0 + i * step, y) : c.moveTo(x0, y);
  });
  c.strokeStyle = color;
  c.lineWidth = 1.5;
  c.stroke();
  c.lineTo(w, hgt);
  c.lineTo(x0, hgt);
  c.closePath();
  c.globalAlpha = 0.15;
  c.fillStyle = color;
  c.fill();
  c.globalAlpha = 1;
}

// ---------- shared state ----------

const HISTORY = 60; // seconds of sparkline history
const state = {
  hw: null,          // /api/hw
  tele: null,        // last telemetry event
  hist: {},          // key -> number[] (last HISTORY samples)
  tab: "overview",
};
function pushHist(key, v) {
  const a = state.hist[key] || (state.hist[key] = []);
  a.push(v || 0);
  if (a.length > HISTORY) a.shift();
  return a;
}
async function loadHW(force) {
  if (!state.hw || force) {
    state.hw = await api("/api/hw");
    $("#hostname").textContent = state.hw.host && state.hw.host.hostname || "";
  }
  return state.hw;
}
function gpuName(i) {
  const g = state.hw && (state.hw.gpus || []).find((g) => g.hip_index === i);
  return g ? g.name || g.gfx : "";
}
function runsOf(data) { return Array.isArray(data) ? data : (data && data.runs) || []; }
function runURL(r) { return "http://" + (r.host || "127.0.0.1") + ":" + r.port; }

// ---------- telemetry stream (shared by tabs) ----------

let es = null;
function startTelemetry() {
  if (es) return;
  es = new EventSource("/api/telemetry");
  es.onopen = () => $("#live").classList.add("on");
  es.onerror = () => $("#live").classList.remove("on");
  es.onmessage = (ev) => {
    const t = JSON.parse(ev.data);
    state.tele = t;
    for (const s of t.gpus || []) {
      pushHist("busy" + s.hip_index, s.busy_pct);
      pushHist("pow" + s.hip_index, s.power_w);
    }
    for (const r of t.runs || []) pushHist("gen:" + r.name, r.gen_tok_s);
    if (state.tab === "gpus") renderGPUs();
  };
}

// ---------- Overview ----------

async function viewOverview(root) {
  const hostCard = h("div.card", h("h2", "Host"), loading());
  const docCard = h("div.card", h("h2", "Doctor"), loading());
  const runCard = h("div.card", h("h2", "Runs"), loading());
  put(root, h("div.cols", h("div", hostCard, h("div.gap"), runCard), docCard));

  loadHW(true).then((hw) => put(hostCard, h("h2", "Host"), hostSummary(hw))).catch((e) => put(hostCard, h("h2", "Host"), failed(e)));
  api("/api/runs").then((d) => put(runCard, h("h2", "Runs", h("span.grow"), h("a.small", { href: "#runs" }, "manage")), runsTable(runsOf(d), true)))
    .catch((e) => put(runCard, h("h2", "Runs"), failed(e)));
  loadDoctor(docCard);
}

function hostSummary(hw) {
  const ho = hw.host || {};
  const cpu = ho.cpu || {};
  const gpus = hw.gpus || [];
  const byName = {};
  for (const g of gpus) {
    const k = (g.name || "GPU") + " (" + g.gfx + ")";
    byName[k] = (byName[k] || 0) + 1;
  }
  const engines = (ho.engines || []).map((e) => e.name + " " + (e.version || "") + (e.usable ? "" : " (not usable)") + (e.rootless ? ", rootless" : ""));
  return h("dl.kv",
    h("dt", "Host"), h("dd", ho.hostname || "–"),
    h("dt", "OS"), h("dd", (ho.os && (ho.os.pretty || ho.os.id)) || "–", ho.pkg_manager ? h("span.muted", " · " + ho.pkg_manager) : null),
    h("dt", "Kernel"), h("dd", ho.kernel || "–"),
    h("dt", "CPU"), h("dd", cpu.model || "–", cpu.threads ? h("span.muted", ` · ${cpu.sockets || 1}× socket, ${cpu.cores} cores / ${cpu.threads} threads`) : null),
    h("dt", "RAM"), h("dd", bytes(ho.ram_bytes)),
    h("dt", "GPUs"), h("dd", gpus.length ? Object.entries(byName).map(([k, n]) => h("div", n + "× " + k)) : h("span.muted", "none found")),
    h("dt", "Engines"), h("dd", engines.length ? engines.join("; ") : h("span.muted", "none")),
    ho.selinux ? [h("dt", "SELinux"), h("dd", ho.selinux)] : null,
  );
}

async function loadDoctor(card) {
  const title = () => h("h2", "Doctor", h("span.grow"), h("button.small", { onclick: () => { put(card, title(), loading()); loadDoctor(card); } }, "Re-run"));
  put(card, title(), loading());
  try {
    const rep = await api("/api/doctor");
    const sum = rep.summary || {};
    const checks = rep.checks || [];
    const bad = checks.filter((c) => c.status === "fail" || c.status === "warn")
      .sort((a, b) => (a.status === "fail" ? 0 : 1) - (b.status === "fail" ? 0 : 1));
    put(card, title(),
      h("div.counts", ["fail", "warn", "ok", "skip"].map((s) => h("span.badge", { class: "b-" + s }, (sum[s] || 0) + " " + s))),
      bad.length ? bad.map(checkRow) : h("div.empty", "No failing or warning checks."),
      h("details", h("summary.muted.small", "All checks (" + checks.length + ")"), checks.map(checkRow)));
  } catch (e) {
    put(card, title(), failed(e));
  }
}

function checkRow(c) {
  return h("div.check",
    h("div.t", badge(c.status), h("b", c.title), h("span.id", c.id)),
    c.detail ? h("div.muted.small", c.detail) : null,
    c.fix ? h("pre", c.fix) : null,
    c.doc ? h("a.small", { href: c.doc, target: "_blank", rel: "noopener noreferrer" }, "docs") : null);
}

// ---------- GPUs ----------

async function viewGPUs(root) {
  put(root, loading());
  try { await loadHW(); } catch { /* names and PCI info are optional; telemetry still renders */ }
  root.replaceChildren();
  renderGPUs();
}

function renderGPUs() {
  const root = $("#tab-gpus");
  const t = state.tele;
  const gpus = (state.hw && state.hw.gpus) || [];
  if (!gpus.length && !(t && t.gpus && t.gpus.length)) {
    put(root, h("div.card", h("h2", "GPUs"), h("div.empty", "No AMD GPUs found on this host (KFD topology is empty).")));
    renderServers(root);
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
}

function gpuCard(i) {
  const g = (state.hw.gpus || []).find((x) => x.hip_index === i) || {};
  const metric = (name) => h("div.metric", { "data-m": name }, h("div.lbl", h("span", name), h("b")), h("div.bar", h("i")));
  return h("div.card.gpu", { "data-gpu": String(i) },
    h("div.head", h("h3", "GPU " + i + " ", h("span.muted.small", g.name || "")), h("span.big", { "data-busy": "" }, "–")),
    h("div.muted.small", [g.gfx, g.pci, g.link && g.link.gen ? `PCIe ${g.link.gen}.0 x${g.link.width}` : "", g.numa_node >= 0 ? "NUMA " + g.numa_node : ""].filter(Boolean).join(" · ")),
    h("canvas.spark", { "data-s": "busy", title: "busy %, last 60 s" }),
    metric("VRAM"), metric("Power"), metric("Temp"),
    h("canvas.spark", { "data-s": "pow", title: "power W, last 60 s" }));
}

function updateGPUCard(card, i, s) {
  const set = (m, text, frac, hot) => {
    const el = card.querySelector(`[data-m="${m}"]`);
    el.querySelector("b").textContent = text;
    el.querySelector(".bar > i").style.width = Math.max(0, Math.min(100, frac * 100)) + "%";
    el.querySelector(".bar").classList.toggle("hot", !!hot);
  };
  if (!s) return;
  card.querySelector("[data-busy]").textContent = Math.round(s.busy_pct) + "%";
  const vfrac = s.vram_total ? s.vram_used / s.vram_total : 0;
  set("VRAM", `${(s.vram_used / GiB).toFixed(1)} / ${(s.vram_total / GiB).toFixed(1)} GiB`, vfrac, vfrac > 0.95);
  const cap = s.power_cap_w || 0;
  set("Power", `${Math.round(s.power_w)} W` + (cap ? ` / ${Math.round(cap)} W cap` : ""), cap ? s.power_w / cap : 0, cap && s.power_w > cap * 0.98);
  set("Temp", s.temp_c ? Math.round(s.temp_c) + " °C" : "–", s.temp_c / 110, s.temp_c >= 95);
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
  if (!card) put(box, card = h("div.card", h("h2", "vLLM servers"), h("div.scroll", h("table", h("thead", h("tr",
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
        topo.bandwidth ? null : h("div.small.muted", "Run `rdnactl topo --bandwidth` to measure GB/s per pair."))));
  } catch (e) {
    put(root, failed(e));
  }
}

// ---------- Recipes ----------

const recipeUI = { list: null, sel: null };

async function viewRecipes(root) {
  put(root, loading());
  try {
    [recipeUI.list] = await Promise.all([api("/api/recipes"), loadHW().catch(() => null)]);
  } catch (e) { put(root, failed(e)); return; }
  const recipes = recipeUI.list.recipes || [];
  const errors = recipeUI.list.errors || [];
  const detail = h("div.card.detail", h("h2", "Plan"), h("div.empty", "Select a recipe."));
  const rows = recipes.map((r) => {
    const tr = h("tr.click", { onclick: () => { recipeUI.sel = r.id; tbody.querySelectorAll("tr").forEach((x) => x.classList.toggle("sel", x === tr)); showRecipe(detail, r); } },
      h("td", h("b", r.id), h("div.small.muted", r.name)),
      h("td", badge(r.status)),
      h("td.num", r.gpus || "–"),
      h("td.small", (r.arch || []).join(", ")),
      h("td", h("span.badge", { class: "b-" + (r.fit && r.fit.status), title: (r.fit && r.fit.reasons || []).join("\n") }, (r.fit && r.fit.status) || "?"),
        r.fit && r.fit.reasons && r.fit.reasons.length ? h("div.small.muted", r.fit.reasons.join("; ")) : null));
    return tr;
  });
  const tbody = h("tbody", rows);
  put(root, h("div.cols",
    h("div.card", h("h2", "Recipes", h("span.grow"), h("span.small.muted", recipes.length + " loaded")),
      recipes.length ? h("div.scroll", h("table", h("thead", h("tr", h("th", "Recipe"), h("th", "Status"), h("th.num", "GPUs"), h("th", "Arch"), h("th", "Fit"))), tbody)) : h("div.empty", "No recipes."),
      errors.length ? h("div", h("h3", "Load errors"), errors.map((e) => h("div.err.small", (e.source || "") + ": " + (e.err || e.error || "")))) : null),
    detail));
  const pre = recipes.findIndex((r) => r.id === recipeUI.sel);
  if (pre >= 0) rows[pre].click();
}

function showRecipe(card, r) {
  const inputs = r.inputs || {};
  const inputEls = {};
  const fields = Object.entries(inputs).map(([k, inp]) => {
    inputEls[k] = h("input", { name: k, value: inp.default || "", placeholder: inp.path ? "/absolute/path" : "", required: !!inp.required });
    return [h("label", { for: "in-" + k }, k + (inp.required ? " *" : "")), Object.assign(inputEls[k], { id: "in-" + k }),
      inp.description ? h("div.hint", inp.description) : null];
  });
  // Container mode needs a usable Docker/Podman engine; without one only host mode is offered.
  const engineOK = !state.hw || ((state.hw.host && state.hw.host.engines) || []).some((e) => e.usable);
  const modes = (r.modes || ["host", "container"]).filter((m) => m !== "container" || engineOK);
  const mode = h("select", { name: "mode" }, (modes.length > 1 ? ["", ...modes] : modes).map((m) => h("option", { value: m }, m || "auto")));
  const name = h("input", { name: "name", placeholder: r.id });
  const sets = h("textarea", { rows: 3, placeholder: "args.max-model-len=32768\nenv.VLLM_RDNA_AR=0" });
  const out = h("div");
  const collect = () => {
    const s = [];
    for (const [k, el] of Object.entries(inputEls)) if (el.value !== "") s.push(k + "=" + el.value);
    for (const line of sets.value.split("\n")) if (line.trim()) s.push(line.trim());
    return s;
  };
  const preview = async () => {
    put(out, loading());
    const q = new URLSearchParams();
    for (const s of collect()) q.append("set", s);
    if (mode.value) q.set("mode", mode.value);
    try {
      put(out, planView(await api(`/api/recipes/${encodeURIComponent(r.id)}/plan?` + q)));
    } catch (e) { put(out, failed(e)); }
  };
  const start = async (ev) => {
    ev.target.disabled = true;
    try {
      const rec = await api("/api/runs", { method: "POST", body: { recipe: r.id, set: collect(), mode: mode.value, name: name.value } });
      toast(`Started run ${rec.name} on port ${rec.port}.`, true);
      location.hash = "#runs";
    } catch (e) { toast("Start failed: " + e.message); }
    ev.target.disabled = false;
  };
  put(card, h("h2", "Plan: " + r.id),
    h("div.small.muted", r.model || ""),
    h("form.f", { onsubmit: (e) => { e.preventDefault(); preview(); } },
      fields,
      h("label", "mode"), mode,
      h("label", "run name"), name,
      h("label.full", "Overrides (--set KEY=VALUE, one per line)"), h("div.full", sets),
      h("div.full.row", h("button", { type: "submit" }, "Preview plan"), h("button.primary", { type: "button", onclick: start }, "Start"))),
    out);
  preview();
}

function planView(plan) {
  const ex = plan.exec || {};
  const argv = ex.argv && ex.argv.length ? ex.argv : ["vllm", "serve", ...(plan.args || [])];
  const envRows = ex.env && ex.env.length
    ? ex.env.map((kv) => { const i = kv.indexOf("="); return [kv.slice(0, i), kv.slice(i + 1)]; })
    : Object.entries(plan.env || {});
  const rows = envRows.sort((a, b) => a[0].localeCompare(b[0])).map(([k, v]) => h("tr", h("td.mono", k), h("td.mono", v)));
  const kv = [["Mode", plan.mode], ["Model", plan.model], ["Served name", plan.served_name], ["Address", plan.host && plan.port ? plan.host + ":" + plan.port : plan.port],
    ["GPUs", plan.hip_devices], ["Selection", plan.gpu_select_reason], ["Image", plan.image], ["shm", plan.shm_size], ["TunableOp", plan.tunableop_profile],
    ["Path inputs", (plan.path_inputs || []).join(", ")], ["Directory", ex.dir], ["Unset", (ex.unset || []).join(", ")]].filter(([, v]) => v != null && v !== "");
  return h("div",
    (plan.warnings || []).map((w) => h("div.err.small", "⚠ " + w)),
    h("h3", "Command"),
    h("pre", argv.map(sh).join(" ")),
    h("dl.kv", kv.map(([k, v]) => [h("dt", k), h("dd", String(v))])),
    rows.length ? [h("h3", ex.env && ex.env.length ? "Environment (added)" : "Environment"), h("div.scroll", h("table", h("tbody", rows)))] : null);
}

// ---------- Runs ----------

let logES = null;
function stopLogs() { if (logES) { logES.close(); logES = null; } }

async function viewRuns(root) {
  const list = h("div.card", h("h2", "Runs", h("span.grow"), h("button.small", { onclick: () => viewRuns(root) }, "Refresh")), loading());
  const logs = h("div.card", h("h2", "Logs"), h("div.empty", "Select a run to follow its log."));
  put(root, list, h("div.gap"), logs);
  try {
    const runs = runsOf(await api("/api/runs"));
    list.lastChild.replaceWith(runsTable(runs, false, logs, root));
  } catch (e) { list.lastChild.replaceWith(failed(e)); }
}

function runsTable(runs, compact, logCard, root) {
  if (!runs.length) return h("div.empty", "No runs. Start one from the Recipes tab or with `rdnactl serve <recipe>`.");
  const tele = new Map(((state.tele && state.tele.runs) || []).map((r) => [r.name, r]));
  return h("div.scroll", h("table",
    h("thead", h("tr", h("th", "Name"), h("th", "Recipe"), h("th", "State"), compact ? null : h("th", "Mode"), h("th", "Address"), compact ? null : h("th", "Started"), compact ? null : h("th", ""))),
    h("tbody", runs.map((r) => {
      const t = r.state !== "exited" ? tele.get(r.name) : null;
      return h("tr",
        h("td", h("b", r.name)),
        h("td", r.recipe),
        h("td", badge(r.state), t && t.stats ? h("div.small.muted", `${num(t.stats.requests_running, 0)} running · ${num(t.gen_tok_s)} tok/s`) : null),
        compact ? null : h("td", r.mode + (r.engine ? " (" + r.engine + ")" : "")),
        h("td.mono", r.port ? h("a", { href: runURL(r) + "/v1/models", target: "_blank", rel: "noopener noreferrer" }, (r.host || "127.0.0.1") + ":" + r.port) : "–"),
        compact ? null : h("td.small", r.started_at ? ago(r.started_at) : ""),
        compact ? null : h("td", h("div.row",
          h("button.small", { onclick: () => followLogs(logCard, r.name) }, "Logs"),
          r.state !== "exited" ? h("button.small.danger", { onclick: (ev) => stopRun(ev, r.name, root) }, "Stop") : null)));
    }))));
}

async function stopRun(ev, name, root) {
  if (!confirm(`Stop run ${name}?`)) return;
  ev.target.disabled = true;
  try {
    await api("/api/runs/" + encodeURIComponent(name), { method: "DELETE" });
    toast(`Stopped ${name}.`, true);
  } catch (e) { toast("Stop failed: " + e.message); }
  viewRuns(root);
}

function followLogs(card, name) {
  stopLogs();
  const box = h("div.log");
  const status = h("span.small.muted", "connecting…");
  let follow = true;
  box.addEventListener("scroll", () => { follow = box.scrollTop + box.clientHeight >= box.scrollHeight - 4; });
  put(card, h("h2", "Logs: " + name, h("span.grow"), status, h("button.small", { onclick: () => { stopLogs(); status.textContent = "stopped"; } }, "Close")), box);
  logES = new EventSource(`/api/runs/${encodeURIComponent(name)}/logs`);
  logES.onopen = () => (status.textContent = "following");
  logES.onerror = () => (status.textContent = "disconnected");
  logES.addEventListener("end", () => { status.textContent = "log ended"; stopLogs(); });
  logES.addEventListener("error", (ev) => { if (ev.data) { box.append(h("div.err", ev.data)); } });
  let pending = [];
  let raf = 0;
  logES.onmessage = (ev) => {
    pending.push(ev.data);
    if (raf) return;
    raf = requestAnimationFrame(() => {
      raf = 0;
      box.append(pending.join("\n") + "\n");
      pending = [];
      while (box.childNodes.length > 4000) box.firstChild.remove();
      if (follow) box.scrollTop = box.scrollHeight;
    });
  };
}

// ---------- Bench ----------

const CELL_COLS = [
  ["concurrency", "Conc."], ["completed", "OK"], ["failed", "Fail"],
  ["ttft_p50_ms", "TTFT p50 ms"], ["ttft_p95_ms", "TTFT p95 ms"], ["itl_p50_ms", "ITL p50 ms"], ["itl_p95_ms", "ITL p95 ms"],
  ["decode_tok_s", "Decode tok/s"], ["prefill_tok_s", "Prefill tok/s"], ["output_tok_s", "Output tok/s"], ["duration_s", "Duration s"],
  ["num_prompts", "Prompts"], ["mean_prompt_tokens", "In tok"], ["mean_output_tokens", "Out tok"], ["tokens_per_chunk", "Tok/chunk"],
];

function cellsTable(cells) {
  if (!cells || !cells.length) return h("div.empty", "No cells.");
  const known = new Set(CELL_COLS.map((c) => c[0]));
  const extra = Object.keys(cells[0]).filter((k) => !known.has(k) && typeof cells[0][k] !== "object");
  const cols = CELL_COLS.filter(([k]) => k in cells[0]).concat(extra.map((k) => [k, k]));
  const fmt = (k, v) => typeof v === "number" && /^itl/.test(k) ? v.toFixed(2) : num(v);
  return h("div.scroll", h("table",
    h("thead", h("tr", cols.map(([, l]) => h("th.num", l)))),
    h("tbody", cells.map((c) => h("tr", cols.map(([k]) => h("td.num", fmt(k, c[k]))))))));
}

async function viewBench(root) {
  const f = (name, attrs) => h("input", Object.assign({ name, id: "b-" + name }, attrs));
  const form = h("form.f",
    h("label", { for: "b-url" }, "URL"), f("url", { placeholder: "the only running run, else http://127.0.0.1:8000" }),
    h("label", { for: "b-model" }, "Model"), f("model", { placeholder: "first id of /v1/models" }),
    h("label", { for: "b-input_len" }, "Input tokens"), f("input_len", { type: "number", min: 1, placeholder: "1024" }),
    h("label", { for: "b-output_len" }, "Output tokens"), f("output_len", { type: "number", min: 1, placeholder: "512" }),
    h("label", { for: "b-concurrency" }, "Concurrency"), f("concurrency", { placeholder: "1,8" }),
    h("label", { for: "b-num_prompts" }, "Prompts per cell"), f("num_prompts", { type: "number", min: 1, placeholder: "max(4, 2 × concurrency)" }),
    h("label", { for: "b-warmup" }, "Warmup"), f("warmup", { type: "number", min: 0, placeholder: "1" }),
    h("label", { for: "b-seed" }, "Seed"), f("seed", { type: "number", placeholder: "0" }),
    h("label", { for: "b-tag" }, "Tag"), f("tag", { placeholder: "free label" }),
    h("div.full.row", h("button.primary", { type: "submit" }, "Run benchmark"), h("span.small.muted.status")));
  const result = h("div.card", h("h2", "Result"), h("div.empty", "Run a benchmark or pick one from the history."));
  const hist = h("div.card", h("h2", "History"), loading());
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const body = {};
    for (const el of form.querySelectorAll("input")) {
      if (el.value === "") continue;
      body[el.name] = el.type === "number" ? Number(el.value) : el.value;
    }
    if (body.concurrency) body.concurrency = String(body.concurrency).split(",").map((s) => Number(s.trim())).filter((n) => n > 0);
    const btn = form.querySelector("button");
    const st = form.querySelector(".status");
    btn.disabled = true;
    put(st, h("span.spin"), " running… (this takes minutes)");
    try {
      const r = await api("/api/bench", { method: "POST", body });
      put(result, h("h3", "Result " + r.id), h("div.small.muted", `${r.model} @ ${r.url}`), cellsTable(r.cells));
      st.textContent = "done";
      loadHistory(hist, result);
    } catch (err) { st.textContent = ""; put(result, failed(err)); }
    btn.disabled = false;
  });
  put(root, h("div.cols", h("div.card", h("h2", "Run benchmark"), form), hist), h("div.gap"), result);
  loadHistory(hist, result);
}

async function loadHistory(card, result) {
  try {
    const d = await api("/api/bench");
    const list = (Array.isArray(d) ? d : (d && d.results) || []).slice().sort((a, b) => new Date(b.time) - new Date(a.time));
    if (!list.length) { put(card, h("h2", "History"), h("div.empty", "No saved results.")); return; }
    put(card, h("h2", "History"), h("div.scroll", h("table",
      h("thead", h("tr", h("th", "When"), h("th", "Tag"), h("th", "Model"), h("th.num", "Conc."), h("th.num", "Best output tok/s"))),
      h("tbody", list.map((r) => h("tr.click", {
        onclick: () => put(result, h("h3", "Result " + r.id), h("div.small.muted", `${r.model} @ ${r.url} · ${new Date(r.time).toLocaleString()}`), cellsTable(r.cells)),
      },
        h("td.small", ago(r.time)), h("td", r.tag || ""), h("td", r.model),
        h("td.num", (r.cells || []).map((c) => c.concurrency).join(",")),
        h("td.num", num(Math.max(0, ...(r.cells || []).map((c) => c.output_tok_s || 0))))))))));
  } catch (e) { put(card, h("h2", "History"), failed(e)); }
}

// ---------- router ----------

const views = { overview: viewOverview, gpus: viewGPUs, topology: viewTopology, recipes: viewRecipes, runs: viewRuns, bench: viewBench };

function route() {
  const tab = (location.hash || "#overview").slice(1);
  state.tab = views[tab] ? tab : "overview";
  if (state.tab !== "runs") stopLogs();
  for (const a of document.querySelectorAll("#tabs a")) a.setAttribute("aria-selected", String(a.dataset.tab === state.tab));
  for (const s of document.querySelectorAll("main > .tab")) s.hidden = s.id !== "tab-" + state.tab;
  views[state.tab]($("#tab-" + state.tab));
}

window.addEventListener("hashchange", route);
window.addEventListener("resize", () => state.tab === "gpus" && renderGPUs());
startTelemetry();
loadHW().catch(() => {}); // header hostname, GPU names
route();
