// The recipe chooser and the plan of the picked recipe.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
"use strict";

/** loadSetupRecipes reads the recipes of the host and paints the chooser. */
async function loadSetupRecipes() {
  const box = setupState.recipeBox;
  if (!box) return;
  let list = null;
  try {
    list = await api("/api/recipes");
  } catch (e) {
    put(box, h("p", "The recipes could not be read."),
      h("p", String(e.message || e)));
    return;
  }
  setupState.recipes = list;
  const recipes = list.recipes || [];
  const errors = list.errors || [];
  if (!recipes.length) {
    put(box, emptyState("There is no recipe to serve.",
      "A recipe is a model, the flags it needs and the way it is served. The recipes live in the recipes/ directory of the repository."));
  } else {
    // The recipe bernie recommends comes first, with its name and what it
    // is. The other recipes wait behind "Choose another".
    put(box, setupRecommendedCard(recipes));
  }
  if (!setupState.recipeID || !recipes.some((r) => r.id === setupState.recipeID)) {
    const best = setupRecommendedRecipe(recipes);
    setupState.recipeID = best ? best.id : (recipes[0] ? recipes[0].id : "");
  }
  await loadSetupPlan();
}

/** setupRecipeOrder sorts the recipes: the one that fits this host first,
 *  then the ones this host cannot tell, then the ones that do not fit. */
function setupRecipeOrder(recipes) {
  const rank = (r) => {
    const fit = setupFitStatus(r);
    if (fit === "fits") return 0;
    if (fit === "unknown") return 1;
    return 2;
  };
  return recipes.slice().sort((a, b) => {
    const d = rank(a) - rank(b);
    if (d) return d;
    return (b.priority || 0) - (a.priority || 0);
  });
}

/** setupRecipeError is one recipe file that did not load. */
function setupRecipeError(error) {
  return h("div.recipe-error", h("span.recipe-error-source", error.source || "recipe"),
    h("span.recipe-error-text", error.error || error.err || "the file did not load"));
}

/** setupRecommendedCard is the recipe bernie recommends for this host: its
 *  name, what it is and why it is the one. The other recipes wait behind
 *  "Choose another". */
function setupRecommendedCard(recipes) {
  const best = setupRecommendedRecipe(recipes);
  const others = setupRecipeOrder(recipes).filter((r) => !best || r.id !== best.id);
  const list = h("div.recipe-chooser");
  const out = [];
  if (best) {
    out.push(h("div.recipe-card.picked", h("h3", setupRecipeLabel(best)),
      h("p.recipe-fit", { class: "recipe-fit fit-" + setupFitStatus(best) }, setupFitLine(best))));
    out.push(h("p.small.muted", setupRecommendationWhy(best)));
  }
  if (others.length) {
    out.push(h("details.advanced", {
      id: "setup-recipes-other",
      ontoggle: (ev) => {
        if (!ev.target.open || list.childElementCount) return;
        // The list of the recipes comes with the disclosure that opens it.
        setupEnsureModule("views/setup/chooser.js")
          .then(() => setupPaintRecipeList(list, others))
          .catch((e) => put(list, h("p", "The list could not be read: " + (e.message || e))));
      },
    },
      h("summary", "Choose another recipe (" + others.length + ")"),
      list));
  }
  return h("div", out);
}
/** setupRecommendedRecipe is the recipe bernie recommends: the one the
 *  checklist names (spec §3.3), else the one that fits this host. */
function setupRecommendedRecipe(recipes) {
  const rec = setupState.doc && setupState.doc.recommendation;
  const list = recipes || (setupState.recipes && setupState.recipes.recipes) || [];
  if (rec && rec.id) {
    const found = list.find((r) => r.id === rec.id);
    if (found) return found;
  }
  return list.find((r) => setupFitStatus(r) === "fits") || null;
}
/** setupFitStatus is the fit of a recipe: fits, no or unknown. */
function setupFitStatus(recipe) {
  return (recipe.fit && recipe.fit.status) || "unknown";
}
/** setupFitLine is the fit of a recipe in one line, with the reason. */
function setupFitLine(recipe) {
  const status = setupFitStatus(recipe);
  const reasons = (recipe.fit && recipe.fit.reasons) || [];
  if (status === "unknown") return "fit: unknown - No GPU data on this host.";
  if (status === "no") return "no: " + (reasons.join("; ") || "this recipe does not fit this host.");
  const need = (recipe.arch || []).join(", ") || "any architecture";
  const gpus = recipe.gpus ? recipe.gpus + " GPUs" : "any number of GPUs";
  let line = "fits: " + need + ", " + gpus;
  if (reasons.length) line += " - " + reasons.join("; ");
  return line;
}
/** loadSetupPlan reads the plan of the picked recipe and paints it. */
async function loadSetupPlan() {
  const box = setupState.planBox;
  if (!box || !setupState.recipeID) return;
  const recipe = setupRecipe();
  put(box, loading());
  const query = new URLSearchParams();
  query.set("mode", setupState.mode);
  for (const set of setupState.sets) query.append("set", set);
  let plan = null;
  try {
    plan = await api("/api/recipes/" + encodeURIComponent(setupState.recipeID) + "/plan?" + query.toString());
  } catch (e) {
    put(box, h("h3", setupState.recipeID),
      h("p", "The plan could not be built."),
      h("p", String(e.message || e)),
      h("button", { type: "button", onclick: loadSetupPlan }, "Try again"));
    return;
  }
  setupState.plan = plan;
  put(box, setupPlanView(plan, recipe));
}
/** setupRecipe is the picked recipe of the listing. */
function setupRecipe() {
  const recipes = (setupState.recipes && setupState.recipes.recipes) || [];
  return recipes.find((r) => r.id === setupState.recipeID) || null;
}
/** setupPlanView is the plan card: the recipe, its fit, the mode, the
 *  command, the cards it uses, the settings with their reasons and the
 *  button that starts the server. */
function setupPlanView(plan, recipe) {
  const modes = (recipe && recipe.modes) || [plan.mode || "host"];
  const card = h("div.recipe-plan",
    h("div.recipe-plan-head",
      h("h3", (recipe && (recipe.name || recipe.id)) || plan.recipe || "Plan"),
      h("div.segmented-control", { role: "group", "aria-label": "The mode of the server" },
        modes.map((m) => h("button", {
          type: "button",
          class: m === setupState.mode ? "active" : "",
          "aria-pressed": String(m === setupState.mode),
          onclick: () => setupSwitchMode(m),
        }, m)))),
    h("p.recipe-fit", { class: "recipe-fit fit-" + setupFitStatus(recipe) }, recipe ? setupFitLine(recipe) : "fit: unknown"),
    setupPlanOverrides(recipe),
    (plan.warnings || []).map((w) => h("p.recipe-warning", w)),
    h("h4", "The launch command"),
    cmdBlock(setupPlanCommand(plan), "Copy the launch command"),
    h("div.recipe-gpus",
      h("span.recipe-gpus-label", "GPUs"),
      h("span.recipe-gpus-value", (plan.gpus || []).length ? (plan.gpus || []).join(", ") : "–"),
      plan.gpu_select_reason ? h("span.recipe-gpus-why", plan.gpu_select_reason) : null),
    setupPlanSettings(plan),
    setupPlanAdjustments(plan),
    setupPlanActions(recipe, plan));
  return card;
}
/** setupPlanActions is the command of the plan and the copy button. The
 *  start of the plan is the one primary action of the tab, in the hero. */
function setupPlanActions(recipe, plan) {
  const ready = setupPlanReady(recipe);
  return h("div.recipe-plan-actions",
    h("button", { type: "button", onclick: setupCopyText.bind(null, setupPlanCommand(plan)) }, "Copy the launch command"),
    ready ? null : h("span.small.muted", "The plan is not ready: " + setupFitLine(recipe)));
}
/** setupPlanReady says whether the plan can be started here. */
function setupPlanReady(recipe) {
  return setupFitStatus(recipe) === "fits";
}
/** setupPlanCommand is the exact command of the plan. */
function setupPlanCommand(plan) {
  if (plan.command) return plan.command;
  const exec = plan.exec || {};
  if (Array.isArray(exec.argv) && exec.argv.length) return exec.argv.map(setupQuote).join(" ");
  return "bernie serve " + (plan.recipe || setupState.recipeID) + " --mode " + (plan.mode || setupState.mode);
}
/** setupQuote makes one argument safe for a terminal. */
function setupQuote(arg) {
  const text = String(arg == null ? "" : arg);
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(text) ? text : "'" + text.replace(/'/g, "'\\''") + "'";
}
/** setupPlanSettings lists every setting of the plan with its reason. */
function setupPlanSettings(plan) {
  const settings = plan.settings || [];
  if (!settings.length) return null;
  return h("div.recipe-settings",
    h("h4", "The settings"),
    settings.map((s) => h("div.recipe-setting",
      h("span.recipe-setting-name", s.setting),
      h("span.recipe-setting-value", s.value == null || s.value === "" ? "–" : String(s.value)),
      s.why ? h("span.recipe-setting-why", s.why) : null)));
}
/** setupPlanAdjustments lists what the plan changed to fit this host. */
function setupPlanAdjustments(plan) {
  const adjustments = plan.adjustments || [];
  if (!adjustments.length) return null;
  return h("div.recipe-adjustments",
    h("h4", "What the plan changed"),
    adjustments.map((a) => {
      if (typeof a === "string") return h("div.recipe-adjustment", a);
      const what = a.what || a.setting || a.path || "";
      return h("div.recipe-adjustment",
        h("span.recipe-adjustment-what", what),
        a.why ? h("span.recipe-adjustment-why", a.why) : null);
    }));
}
/** setupPlanOverrides are the inputs of the recipe and the extra settings
 *  the operator wants. A change builds the plan again. */
function setupPlanOverrides(recipe) {
  if (!recipe) return null;
  const inputs = recipe.inputs || {};
  const fields = Object.keys(inputs).sort().map((name) => {
    const input = inputs[name];
    const field = h("input", {
      type: "text",
      name: name,
      value: setupSetValue(name, input.default || ""),
      placeholder: input.path ? "/absolute/path" : "",
      required: !!input.required,
      onchange: () => setupApplyOverrides(),
    });
    return h("label.recipe-input", h("span.recipe-input-name", name + (input.required ? " *" : "")), field);
  });
  const extra = h("textarea", {
    rows: 2,
    placeholder: "args.max-model-len=32768",
    onchange: () => setupApplyOverrides(),
  });
  extra.value = setupExtraSets(inputs).join("\n");
  return h("div.recipe-overrides", fields,
    h("label.recipe-input", h("span.recipe-input-name", "Overrides"), extra));
}
/** setupSetValue is the value of one input of the recipe. */
function setupSetValue(name, fallback) {
  const prefix = name + "=";
  const found = setupState.sets.find((s) => s.startsWith(prefix));
  return found ? found.slice(prefix.length) : fallback;
}
/** setupExtraSets are the overrides that are not an input of the recipe. */
function setupExtraSets(inputs) {
  const names = Object.keys(inputs || {});
  return setupState.sets.filter((s) => !names.some((n) => s.startsWith(n + "=")));
}
/** setupApplyOverrides reads the fields of the plan and builds it again. */
async function setupApplyOverrides() {
  const box = setupState.planBox;
  if (!box) return;
  const sets = [];
  const fields = box.querySelectorAll(".recipe-overrides input");
  fields.forEach((field) => {
    if (field.value !== "") sets.push(field.name + "=" + field.value);
  });
  const extra = box.querySelector(".recipe-overrides textarea");
  if (extra) {
    for (const line of extra.value.split("\n")) {
      if (line.trim()) sets.push(line.trim());
    }
  }
  setupState.sets = sets;
  await loadSetupPlan();
}
/** setupSwitchMode changes the mode of the plan and builds it again. */
async function setupSwitchMode(mode) {
  if (mode === setupState.mode) return;
  setupState.mode = mode;
  await loadSetupPlan();
}
/** startSetupRecipe starts the server of the picked recipe. */
async function startSetupRecipe() {
  const recipe = setupRecipe();
  if (!recipe) return;
  try {
    const run = await api("/api/runs", {
      method: "POST",
      body: { recipe: recipe.id, set: setupState.sets, mode: setupState.mode },
    });
    toast("Started " + (run.name || recipe.id) + ".", true);
    if (setupState.onRefresh) setupState.onRefresh();
  } catch (e) {
    toast("Start failed: " + (e.message || e));
  }
}
/** paintSetupRecipeHint says what to do when no recipe fits this host: the
 *  reason, the next step and the command (Setup rule 11). */
function paintSetupRecipeHint() {
  const box = setupState.recipeHint;
  if (!box) return;
  const list = (setupState.recipes && setupState.recipes.recipes) || [];
  if (!list.length) {
    put(box);
    return;
  }
  const fits = list.filter((r) => setupFitStatus(r) === "fits");
  if (fits.length) {
    put(box);
    return;
  }
  const why = list.map(setupFitLine).filter(Boolean).slice(0, 2).join(" ");
  put(box, emptyState("No recipe fits this host yet.",
    why + " A recipe is a model and the way it is served. Fix the steps above, then press Re-check.",
    "bernie recipe list --all"));
}

/** setupRecommendationWhy is the one sentence that says why this recipe is
 *  the recommended one. The words come from the checklist (spec §3.3), so
 *  the CLI, the MCP server and the page say the same thing. */
function setupRecommendationWhy(recipe) {
  const rec = setupState.doc && setupState.doc.recommendation;
  if (rec && rec.id === recipe.id && rec.why) return rec.why + ".";
  return setupFitLine(recipe);
}

/** setupRecipeNote is the line under the recipe step of the checklist: the
 *  recommended choice and why it is recommended. */
function setupRecipeNote() {
  const rec = setupState.doc && setupState.doc.recommendation;
  if (!rec) return null;
  return "Recommended: " + setupRecipeLabel(rec) + " - " + rec.why + ".";
}

/** setupRecipeLabel is the human name of a recipe and the one line that
 *  says what it is. The id is for the terminal, not for the reader. */
function setupRecipeLabel(recipe) {
  const name = recipe.name || recipe.id;
  return recipe.description ? name + " - " + recipe.description : name;
}
