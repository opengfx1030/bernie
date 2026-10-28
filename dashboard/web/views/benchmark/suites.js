"use strict";
// The Benchmark tab: the suite cards, the live progress, the history, the
// A/B run and the Advanced panel. See docs/design/benchmark.md.

/** benchSuiteCells counts the cells of a suite. */
function benchSuiteCells(s) {
  let n = 0;
  for (const g of (s && s.groups) || []) n += (g.concurrency || []).length;
  return n;
}

/** benchSuiteEstS is the rough wall time of a suite in seconds. */
function benchSuiteEstS(s) {
  let n = 0;
  for (const g of (s && s.groups) || []) n += (g.est_s || 0) * (g.reps || 1);
  return n;
}

/** benchMinutes turns a number of seconds into a short time. */
function benchMinutes(s) {
  if (!s) return "–";
  if (s < 60) return s + " s";
  const m = Math.round(s / 60);
  return m + " min";
}

/** benchSuiteCard is one suite: a radio, the name, what it measures, the
 *  number of cells, the estimated time and the digest in mono. */
function benchSuiteCard(s, picked, onPick) {
  const id = "bench-suite-" + s.id;
  const digest = s.digest || "";
  const radio = h("input.bench-radio", {
    type: "radio", name: "bench-suite", id: id, value: s.id, checked: picked,
  });
  radio.addEventListener("change", () => onPick(s.id));
  return h("label.bench-suite", { for: id, class: picked ? "bench-suite picked" : "bench-suite" },
    radio,
    h("div.bench-suite-body",
      h("p.bench-suite-name", s.name || s.id),
      h("p.bench-suite-desc", s.description || ""),
      h("p.bench-suite-meta",
        benchSuiteCells(s) + " cells · about " + benchMinutes(benchSuiteEstS(s))),
      h("p.bench-suite-digest",
        h("code", "digest: " + (digest ? digest.slice(0, 12) : "unknown")),
        copyBtn(digest || "unknown", "Copy the digest"))));
}

/** benchSuiteCards is the section of the suites: one radio card each. */
function benchSuiteCards(list, picked, onPick) {
  if (!list || !list.length) {
    return emptyState("There is no suite to run.",
      "The built-in suites live in bench/suites/. One of them is the set of cells a pull request must pass.");
  }
  return h("div.bench-suites", list.map((s) => benchSuiteCard(s, s.id === picked, onPick)));
}

/** benchProgress is the live progress of a run: one sentence and one bar. */
function benchProgress() {
  const sentence = h("p.bench-progress-sentence", { "aria-live": "polite" }, "");
  const fill = h("i");
  const bar = h("div.bench-bar", {
    role: "progressbar", "aria-valuemin": "0", "aria-valuemax": "100", "aria-valuenow": "0",
  }, fill);
  const log = h("p.small.muted.bench-progress-log", "");
  const el = h("div.card.bench-card.bench-progress", { hidden: true }, sentence, bar, log);
  let start = Date.now();
  return {
    el: el,
    /** show writes the state of the job: the phase, the numbers and the bar. */
    show(job) {
      el.hidden = false;
      if (!start) start = Date.now();
      const done = job.done || 0;
      const total = job.total || 0;
      const pct = total ? Math.min(100, Math.round((done / total) * 100)) : 0;
      fill.style.width = pct + "%";
      bar.setAttribute("aria-valuenow", String(pct));
      const secs = Math.round((Date.now() - start) / 1000);
      const phase = job.phase || (total ? "Cell " + done + " of " + total : "Starting…");
      const rate = job.tok_s ? " · " + benchNum(job.tok_s, 1) + " tok/s" : "";
      sentence.textContent = phase + rate + " · " + secs + " s so far.";
    },
    /** log writes one line of the run under the sentence. */
    log(text) {
      if (text) log.textContent = String(text);
    },
    /** fail says that the run stopped and shows the last line. */
    fail(text) {
      el.hidden = false;
      sentence.textContent = "The run stopped.";
      log.textContent = String(text || "");
    },
    /** hide takes the card away. */
    hide() {
      el.hidden = true;
      start = 0;
    },
  };
}

/** benchBest returns the largest value a field of the cells of a result has. */
function benchBest(cells, get) {
  let best = 0;
  for (const c of cells || []) {
    const v = get(c);
    if (v != null && v > best) best = v;
  }
  return best > 0 ? best : null;
}

/** benchSuiteOf names the suite of a saved result. */
function benchSuiteOf(r) {
  if (r && r.suite && r.suite.id) return r.suite.id;
  return (r && r.tag) || "–";
}

/** benchHistoryRow is one saved result of the history table. */
function benchHistoryRow(r, verdict, onPick) {
  const cells = r.cells || [];
  const tr = h("tr",
    h("td.small", ago(r.time)),
    h("td", benchSuiteOf(r)),
    h("td.small", r.model || "–"),
    h("td.num", benchNum(benchBest(cells, (c) => c.output_tok_s), 1)),
    h("td.num", benchNum(benchBest(cells, (c) => (c.prefill || {}).agg_tok_s), 1)),
    h("td.num", benchNum(benchBest(cells, (c) => (c.decode || {}).agg_tok_s), 1)),
    h("td.num", cells.length),
    h("td", verdict ? verdictChip(verdict) : h("span.muted", "–")));
  if (onPick) {
    tr.classList.add("click");
    tr.addEventListener("click", () => onPick(r));
  }
  return tr;
}

/** benchHistory is the table of the saved results. */
function benchHistory(list, verdicts, onPick) {
  if (!list || !list.length) {
    return emptyState("No saved result yet.",
      "Every benchmark is saved with its workload, so two results can be compared only when they measured the same thing.");
  }
  return h("div.scroll", h("table.bench-table",
    h("thead", h("tr",
      h("th.l", "WHEN"), h("th.l", "SUITE"), h("th.l", "MODEL"),
      h("th.n", "OUT TOK/S"), h("th.n", "PREFILL"), h("th.n", "DECODE"),
      h("th.n", "CELLS"), h("th.l", "VERDICT"))),
    h("tbody", list.map((r) => benchHistoryRow(r, verdicts && verdicts[r.id], onPick)))));
}

/** benchAB is the A/B run: the two arms and the button that starts it. */
function benchAB(onRun) {
  const base = h("input", { type: "text", id: "bench-ab-base", placeholder: "flashnext" });
  const cand = h("input", { type: "text", id: "bench-ab-cand", placeholder: "http://127.0.0.1:8001" });
  const btn = h("button.primary", { type: "button" }, "Run the A/B");
  const out = h("div.bench-ab-out");
  const form = h("form.bench-ab",
    h("label", { for: "bench-ab-base" }, "Base"), base,
    h("label", { for: "bench-ab-cand" }, "Cand"), cand,
    btn);
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    if (!base.value.trim() || !cand.value.trim()) {
      put(out, h("p.small.muted", "An A/B run needs both arms: a recipe id or the URL of a server."));
      return;
    }
    onRun(base.value.trim(), cand.value.trim(), out, btn);
  });
  btn.addEventListener("click", () => form.requestSubmit());
  return h("div.card.bench-card",
    h("p.small.muted", "An A/B run measures one server, then the other, then the first again."),
    form, out);
}

/** benchAdvanced is the panel with the raw JSON of the endpoints of the tab
 *  and the commands that produce the same documents. */
function benchAdvanced(docs) {
  const body = h("div.bench-advanced-body", { hidden: true });
  for (const d of docs || []) {
    body.appendChild(h("div.bench-advanced-doc",
      h("p.bench-advanced-head",
        h("a.json-link", { href: d.path, target: "_blank", rel: "noopener noreferrer" }, "json"),
        h("code", d.path)),
      h("pre.bench-json", d.text || ""),
      copyBtn(d.text || "", "Copy the JSON")));
  }
  if (docs && docs.length) {
    body.appendChild(h("div.bench-advanced-cmds",
      h("p.eyebrow", "The same things in the command line"),
      cmdBlock("bernie bench list"),
      cmdBlock("bernie bench suites"),
      cmdBlock("bernie bench compare A B")));
  }
  const btn = h("button", { type: "button", "aria-expanded": "false" }, "Advanced");
  btn.addEventListener("click", () => {
    const open = body.hidden;
    body.hidden = !open;
    btn.setAttribute("aria-expanded", String(open));
  });
  return { el: h("div.card.bench-card.bench-advanced", btn, body), btn: btn };
}
