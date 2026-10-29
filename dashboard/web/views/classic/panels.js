"use strict";
// ---------- Classic: the panels ----------
// The DOM of the Classic tab: the hero, the six tiles, the GPU cards, the
// host panel, the connect panel, the marks and the tooltip. The tab is built
// once; the stream only fills it afterwards.
//
// The tab is three files: classic.js holds the state and the model, paint.js
// holds the update. The bundle is named in js/boot.js.

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
        classicUpdate(state.tele);
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

/* ---------- the parts of the panels ---------- */
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
          h("span", h("span.muted", "Clock"), " ", h("b", g.sclk == null ? "–" : cfmt(g.sclk) + " MHz"))))),
    h("div.gpu-spark", classicSpark(tail, "s1", 28)));
}
