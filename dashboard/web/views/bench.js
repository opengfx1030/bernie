"use strict";
// ---------- Bench ----------

/** METRIC_HELP explains in one line how the prefill and the decode numbers are measured. */
const METRIC_HELP = "Prefill per request = prompt tokens ÷ time to first token (the queue time is in it); per cell = prompt tokens ÷ (first request → last first token). " +
  "Decode per request = (tokens − 1) ÷ (last token − first token); per cell = decode tokens ÷ the time any request was decoding.";

/** DIST_COLS are the per-request columns of a cell table. */
const DIST_COLS = [["mean", "req mean"], ["p50", "req p50"], ["p95", "req p95"], ["min", "req min"], ["max", "req max"]];

/** rateNum formats a rate or a latency. A number that was not measured shows as "–". */
function rateNum(v, digits) {
  if (v == null || v <= 0) return "–";
  return num(v, digits);
}

/** distNum formats one number of a distribution. A missing number shows as "–". */
function distNum(d, key, digits) {
  if (!d || d[key] == null) return "–";
  return rateNum(d[key], digits);
}

/** recallText names the pass key verdict of a cell in words. */
function recallText(v) {
  if (!v) return "–";
  return `${v.passed} of ${v.asked} passed`;
}

/** recallNote explains a cell whose replies did not hold the hidden fact. */
function recallNote(cells) {
  const bad = (cells || []).filter((c) => c.recall && c.recall.asked > 0 && c.recall.passed < c.recall.asked);
  if (!bad.length) return null;
  const cs = bad.map((c) => "c=" + c.concurrency).join(", ");
  return h("div.finding",
    h("div", h("b", "Recall failed"), ` in ${cs}: the model did not repeat the hidden fact in every reply.`),
    h("div.small.muted", "The server may not read the whole prompt. The speed numbers of that cell are meaningless until the recall passes."));
}

/** resultRecall totals the recall of the cells of a result. */
function resultRecall(r) {
  let asked = 0, passed = 0;
  for (const c of (r && r.cells) || []) {
    if (!c.recall) continue;
    asked += c.recall.asked;
    passed += c.recall.passed;
  }
  return asked ? { asked, passed } : null;
}

/** cellTable is the table of one cell: the prefill speed, the decode speed,
 *  the time to first token, the inter-token latency and the output rate. */
function cellTable(c) {
  const rows = [
    ["prefill tok/s", c.prefill ? c.prefill.agg_tok_s : null, c.prefill ? c.prefill.req : null, 1],
    ["decode tok/s", c.decode ? c.decode.agg_tok_s : null, c.decode ? c.decode.req : null, 1],
    ["TTFT s", null, c.ttft_s, 3],
    ["ITL ms", null, c.itl_ms, 2],
    ["output tok/s", c.output_tok_s, null, 1],
  ];
  return h("div.scroll", h("table",
    h("thead", h("tr", h("th", "metric"), h("th.num", "agg"), DIST_COLS.map(([, l]) => h("th.num", l)))),
    h("tbody", rows.map(([label, agg, d, digits]) => h("tr",
      h("td", label),
      h("td.num", rateNum(agg, digits)),
      DIST_COLS.map(([k]) => h("td.num", distNum(d, k, digits))))))));
}

/** serverNote says what the server itself measured over the same requests. */
function serverNote(c) {
  const s = c.server_side;
  if (!s) return null;
  const parts = [];
  if (s.ttft_s) parts.push(`TTFT s p50 ${rateNum(s.ttft_s.p50, 3)}, p95 ${rateNum(s.ttft_s.p95, 3)}`);
  if (s.itl_ms) parts.push(`ITL ms p50 ${rateNum(s.itl_ms.p50, 2)}, p95 ${rateNum(s.itl_ms.p95, 2)}`);
  if (!parts.length) return null;
  return h("div.small.muted", `Server side (${s.count || 0} requests): ${parts.join(" · ")}`);
}

/** cellCard is one cell of a result: its numbers in a table. */
function cellCard(c) {
  return h("div",
    h("h3", `Concurrency ${c.concurrency}`),
    h("div.small.muted", `${c.completed} of ${c.num_prompts} requests ok, ${c.failed} failed · wall ${rateNum(c.duration_s, 2)} s · ` +
      `${rateNum(c.tokens_per_chunk, 2)} tok/chunk · in/out ${rateNum(c.mean_prompt_tokens, 0)}/${rateNum(c.mean_output_tokens, 0)} tok`),
    cellTable(c), serverNote(c));
}

/** cellsView shows one table per cell. */
function cellsView(cells) {
  if (!cells || !cells.length) return h("div.empty", "No cells.");
  return h("div", cells.map(cellCard));
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
    h("label", { for: "b-recall" }, "Recall check"), f("recall", { type: "checkbox" }),
    h("div.hint", "Hide a short random fact in every prompt and check that the reply holds it."),
    h("div.full.row", h("button.primary", { type: "submit" }, "Run benchmark"), h("span.small.muted.status")));
    const result = h("div.card", cardHead("Result", "/api/bench"), h("div.empty", "Run a benchmark or pick one from the history."));
    const hist = h("div.card", cardHead("History", "/api/bench"), loading());
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const body = {};
    for (const el of form.querySelectorAll("input")) {
      if (el.type === "checkbox") { body[el.name] = el.checked; continue; }
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
      put(result, h("h3", "Result " + r.id), h("div.small.muted", `${r.model} @ ${r.url}`), h("div.small.muted", METRIC_HELP), cellsView(r.cells), recallNote(r.cells));
      st.textContent = "done";
      loadHistory(hist, result);
    } catch (err) { st.textContent = ""; put(result, failed(err)); }
    btn.disabled = false;
  });
  put(root, h("div.cols", h("div.card", cardHead("Run benchmark", "/api/bench"), form), hist), h("div.gap"), result);
  loadHistory(hist, result);
}

/** benchGPUText names the GPUs used by a saved result. */
function benchGPUText(r) {
  const fp = (r && r.fingerprint) || {};
  if (!fp.gpus) return "–";
  return fp.gpu_scope ? `${fp.gpus} (${fp.gpu_scope})` : fp.gpus;
}

/** benchRow is one saved result in the history table. */
function benchRow(r, result) {
  const cells = r.cells || [];
  return h("tr.click", {
    onclick: () => put(result, h("h3", "Result " + r.id), h("div.small.muted", `${r.model} @ ${r.url} · ${new Date(r.time).toLocaleString()}`), h("div.small.muted", METRIC_HELP), cellsView(cells), recallNote(cells)),
  },
    h("td.small", ago(r.time)), h("td", r.tag || ""), h("td", r.model),
    h("td.small", benchGPUText(r)),
    h("td.num", cells.map((c) => c.concurrency).join(",")),
    h("td.num", rateNum(benchBest(cells, (c) => c.output_tok_s), 1)),
    h("td.num", rateNum(benchBest(cells, (c) => (c.decode || {}).agg_tok_s), 1)),
    h("td.num", rateNum(benchBest(cells, (c) => (c.prefill || {}).agg_tok_s), 1)),
    h("td.num", recallText(resultRecall(r))));
}

async function loadHistory(card, result) {
  try {
    const d = await api("/api/bench");
    const list = (Array.isArray(d) ? d : (d && d.results) || []).slice().sort((a, b) => new Date(b.time) - new Date(a.time));
    if (!list.length) { put(card, cardHead("History", "/api/bench"), h("div.empty", "No saved results.")); return; }
    put(card, cardHead("History", "/api/bench"), h("div.scroll", h("table",
      h("thead", h("tr", h("th", "When"), h("th", "Tag"), h("th", "Model"), h("th", "GPUs"), h("th.num", "Conc."),
        h("th.num", "Best output tok/s"), h("th.num", "Best decode tok/s"), h("th.num", "Best prefill tok/s"), h("th.num", "Recall"))),
      h("tbody", list.map((r) => benchRow(r, result))))));
  } catch (e) { put(card, h("h2", "History"), failed(e)); }
}
