"use strict";
// The Benchmark tab: the state headline, the suites, the result, the
// comparison, the history and the A/B run. See docs/design/benchmark.md.

/** benchState is what the tab knows between two calls. */
const benchState = {
  suites: [],
  results: [],
  suite: "",
  result: null,
  compare: null,
  stop: null,
};

/** benchSection is one section of the tab: its label and what it holds. */
function benchSection(label, ...body) {
  return h("section.section", h("h2.section-label", label), ...body);
}

/** benchFailed says what could not be read and offers to read it again. */
function benchFailed(what, e, retry) {
  const btn = h("button", { type: "button" }, "Try again");
  if (retry) btn.addEventListener("click", retry);
  return h("div.err", h("b", what), h("p.small", String((e && e.message) || e || "")), btn);
}

/** benchResults reads the saved results, the newest first. */
async function benchResults() {
  const d = await api("/api/bench");
  return (Array.isArray(d) ? d : (d && d.results) || []).slice()
    .sort((a, b) => new Date(b.time) - new Date(a.time));
}

/** benchSuiteLine names the suite of a result and its digest. */
function benchSuiteLine(r) {
  if (!r || !r.suite) return null;
  const digest = String(r.suite.digest || "");
  return "Suite " + r.suite.id + " · digest " + (digest ? digest.slice(0, 12) : "unknown") +
    " · version " + (r.suite.version || 1);
}

/** benchCards counts the cards a result ran on. The fingerprint names them
 *  as "2x Radeon PRO W7800, 8x RADEON PRO V620". */
function benchCards(fp) {
  const s = (fp && fp.gpus) || "";
  if (!s) return null;
  let n = 0, found = false;
  for (const part of s.split(",")) {
    const m = part.trim().match(/^(\d+)\s*x\s/i);
    if (m) { n += Number(m[1]); found = true; }
  }
  return found ? n : null;
}

/** benchHeadline writes the state headline of the tab: what the last
 *  benchmark reached and what it measured. */
function benchHeadline(head, results) {
  const last = results && results[0];
  if (!last) {
    setHead(head, "Nothing has been measured yet.",
      "A benchmark measures the tokens per second of a running server. Run one below and this page fills with its numbers.");
    return;
  }
  const cells = last.cells || [];
  const out = benchBest(cells, (c) => c.output_tok_s);
  const prefill = benchBest(cells, (c) => (c.prefill || {}).agg_tok_s);
  const decode = benchBest(cells, (c) => (c.decode || {}).agg_tok_s);
  const cards = benchCards(last.fingerprint);
  const title = out
    ? "The last benchmark reached " + benchNum(out, 1) + " tok/s."
    : "The last benchmark has no output rate.";
  const lede = "The " + benchSuiteOf(last) + " suite measured " + cells.length + " cells" +
    (cards ? " on " + cards + " cards" : "") + ". The prefill was " + benchNum(prefill, 0) +
    " tok/s and the decode " + benchNum(decode, 1) + " tok/s.";
  setHead(head, title, lede);
}

/** benchResultBody is the content of the result card: the table of the cells
 *  and the sentence that says how the numbers are measured. */
function benchResultBody(r) {
  if (!r) return emptyState("No result yet.", "Run a suite above to fill this table.");
  return h("div",
    h("p.bench-result-title", "Result " + r.id),
    h("p.small.muted", benchSuiteLine(r)),
    h("p.small.muted", METRIC_HELP),
    cellsTable(r.cells));
}

/** benchCopy puts a text on the clipboard. The shell owns the control; the
 *  view only asks for it. */
async function benchCopy(text) {
  if (typeof copy === "function") return await copy(text);
  if (typeof copyText === "function") return await copyText(text);
  return false;
}

/** benchReceipt reads the markdown receipt of a comparison. */
async function benchReceipt(a, b) {
  const res = await fetch("/api/bench/compare?a=" + encodeURIComponent(a) + "&b=" + encodeURIComponent(b) + "&format=markdown",
    { credentials: "same-origin" });
  if (!res.ok) throw new Error("The receipt could not be read.");
  return await res.text();
}

/** benchWatch follows a running job: it reads the job document and the event
 *  stream, and it stops when the job ends. */
function benchWatch(id, onProgress, onDone, onFail) {
  let stopped = false;
  let stream = null;
  const stop = () => {
    stopped = true;
    clearInterval(timer);
    if (stream) stream.close();
  };
  const timer = setInterval(async () => {
    if (stopped) return;
    try {
      const job = await api("/api/bench/" + encodeURIComponent(id));
      onProgress(job);
      if (job && (job.status === "done" || job.status === "failed" || job.result_id)) {
        stop();
        if (job.status === "failed") onFail(new Error(job.phase || "The run stopped."));
        else onDone(job);
      }
    } catch (e) {
      stop();
      onFail(e);
    }
  }, 2000);
  try {
    stream = new EventSource("/api/bench/" + encodeURIComponent(id) + "/events");
    stream.addEventListener("progress", (ev) => {
      try { onProgress(JSON.parse(ev.data)); } catch (e) { /* a plain line */ }
    });
    stream.addEventListener("log", (ev) => {
      try { onProgress({ log: JSON.parse(ev.data).text }); } catch (e) { onProgress({ log: ev.data }); }
    });
    stream.addEventListener("done", () => { stop(); onDone({}); });
    stream.addEventListener("failed", (ev) => {
      stop();
      let text = "The run stopped.";
      try { text = JSON.parse(ev.data).error || text; } catch (e) { /* keep the sentence */ }
      onFail(new Error(text));
    });
    stream.onerror = () => { /* the poll keeps the card alive */ };
  } catch (e) { /* no stream: the poll shows the progress */ }
  return stop;
}

async function viewBenchmark(root) {
  const runBtn = h("button.primary", { type: "button" }, "Run the suite");
  const advBtn = h("button", { type: "button" }, "Advanced");
  const head = stateHead("Benchmark", h("div.head-actions", runBtn, advBtn));

  const suitesCard = h("div.card.bench-card");
  const progress = benchProgress();
  const resultCard = h("div.card.bench-card");
  const curveCard = h("div.card.bench-card");
  const compareCard = h("div.card.bench-card");
  const historyCard = h("div.card.bench-card");
  const advanced = benchAdvanced([]);

  const base = h("select", { id: "bench-cmp-base" });
  const cand = h("select", { id: "bench-cmp-cand" });
  const cmpBtn = h("button", { type: "button" }, "Compare");
  const cmpOut = h("div.bench-compare-out");
  const receiptBtn = h("button", { type: "button" }, "Copy the receipt");
  const abOut = h("div.bench-ab-out");

  put(root, head,
    benchSection("The suites", suitesCard, progress.el),
    benchSection("The result", resultCard, curveCard),
    benchSection("The comparison", h("div.card.bench-card",
      h("div.bench-pickers",
        h("label", { for: "bench-cmp-base" }, "Baseline"), base,
        h("label", { for: "bench-cmp-cand" }, "Candidate"), cand,
        cmpBtn, receiptBtn),
      cmpOut)),
    benchSection("The history", historyCard),
    benchSection("An A/B run", benchAB((b, c, out, btn) => runAB(b, c, out, btn))),
    advanced.el);
  advBtn.addEventListener("click", () => advanced.btn.click());

  /** loadSuites reads the built-in suites. */
  async function loadSuites() {
    try {
      const list = await api("/api/bench/suites");
      benchState.suites = list || [];
      put(suitesCard, benchSuiteCards(benchState.suites, benchState.suite, pickSuite));
    } catch (e) {
      put(suitesCard, benchFailed("The suites could not be read.", e, loadSuites));
    }
  }

  /** pickSuite takes the suite of the next run. */
  function pickSuite(id) {
    benchState.suite = id;
    runBtn.disabled = false;
    for (const card of suitesCard.querySelectorAll(".bench-suite")) {
      const radio = card.querySelector("input");
      if (radio) card.classList.toggle("picked", radio.value === id);
    }
  }

  /** fillPickers fills the two pickers of the comparison. */
  function fillPickers(list) {
    const keep = [base.value, cand.value];
    put(base, list.map((r) => h("option", { value: r.id }, r.id + " · " + benchSuiteOf(r))));
    put(cand, list.map((r) => h("option", { value: r.id }, r.id + " · " + benchSuiteOf(r))));
    if (list.length > 1) {
      base.value = keep[0] && list.some((r) => r.id === keep[0]) ? keep[0] : list[1].id;
      cand.value = keep[1] && list.some((r) => r.id === keep[1]) ? keep[1] : list[0].id;
    }
    const off = list.length < 2;
    cmpBtn.disabled = off;
    receiptBtn.disabled = off;
  }

  /** loadResults reads the saved results and draws the tab. */
  async function loadResults(pickId) {
    try {
      const list = await benchResults();
      benchState.results = list;
      benchHeadline(head, list);
      const pick = pickId ? list.find((r) => r.id === pickId) : (list[0] || null);
      benchState.result = pick || null;
      put(resultCard, benchResultBody(pick));
      const curve = pick ? lengthCurve(pick.cells) : null;
      put(curveCard, curve || emptyState("This suite has no ladder.",
        "Run the long-context suite to see the curve."));
      put(historyCard, benchHistory(list, null, (r) => {
        cand.value = r.id;
        compare();
      }));
      fillPickers(list);
    } catch (e) {
      put(resultCard, benchFailed("The results could not be read.", e, () => loadResults()));
      setHead(head, "The results could not be read.", "Try again: the endpoint may be busy.");
    }
  }

  /** compare draws the comparison of the two picked results. */
  async function compare() {
    const a = base.value, b = cand.value;
    if (!a || !b || a === b) {
      put(cmpOut, emptyState("Pick two results to compare."));
      return;
    }
    cmpBtn.disabled = true;
    put(cmpOut, loading());
    try {
      const cmp = await api("/api/bench/compare?a=" + encodeURIComponent(a) + "&b=" + encodeURIComponent(b));
      benchState.compare = cmp;
      if (cmp.valid === false) {
        // The two runs measured the same thing, but the comparison could
        // not decide: the reasons say why.
        put(cmpOut, h("div",
          h("p.small.muted", (cmp.reasons || []).join(" · ") || "The comparison could not decide."),
          barCompare(cmp)));
      } else {
        put(cmpOut, barCompare(cmp));
      }
    } catch (e) {
      put(cmpOut, benchFailed("The two results measured different things.", e, compare));
    }
    cmpBtn.disabled = false;
  }

  /** runSuite starts the run of the picked suite. */
  async function runSuite() {
    if (!benchState.suite) return;
    runBtn.disabled = true;
    progress.show({ done: 0, total: 0 });
    let res;
    try {
      res = await fetch("/api/bench/run", {
        method: "POST",
        headers: { "X-Bernie": "1", "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ suite: benchState.suite }),
      });
    } catch (e) {
      progress.fail("The run could not be started.");
      runBtn.disabled = false;
      return;
    }
    if (res.status === 404 || res.status === 405) {
      // This build has no job endpoint: the run answers in one call.
      try {
        const r = await api("/api/bench", { method: "POST", body: { suite: benchState.suite } });
        progress.hide();
        runBtn.disabled = false;
        await loadResults(r && r.id);
      } catch (e) {
        progress.fail(String(e.message || e));
        runBtn.disabled = false;
      }
      return;
    }
    if (!res.ok) {
      let text = "The run could not be started.";
      try { text = (await res.json()).error || text; } catch (e) { /* keep the sentence */ }
      progress.fail(text);
      runBtn.disabled = false;
      return;
    }
    const job = await res.json();
    benchWatch(job.id,
      (j) => {
        if (j.log) progress.log(j.log);
        else progress.show(j);
      },
      async (j) => {
        progress.hide();
        runBtn.disabled = false;
        await loadResults(j && j.result_id);
      },
      (e) => {
        progress.fail(String(e.message || e));
        runBtn.disabled = false;
      });
  }

  /** runAB starts the A/B run of the two arms. */
  async function runAB(baseArm, candArm, out, btn) {
    btn.disabled = true;
    put(out, h("p.small.muted", "The A/B run starts. It visits the base, then the candidate, then the candidate again, then the base."));
    let res;
    try {
      res = await fetch("/api/bench/ab", {
        method: "POST",
        headers: { "X-Bernie": "1", "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ base: baseArm, cand: candArm }),
      });
    } catch (e) {
      put(out, benchFailed("The A/B run could not be started.", e, null));
      btn.disabled = false;
      return;
    }
    if (!res.ok) {
      let text = "The A/B run could not be started.";
      try { text = (await res.json()).error || text; } catch (e) { /* keep the sentence */ }
      put(out, benchFailed(text, null, null));
      btn.disabled = false;
      return;
    }
    const job = await res.json();
    benchWatch(job.id,
      (j) => { if (j.log) progress.log(j.log); },
      async () => {
        btn.disabled = false;
        await loadResults();
        put(out, h("p.small.muted", "The A/B run ended. Pick its two results in the comparison above."));
      },
      (e) => {
        btn.disabled = false;
        put(out, benchFailed("The A/B run stopped.", e, null));
      });
  }

  runBtn.addEventListener("click", runSuite);
  cmpBtn.addEventListener("click", compare);
  receiptBtn.addEventListener("click", async () => {
    const a = base.value, b = cand.value;
    if (!a || !b || a === b) return;
    receiptBtn.disabled = true;
    try {
      const text = await benchReceipt(a, b);
      const ok = await benchCopy(text);
      toast(ok ? "Copied the receipt." : "Copy failed: select the text and press Ctrl-C.", ok);
    } catch (e) {
      toast("The receipt could not be read.", false);
    }
    receiptBtn.disabled = false;
  });

  await loadSuites();
  await loadResults();
  if (!benchState.suite && benchState.suites.length) pickSuite(benchState.suites[0].id);
}
