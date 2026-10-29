"use strict";
// ---------- Classic: the paint ----------
// The update of the Classic tab: the numbers of the host on the panels the
// tab is built of, and the crosshair of the charts.
//
// The tab is three files: classic.js holds the state and the model, panels.js
// holds the DOM. The bundle is named in js/boot.js.

/* ---------- the update ---------- */

/** classicUpdate paints the tab with the data of this host. The doc is the
 *  one of the sample that arrived (the registry of js/data.js passes it
 *  through); without one the view paints the last document of the shared
 *  state, so the first paint at tab-open uses what the page already has. */
function classicUpdate(doc) {
  const el = classic.el;
  if (!el || !classic.panel) return;
  const m = classicModel(doc);

  // the hero: the state, the name of the run and the address
  el.pill.className = "pill " + m.dot;
  put(el.pill, h("span", m.word));
  el.layout.textContent = m.layout;
  put(el.word, m.live ? h("span.key", m.word) : m.word);
  el.sub.textContent = m.sub || "";
  if (m.sub) {
    if (m.model) put(el.sub, h("b", m.model), " " + m.sub);
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
  el["sub-sd"].textContent = spec.note || (m.stats && m.stats.spec_draft_tokens_total > 0 ? "MTP drafts kept" : "no draft tokens yet");
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
