"use strict";
// ---------- Bench ----------

const CELL_COLS = [
  ["concurrency", "Conc."], ["completed", "OK"], ["failed", "Fail"],
  ["ttft_p50_ms", "TTFT p50 ms"], ["ttft_p95_ms", "TTFT p95 ms"], ["itl_p50_ms", "ITL p50 ms"], ["itl_p95_ms", "ITL p95 ms"],
  ["decode_tok_s", "Decode tok/s"], ["prefill_tok_s", "Prefill tok/s"], ["output_tok_s", "Output tok/s"], ["duration_s", "Duration s"],
  ["num_prompts", "Prompts"], ["mean_prompt_tokens", "In tok"], ["mean_output_tokens", "Out tok"], ["tokens_per_chunk", "Tok/chunk"],
  ["recall", "Recall"],
];

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

function cellsTable(cells) {
  if (!cells || !cells.length) return h("div.empty", "No cells.");
  const known = new Set(CELL_COLS.map((c) => c[0]));
  const extra = Object.keys(cells[0]).filter((k) => !known.has(k) && typeof cells[0][k] !== "object");
  const cols = CELL_COLS.filter(([k]) => k in cells[0]).concat(extra.map((k) => [k, k]));
  const fmt = (k, v) => {
    if (k === "recall") return recallText(v);
    return typeof v === "number" && /^itl/.test(k) ? v.toFixed(2) : num(v);
  };
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
    h("label", { for: "b-recall" }, "Recall check"), f("recall", { type: "checkbox" }),
    h("div.hint", "Hide a short random fact in every prompt and check that the reply holds it."),
    h("div.full.row", h("button.primary", { type: "submit" }, "Run benchmark"), h("span.small.muted.status")));
  const result = h("div.card", h("h2", "Result"), h("div.empty", "Run a benchmark or pick one from the history."));
  const hist = h("div.card", h("h2", "History"), loading());
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
      put(result, h("h3", "Result " + r.id), h("div.small.muted", `${r.model} @ ${r.url}`), cellsTable(r.cells), recallNote(r.cells));
      st.textContent = "done";
      loadHistory(hist, result);
    } catch (err) { st.textContent = ""; put(result, failed(err)); }
    btn.disabled = false;
  });
  put(root, h("div.cols", h("div.card", h("h2", "Run benchmark"), form), hist), h("div.gap"), result);
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
    onclick: () => put(result, h("h3", "Result " + r.id), h("div.small.muted", `${r.model} @ ${r.url} · ${new Date(r.time).toLocaleString()}`), cellsTable(cells), recallNote(cells)),
  },
    h("td.small", ago(r.time)), h("td", r.tag || ""), h("td", r.model),
    h("td.small", benchGPUText(r)),
    h("td.num", cells.map((c) => c.concurrency).join(",")),
    h("td.num", num(Math.max(0, ...cells.map((c) => c.output_tok_s || 0)))),
    h("td.num", recallText(resultRecall(r))));
}

async function loadHistory(card, result) {
  try {
    const d = await api("/api/bench");
    const list = (Array.isArray(d) ? d : (d && d.results) || []).slice().sort((a, b) => new Date(b.time) - new Date(a.time));
    if (!list.length) { put(card, h("h2", "History"), h("div.empty", "No saved results.")); return; }
    put(card, h("h2", "History"), h("div.scroll", h("table",
      h("thead", h("tr", h("th", "When"), h("th", "Tag"), h("th", "Model"), h("th", "GPUs"), h("th.num", "Conc."), h("th.num", "Best output tok/s"), h("th.num", "Recall"))),
      h("tbody", list.map((r) => benchRow(r, result))))));
  } catch (e) { put(card, h("h2", "History"), failed(e)); }
}
