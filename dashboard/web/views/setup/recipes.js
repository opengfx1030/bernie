// The recipe chooser and the plan of the picked recipe.
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
    put(box, h("div.recipe-chooser", recipes.map(setupRecipeCard)));
  }
  if (setupState.recipeErrors) {
    put(setupState.recipeErrors, errors.length
      ? [h("h3", "LOAD ERRORS"), errors.map(setupRecipeError)]
      : null);
  }
  if (!setupState.recipeID || !recipes.some((r) => r.id === setupState.recipeID)) {
    const best = recipes.find((r) => r.fit && r.fit.status === "fits");
    setupState.recipeID = best ? best.id : (recipes[0] ? recipes[0].id : "");
  }
  setupPaintRecipeSelection();
  await loadSetupPlan();
}

/** setupRecipeCard is one radio card of the chooser: the name, the status,
 *  the cards it wants, its architecture and its fit. */
function setupRecipeCard(recipe) {
  const input = h("input", {
    type: "radio",
    name: "setup-recipe",
    value: recipe.id,
    checked: recipe.id === setupState.recipeID,
    onchange: () => setupSelectRecipe(recipe.id),
  });
  return h("label.recipe-card", { class: "recipe-card" + (recipe.id === setupState.recipeID ? " picked" : "") },
    input,
    h("h3", recipe.name || recipe.id),
    h("div.recipe-meta",
      h("span.recipe-status", recipe.status || ""),
      h("span.recipe-need", recipe.gpus ? recipe.gpus + " GPUs" : "any number of GPUs"),
      h("span.recipe-arch", (recipe.arch || []).join(", ") || "any architecture")),
    h("p.recipe-fit", { class: "recipe-fit fit-" + setupFitStatus(recipe) }, setupFitLine(recipe)));
}

/** setupRecipeError is one recipe file that did not load. */
function setupRecipeError(error) {
  return h("div.recipe-error", h("span.recipe-error-source", error.source || "recipe"),
    h("span.recipe-error-text", error.error || error.err || "the file did not load"));
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

/** setupSelectRecipe picks a recipe and builds its plan again. */
async function setupSelectRecipe(id) {
  setupState.recipeID = id;
  const recipe = (setupState.recipes && setupState.recipes.recipes || []).find((r) => r.id === id);
  const modes = (recipe && recipe.modes) || ["host"];
  setupState.mode = modes.includes(setupState.mode) ? setupState.mode : modes[0];
  setupState.sets = [];
  setupPaintRecipeSelection();
  await loadSetupPlan();
}

/** setupPaintRecipeSelection marks the picked recipe in the chooser. */
function setupPaintRecipeSelection() {
  const cards = setupState.recipeBox ? setupState.recipeBox.querySelectorAll(".recipe-card") : [];
  cards.forEach((card) => {
    const input = card.querySelector("input");
    const picked = input && input.value === setupState.recipeID;
    card.classList.toggle("picked", picked);
    if (input) input.checked = picked;
  });
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
    h("div.recipe-plan-actions",
      h("button", {
        type: "button",
        class: setupState.primaryOwner === "plan" ? "primary" : "",
        onclick: startSetupRecipe,
      }, "Start the server")));
  return card;
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
