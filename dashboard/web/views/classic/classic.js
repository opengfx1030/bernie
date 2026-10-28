"use strict";
// ---------- Classic ----------
// The Classic tab is the original dashboard design of the reference web UI,
// recovered as it is: the same hero, the same six tiles, the same two charts,
// the same GPU cards with their rings and meters, the same host panel and the
// same connect panel. It is here so that the owner of bernie and the author of
// the reference can put the two side by side and pick one.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
//
// The data is the data of this host: the shared telemetry stream of
// js/data.js (the stream the Dashboard tab reads too), GET /api/hw,
// GET /api/runs and GET /api/gpus/history. This tab opens no timer of its
// own. A number this host cannot measure shows a dash and says so: nothing
// here is invented.

const classic = {
  built: false,
  panel: null,
  stream: null,
  tele: null,      // the last telemetry document
  hist: [],        // [{t, gen, prompt, running, waiting, kv, spec}] the last minutes
  snip: "curl",    // the connect snippet: curl or python
  charts: new Map(),
  counts: {},      // the last value of every counted number
  el: {},          // the parts the stream updates
  tip: null,       // the tooltip of the charts and of the marks
};

const CLASSIC_DOCS = "https://github.com/opengfx1030/bernie/blob/main/docs/design-dashboard.md";
const CLASSIC_HIST = 240;   // the points of a chart: four minutes at one per second
const CLASSIC_TAIL = 60;    // the samples under a tile

/* ---------- the view ---------- */

/** viewClassic opens the tab. The router calls it with the panel of the tab. */
function viewClassic(panel) {
  classic.panel = panel;
  panel.classList.add("classic");
  if (!classic.built) {
    classicBuild();
    classic.built = true;
  }
  classicUpdate();
  classicStream();
  classicHistory();
}

/** classicStream follows the shared telemetry stream of js/data.js. */
function classicStream() {
  if (classic.stream) return classic.stream;
  if (typeof openTelemetry !== "function") return null;
  classic.stream = openTelemetry({
    message: (doc) => {
      if (!doc) return;
      classic.tele = doc;
      classicSample(doc);
      if (typeof state !== "undefined" && state.tab === "classic") classicUpdate();
    },
  });
  return classic.stream;
}

/** classicHistory reads the five minutes of the server's output rate. The
 *  ring buffer of the server holds it; the live samples then keep the chart
 *  moving. */
async function classicHistory() {
  try {
    const doc = await api("/api/gpus/history?window=5m");
    const series = (doc && doc.series) || [];
    if (!series.length) return;
    const vals = series[0].tokens_per_s || [];
    const step = (doc.interval_s || 2) * 1000;
    const now = Date.now();
    const seeded = vals.map((v, i) => ({
      t: now - (vals.length - 1 - i) * step, gen: v == null ? null : v, prompt: null,
    }));
    classic.hist = classic.hist.length ? classic.hist.concat(seeded.filter((p) => p.t < classic.hist[0].t)) : seeded;
    if (classic.hist.length > CLASSIC_HIST) classic.hist = classic.hist.slice(-CLASSIC_HIST);
    if (typeof state !== "undefined" && state.tab === "classic") classicUpdate();
  } catch (e) {
    // The chart starts empty and fills from the live stream.
  }
}

/** classicSample keeps the last minutes of the run in the buffer. */
function classicSample(doc) {
  const run = (doc.runs || []).find((r) => r.state === "running") || (doc.runs || [])[0];
  const stats = run && run.stats ? run.stats : null;
  classic.hist.push({
    t: doc.time || Date.now(),
    gen: run ? run.gen_tok_s : null,
    prompt: run ? run.prompt_tok_s : null,
    running: stats ? stats.requests_running : null,
    waiting: stats ? stats.requests_waiting : null,
    kv: stats ? stats.kv_cache_usage * 100 : null,
    spec: stats && stats.spec_draft_tokens > 0 ? stats.spec_accept_rate * 100 : null,
  });
  if (classic.hist.length > CLASSIC_HIST) classic.hist.shift();
}

/* ---------- the model: bernie's data in the shape of the reference -------- */

/** classicModel reads the state of the host and answers the questions the
 *  reference's dashboard view asks. */
function classicModel() {
  const tele = classic.tele || (typeof state !== "undefined" ? state.tele : null) || { gpus: [], runs: [] };
  const hw = (typeof state !== "undefined" && state.hw) || null;
  const list = (typeof state !== "undefined" && state.runs) || [];
  const runs = tele.runs || [];
  const live = runs.find((r) => r.state === "running") || null;
  const starting = runs.find((r) => r.state === "starting") || null;
  const record = live || starting || null;
  const stats = live && live.stats ? live.stats : null;
  const runList = record ? (list.find((r) => r.name === record.name) || null) : null;

  // the word of the state, the way the reference words it
  let word = "Off";
  let dot = "idle";
  if (live) { word = "Serving"; dot = "ok"; }
  else if (starting) { word = "Starting"; dot = "run"; }

  // what is running: the recipe, the mode and the port
  const layout = record
    ? [record.name, runList && runList.mode, runList && runList.port ? "port " + runList.port : null]
      .filter(Boolean).join(" · ")
    : "no run on this host";

  // the address other machines use
  let url = "";
  if (record) {
    url = record.url || (runList ? runURL(runList) : "");
    if (url && !/\/v1$/.test(url)) url = url.replace(/\/$/, "") + "/v1";
  }

  const hostInfo = (hw && hw.host) || null;
  const hwGpus = (hw && hw.gpus) || [];
  const names = {};
  for (const g of hwGpus) names[g.hip_index] = g.name || g.gfx || "";

  return {
    word: word,
    dot: dot,
    live: !!live,
    starting: !!starting,
    record: record,
    runList: runList,
    stats: stats,
    layout: layout,
    url: url,
    model: record ? record.name : "",
    gpus: (tele.gpus || []).map((g) => ({
      index: g.hip_index,
      name: names[g.hip_index] || "",
      util: g.busy_pct,
      vramUsed: g.vram_used,
      vramTotal: g.vram_total,
      power: g.power_w,
      powerCap: g.power_cap_w,
      temp: g.temp_c,
      tempMem: g.temp_mem_c,
      sclk: g.sclk_mhz,
    })),
    host: hostInfo,
    hostname: hostInfo ? hostInfo.hostname : "",
    ramTotal: hostInfo ? hostInfo.ram_bytes : null,
  };
}

/** classicLive is the number of the reference: the live value, or the last
 *  real one while the server is idle, with the note that says which. */
function classicLive(key, digits) {
  const hist = classic.hist;
  const cur = hist.length ? hist[hist.length - 1][key] : null;
  if (cur != null && cur > 0) {
    const now = Date.now() / 1000;
    const busy = hist.filter((s) => s.t / 1000 > now - 60 && s[key] != null && s[key] > 0).map((s) => s[key]);
    const avg = busy.length ? busy.reduce((a, b) => a + b, 0) / busy.length : null;
    return { v: cur, note: avg != null ? "1-min avg " + cfmt(avg, digits) : null, idle: false };
  }
  for (let i = hist.length - 1; i >= 0; i--) {
    const v = hist[i][key];
    if (v != null && v > 0) return { v, note: "idle · last " + cago(hist[i].t / 1000) + " ago", idle: true };
  }
  return { v: cur, note: null, idle: false };
}

/** classicTail is the last samples of one number: the line under a tile. */
function classicTail(key) {
  return classic.hist.slice(-CLASSIC_TAIL).map((s) => s[key]);
}

/* ---------- the skeleton ---------- */

/** classicBuild builds the tab once. The stream only fills it afterwards. */
function classicBuild() {
  const panel = classic.panel;
  const el = classic.el;

  el.intro = h("p.classic-intro", { role: "note" },
    h("span", "Original reference dashboard design, fed with live data from this host. Compare with the Dashboard tab."),
    h("a", { href: CLASSIC_DOCS, target: "_blank", rel: "noopener" }, "the design notes"));

  el.live = h("span.sr-only", { role: "status", "aria-live": "polite" });

  el.pill = h("span.pill.idle");
  el.layout = h("span.muted");
  el.heroBtns = h("div.row");
  el.word = h("h1.punch.slam");
  el.sub = h("p.hero-sub");
  el.meta = h("div.row");
  el.genNum = h("span", { "data-k": "kpi-gen" }, "–");
  el.genNote = h("div.hero-note");
  el.genSpark = h("div.hero-spark");
  el.tiles = h("div.tiles");
  el.chart1 = h("div");
  el.chart2 = h("div");
  el.gpuHead = h("span.muted");
  el.gpuGrid = h("div.gpu-grid");
  el.host = h("div");
  el.connect = h("div");

  el.hero = h("div.panel.hero-panel",
    h("div.hero-l",
      h("div.row.hero-top", el.pill, el.layout, h("span.spacer"), el.heroBtns),
      el.word, el.sub, el.meta),
    h("div.hero-r",
      h("div.eyebrow", "Output speed", classicMark("The rate at which the server writes the answer. Tokens per second.")),
      h("div.hero-num", el.genNum, h("small", "tok/s")),
      el.genNote, el.genSpark));

  el.tilesHost = h("div", el.tiles);
  el.gpuHeadRow = h("div.panel-head.gpu-head", h("h2", "GPUs"), el.gpuHead);

  put(panel,
    el.intro, el.live,
    el.hero,
    el.tilesHost,
    h("div.grid2",
      h("div.panel", h("div.panel-head", h("h2", "Output speed"), h("span.muted", "tok/s, last 5 min")), el.chart1),
      h("div.panel", h("div.panel-head", h("h2", "Prompt reading"), h("span.muted", "tok/s, last 5 min")), el.chart2)),
    el.gpuHeadRow,
    el.gpuGrid,
    h("div.grid2", h("div.panel", el.host), h("div.panel", el.connect)));

  // the six tiles of the reference
  const tiles = [
    { id: "prompt", label: "Prompt reading", unit: "tok/s", mark: "Reading the prompt. Tokens per second; this is where long documents feel slow.", color: "var(--series-3)" },
    { id: "run", label: "Active requests", unit: "", mark: "The requests the server answers right now.", color: "var(--series-1)" },
    { id: "ttft", label: "First token", unit: "s", mark: "The wait before the answer starts. This host does not measure it.", color: "var(--series-1)" },
    { id: "kv", label: "KV cache", unit: "%", mark: "The share of the GPU memory that holds the conversation so far.", color: "var(--series-3)" },
    { id: "pc", label: "Prefix cache hits", unit: "%", mark: "The share of the prompt work the server reused. This host does not measure it.", color: "var(--series-1)" },
    { id: "sd", label: "Draft accepted", unit: "%", mark: "The share of the drafted tokens the model kept. Higher means the speculation pays off.", color: "var(--series-3)" },
  ];
  for (const t of tiles) {
    const num = h("span", { "data-k": "kpi-" + t.id }, "–");
    const sub = h("div.tile-s");
    const spark = h("div.tile-sp");
    classic.el["num-" + t.id] = num;
    classic.el["sub-" + t.id] = sub;
    classic.el["spark-" + t.id] = spark;
    classic.el["tile-" + t.id] = h("div.tile",
      h("div.tile-l", t.label, classicMark(t.mark)),
      h("div.tile-n", num, t.unit ? h("small", t.unit) : null),
      sub, spark);
  }
  put(el.tiles, tiles.map((t) => classic.el["tile-" + t.id]));

  // the host panel and the connect panel
  el.hostCpu = h("b");
  el.hostRam = h("b");
  el.hostReq = h("b");
  el.hostE2E = h("b");
  put(el.host,
    h("div.panel-head", h("h2", "This machine"), h("span.muted", { id: "classic-host" })),
    h("div.stat", h("div.stat-h", h("span", "CPU"), el.hostCpu), h("div.meter", classicMeterFill(0))),
    h("div.stat", h("div.stat-h", h("span", "RAM"), el.hostRam), h("div.meter.mem", classicMeterFill(0))),
    h("div.stat-row.big",
      h("span", h("span.muted", "Requests served"), el.hostReq),
      h("span", h("span.muted", "Avg request"), el.hostE2E)),
    h("p.classic-note", "The CPU load, the memory in use and the request times are not measured on this host."));
  el.hostNote = el.host.querySelector("#classic-host");

  el.connectText = h("p.muted.connect-text");
  el.connectCmd = h("div.cmd");
  el.seg = h("div.seg");
  for (const [k, label] of [["curl", "curl"], ["python", "Python"]]) {
    el.seg.append(h("button", {
      type: "button",
      class: classic.snip === k ? "on" : "",
      "aria-pressed": String(classic.snip === k),
      onclick: () => {
        classic.snip = k;
        for (const b of el.seg.children) {
          const on = b.textContent === label && classic.snip === k;
          b.classList.toggle("on", on);
          b.setAttribute("aria-pressed", String(on));
        }
        classicUpdate();
      },
    }, label));
  }
  put(el.connect,
    h("div.panel-head", h("h2", "Connect an app"), el.seg),
    el.connectText, el.connectCmd);

  // the tooltip of the tab: one element, moved around
  classic.tip = h("div.tip", { role: "tooltip", hidden: true });
  panel.append(classic.tip);
}

/* ---------- the marks and the tooltip ---------- */

/** classicMark is the small round mark that explains a number. It opens on
 *  hover and on focus, and closes on Escape. */
function classicMark(text) {
  const btn = h("button.q", { type: "button", "aria-label": "What is this?" }, "i");
  const show = () => classicTip(btn, text);
  btn.addEventListener("mouseenter", show);
  btn.addEventListener("focus", show);
  btn.addEventListener("mouseleave", () => classicTip(null));
  btn.addEventListener("blur", () => classicTip(null));
  return btn;
}

/** classicTip shows the explanation next to a mark, or hides it. */
function classicTip(anchor, text) {
  const tip = classic.tip;
  if (!tip) return;
  if (!anchor || !text) { tip.hidden = true; return; }
  put(tip, h("b", "What this means"), h("span", text));
  tip.hidden = false;
  const r = anchor.getBoundingClientRect();
  const box = tip.getBoundingClientRect();
  const left = Math.min(window.innerWidth - box.width - 8, Math.max(8, r.left));
  const top = Math.min(window.innerHeight - box.height - 8, r.bottom + 8);
  tip.style.left = left + "px";
  tip.style.top = top + "px";
}

/* ---------- the update ---------- */

/** classicUpdate paints the tab with the data of this host. */
function classicUpdate() {
  const el = classic.el;
  if (!el || !classic.panel) return;
  const m = classicModel();

  // the hero: the state, the name of the run and the address
  el.pill.className = "pill " + m.dot;
  put(el.pill, h("span", m.word));
  el.layout.textContent = m.layout;
  put(el.word, m.live ? h("span.key", m.word) : m.word);
  el.sub.textContent = m.sub || "";
  if (m.sub) {
    if (m.live && m.model) put(el.sub, h("b", m.model), " is answering requests");
    else el.sub.textContent = m.sub;
  }
  put(el.meta);
  if (m.url) {
    el.meta.append(h("span.chip.mono", m.url));
    el.meta.append(h("button.ghost.small", { type: "button", "data-copy": m.url }, "Copy URL"));
  } else {
    el.meta.append(h("span.muted", "No run is serving on this host."));
  }
  put(el.heroBtns, classicHeroButtons(m));

  // the output speed of the hero
  const gen = classicLive("gen", 1);
  classicCount(el.genNum, gen.v, 1);
  el.genNum.parentElement.classList.toggle("idle", !!gen.idle);
  el.genNote.textContent = gen.note || "";
  put(el.genSpark, classicSpark(classicTail("gen"), "key", 44));

  // the six tiles
  const prompt = classicLive("prompt", 0);
  classicCount(el["num-prompt"], prompt.v, 0);
  el["num-prompt"].parentElement.classList.toggle("idle", !!prompt.idle);
  el["sub-prompt"].textContent = prompt.note || "Prefill, all users";
  put(el["spark-prompt"], classicSpark(classicTail("prompt"), "s3"));

  const running = classicLive("running", 0);
  classicCount(el["num-run"], running.v, 0);
  el["num-run"].parentElement.classList.toggle("idle", false);
  el["sub-run"].textContent = m.stats ? cfmt(m.stats.requests_waiting) + " waiting in line" : "no run is serving";
  put(el["spark-run"], classicSpark(classicTail("running"), "s1"));

  classicCount(el["num-ttft"], null, 2);
  el["sub-ttft"].textContent = "n/a on this host";
  put(el["spark-ttft"], classicSpark([], "s1"));

  const kv = classicLive("kv", 0);
  classicCount(el["num-kv"], kv.v, 0);
  el["sub-kv"].textContent = kv.note || "of GPU cache in use";
  put(el["spark-kv"], classicSpark(classicTail("kv"), "s3"));

  classicCount(el["num-pc"], null, 0);
  el["sub-pc"].textContent = "n/a on this host";
  put(el["spark-pc"], classicSpark([], "s1"));

  const spec = classicLive("spec", 0);
  classicCount(el["num-sd"], spec.v, 0);
  el["num-sd"].parentElement.classList.toggle("idle", !!spec.idle);
  el["sub-sd"].textContent = spec.note || (m.stats && m.stats.spec_draft_tokens > 0 ? "MTP drafts kept" : "no draft tokens yet");
  put(el["spark-sd"], classicSpark(classicTail("spec"), "s3"));

  // the two charts
  const genPts = classic.hist.map((s) => [s.t / 1000, s.gen]);
  put(el.chart1, classicChart({
    id: "classic-gen", label: "Output tokens per second", height: 200, area: true,
    series: [{ name: "Output tok/s", cls: "s1", pts: genPts }],
    yFmt: (v) => cfmt(v, v < 10 ? 1 : 0),
  }));
  const promptPts = classic.hist.filter((s) => s.prompt != null).map((s) => [s.t / 1000, s.prompt]);
  put(el.chart2, classicChart({
    id: "classic-prompt", label: "Prompt tokens per second", height: 200, area: true,
    series: [{ name: "Prompt tok/s", cls: "s3", pts: promptPts }],
  }));
  classicCharts();

  // the GPU cards
  const totPow = m.gpus.reduce((a, g) => a + (g.power || 0), 0);
  const hottest = m.gpus.length ? Math.max(...m.gpus.map((g) => g.temp || 0)) : null;
  el.gpuHead.textContent = m.gpus.length
    ? m.gpus.length + " cards · " + cfmt(totPow) + " W total · hottest " + (hottest == null ? "–" : cfmt(hottest) + "°C")
    : "no GPU readings yet";
  put(el.gpuGrid, m.gpus.length ? m.gpus.map((g) => classicGpuCard(g, m)) : h("p.empty", "No GPU readings yet."));

  // the host panel
  el.hostNote.textContent = m.hostname || "";
  put(el.hostCpu, "–", h("span.muted", " load n/a on this host"));
  put(el.hostRam, "–", h("span.muted", m.ramTotal ? " / " + bytes(m.ramTotal) : ""));
  put(el.hostReq, "–");
  put(el.hostE2E, "–");

  // the connect panel
  const base = m.url ? m.url.replace(/\/v1$/, "") : "";
  const model = m.model || "the model of the run";
  put(el.connectText, "Any OpenAI-compatible app works: give it this base URL and the model name ",
    h("b", model), ". No API key needed.");
  const code = classic.snip === "curl"
    ? (base
      ? "curl " + base + "/v1/chat/completions \\\n  -H 'Content-Type: application/json' \\\n  -d '{\"model\":\"" + model + "\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]}'"
      : "Start a run to get an address.")
    : (base
      ? "from openai import OpenAI\nclient = OpenAI(base_url=\"" + base + "/v1\", api_key=\"not-used\")\nr = client.chat.completions.create(\n    model=\"" + model + "\",\n    messages=[{\"role\": \"user\", \"content\": \"Hello!\"}])\nprint(r.choices[0].message.content)"
      : "Start a run to get an address.");
  put(el.connectCmd, h("code", code), h("button.ghost.small.copy", { type: "button" }, "Copy"));

  // the live region of the tab: one sentence for the reader of a screen
  // reader, so the numbers are not read out one by one
  el.live.textContent = m.live
    ? "Output speed " + cfmt(gen.v, 1) + " tokens per second. " + cfmt(running.v, 0) + " requests running."
    : "No run is serving on this host.";
}

/** classicHeroButtons are the buttons of the hero. The reference starts and
 *  stops its server here; bernie starts a run from the Setup tab and stops
 *  it with the runs endpoint. */
function classicHeroButtons(m) {
  if (m.live && m.record) {
    const btn = h("button.ghost", { type: "button" }, "Stop the run");
    btn.addEventListener("click", async () => {
      if (!confirm("Stop the run " + m.record.name + "?")) return;
      btn.disabled = true;
      try {
        await api("/api/runs/" + encodeURIComponent(m.record.name), { method: "DELETE" });
        toast("The run is stopping.");
      } catch (e) {
        toast("The run did not stop: " + (e.message || e));
      } finally {
        btn.disabled = false;
      }
    });
    return [btn];
  }
  return [h("a.btn.go", { href: "#setup" }, "Start a run")];
}

/** classicMeterFill is the coloured part of a meter. The width is a value and
 *  not a class, so it goes through the CSSOM: the CSP of the page refuses
 *  style attributes but allows the CSSOM. */
function classicMeterFill(pct) {
  const fill = h("i");
  fill.style.width = Math.max(0, Math.min(100, pct || 0)).toFixed(1) + "%";
  return fill;
}

/** classicGpuCard is one card of a GPU, the way the reference draws it. */
function classicGpuCard(g, m) {
  const mem = g.vramTotal ? (100 * (g.vramUsed || 0)) / g.vramTotal : 0;
  const pw = g.powerCap ? (100 * (g.power || 0)) / g.powerCap : 0;
  const hot = (g.temp || 0) >= 95;
  const used = m.live || m.starting;
  const tail = (typeof state !== "undefined" && state.hist && state.hist["busy" + g.index]) || [];
  return h("div.gpu-card",
    h("div.gpu-top",
      h("div", h("div.gpu-name", "GPU " + g.index), h("div.muted.small-t", g.name || "")),
      used ? h("span.pill.ok", "In use") : h("span.pill.idle", "No run")),
    h("div.gpu-mid",
      h("div.ring-wrap",
        classicRing(g.util, "s1"),
        h("div.ring-n", cfmt(g.util), h("small", "%")),
        h("div.ring-l", "busy")),
      h("div.gpu-stats",
        h("div.stat",
          h("div.stat-h", h("span", "Memory"),
            h("b", bytes(g.vramUsed || 0), h("span.muted", g.vramTotal ? " / " + bytes(g.vramTotal) : ""))),
          h("div.meter.mem", classicMeterFill(mem))),
        h("div.stat",
          h("div.stat-h", h("span", "Power"),
            h("b", cfmt(g.power) + " W", h("span.muted", g.powerCap ? " / " + cfmt(g.powerCap) + " W" : ""))),
          h("div.meter.pow", classicMeterFill(pw))),
        h("div.stat-row",
          h("span", h("span.muted", "Temp"), " ", h("b" + (hot ? ".temp-hot" : ""), hot ? "! " : "", cfmt(g.temp) + "°C")),
          h("span", h("span.muted", "Mem"), " ", h("b", g.tempMem == null ? "–" : cfmt(g.tempMem) + "°C")),
          h("span", h("span.muted", "Clock"), " ", h("b", g.sclk == null ? "–" : cfmt(g.sclk)))))),
    h("div.gpu-spark", classicSpark(tail, "s1", 28)));
}

/* ---------- the crosshair of the charts ---------- */

/** classicCharts gives every chart of the tab its crosshair. The pointer
 *  finds the point under it and the tooltip says what it is. */
function classicCharts() {
  for (const box of classic.panel.querySelectorAll(".chart")) {
    if (box.dataset.wired === "1") continue;
    box.dataset.wired = "1";
    box.addEventListener("mousemove", (ev) => classicCrosshair(ev, box));
    box.addEventListener("mouseleave", () => {
      const c = classic.charts.get(box);
      if (c) c.xh.setAttribute("visibility", "hidden");
      classicTip(null);
    });
  }
}

/** classicCrosshair moves the crosshair of a chart and fills the tooltip. */
function classicCrosshair(ev, box) {
  const c = classic.charts.get(box);
  if (!c) return;
  const rect = c.svg.getBoundingClientRect();
  const vx = ((ev.clientX - rect.left) / rect.width) * c.W;
  let best = null;
  let bd = Infinity;
  for (const s of c.opts.series) {
    for (const p of s.pts) {
      const d = Math.abs(p[0] - (c.x0 + ((vx - c.L) / (c.W - c.L - c.R)) * (c.x1 - c.x0)));
      if (d < bd) { bd = d; best = p[0]; }
    }
  }
  if (best == null) return;
  c.xh.setAttribute("x1", c.sx(best));
  c.xh.setAttribute("x2", c.sx(best));
  c.xh.setAttribute("visibility", "visible");
  const rows = c.opts.series.map((s) => {
    const p = s.pts.find((q) => q[0] === best);
    if (!p || p[1] == null) return null;
    return h("div.row",
      h("span.c-dot.c-color-" + s.cls, "●"),
      h("span", s.name + ": "),
      h("b.tip-value", c.opts.yFmt ? c.opts.yFmt(p[1]) : cfmt(p[1], 1)));
  }).filter(Boolean);
  const head = new Date(best * 1000).toLocaleTimeString();
  put(classic.tip, h("b", head), ...(rows.length ? rows : [h("div", "no data")]));
  classic.tip.hidden = false;
  classic.tip.style.left = Math.min(window.innerWidth - 330, ev.clientX + 14) + "px";
  classic.tip.style.top = Math.min(window.innerHeight - 120, ev.clientY + 14) + "px";
}
