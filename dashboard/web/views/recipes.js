"use strict";
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
