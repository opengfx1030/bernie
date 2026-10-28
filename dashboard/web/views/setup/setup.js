// The Setup tab answers one question: what do I do to make this box serve?
// It reads the onboarding checklist, the doctor report and the recipes, and
// it never changes the host on its own.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
"use strict";

// SETUP_EXTRA_STEPS are the steps that come after the path of the host.
const SETUP_EXTRA_STEPS = ["bench", "tuning", "power"];

// SETUP_STEP_NAMES name a step of the path in one plain phrase.
const SETUP_STEP_NAMES = {
  hardware: "the cards",
  access: "the device groups",
  rocm: "the ROCm tools",
  "python-env": "the Python environment",
  model: "the model",
  recipe: "a recipe",
  serve: "the server",
  verify: "the server check",
};

// SETUP_SMOKE_ENABLED says whether the endpoint that answers one prompt
// exists. The backend landed POST /api/smoke with the shape of spec §11.
const SETUP_SMOKE_ENABLED = true;

// setupState holds the documents of the tab and the nodes it paints.
const setupState = {
  doc: null,
  doctor: null,
  recipes: null,
  hw: null,
  plan: null,
  recipeID: "",
  mode: "host",
  sets: [],
  head: null,
  actions: null,
  banner: null,
  hero: null,
  timeline: null,
  recipeBox: null,
  recipeErrors: null,
  planBox: null,
  doctorBox: null,
  advanced: null,
  primaryOwner: "headline",
  generation: 0,
  onRefresh: null,
};

/** viewSetup draws the tab: the headline, the plan, the checklist, the
 *  recipes, the doctor report and the raw documents. */
async function viewSetup(root) {
  setupState.generation++;
  const gen = setupState.generation;
  setupStopStreams();

  const head = stateHead("Setup");
  const actions = h("div.head-actions");
  head.append(actions);
  const banner = h("div.banner.warn", { hidden: true });
  const hero = h("div.setup-hero");
  const timeline = h("div.setup-timeline-body");
  const recipeBox = h("div.setup-recipe-chooser");
  const recipeErrors = h("div.recipe-errors");
  const planBox = h("div.recipe-plan-box");
  const doctorBox = h("div.doctor-findings");
  const advanced = setupAdvancedPanel();

  Object.assign(setupState, {
    head, actions, banner, hero, timeline, recipeBox, recipeErrors, planBox, doctorBox, advanced,
    onRefresh: () => loadSetupDocument(),
  });

  put(root, head, banner, hero,
    h("section.section", { id: "setup-path" },
      h("h2.section-label", "THE PATH TO A SERVING MODEL"),
      timeline),
    h("section.section", { id: "setup-recipes" },
      h("h2.section-label", "THE RECIPE"),
      recipeBox, recipeErrors, planBox),
    h("section.section", { id: "setup-doctor" },
      h("h2.section-label", "DOCTOR", h("span.grow"),
        h("button.small", { type: "button", onclick: loadSetupDoctor }, "Re-run")),
      doctorBox),
    advanced);

  setHead(head, "BERNIE IS CHECKING THE HOST.", "Reading the host. The checklist arrives in a moment.");
  put(hero, loading());
  put(timeline, loading());
  put(recipeBox, loading());
  put(doctorBox, loading());

  await Promise.all([loadSetupDocument(gen), loadSetupRecipes(gen), loadSetupDoctor(gen)]);
}

/** loadSetupDocument reads the onboarding checklist and paints the
 *  headline, the facts and the timeline. */
async function loadSetupDocument(gen) {
  if (gen && gen !== setupState.generation) return;
  let doc = null;
  try {
    doc = await api("/api/onboarding");
  } catch (e) {
    if (!gen || gen === setupState.generation) paintSetupError(e);
    return;
  }
  if (gen && gen !== setupState.generation) return;
  setupState.doc = doc;
  paintSetupHead(doc);
  paintSetupStale(doc);
  paintSetupTimeline(doc);
  await paintSetupFacts(doc);
  attachSetupJobs(doc.steps);
}

/** paintSetupHead writes the state of the machine in one sentence and the
 *  actions that follow from it. */
function paintSetupHead(doc) {
  const say = setupHeadline(doc);
  setupState.primaryOwner = say.action === "doctor" ? "plan" : "headline";
  setHead(setupState.head, say.title, say.lede);
  put(setupState.actions, setupHeadActions(doc, say));
}

/** setupHeadline says what the machine is in one sentence, with the lede
 *  under it naming the single next action and who does it. */
function setupHeadline(doc) {
  const steps = doc.steps || [];
  const path = steps.filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
  const pending = path.filter((s) => s.state !== "done" && s.state !== "skipped");
  const running = path.find((s) => s.state === "running");
  const hardware = steps.find((s) => s.id === "hardware");
  const serve = steps.find((s) => s.id === "serve");
  if (setupServerServing(serve)) {
    return {
      title: "THIS BOX IS SERVING.",
      lede: "The model answers on " + setupServerEndpoint(serve) + ". Try it, or read the logs.",
      action: "try",
    };
  }
  if (running) {
    const phase = (running.progress && running.progress.phase) || running.summary || "bernie is working on the next step.";
    return { title: "SETTING UP THIS BOX.", lede: phase, action: "continue" };
  }
  if (!pending.length) {
    return {
      title: "THIS BOX IS READY TO SERVE.",
      lede: "Everything bernie needs is in place. Start the server and it answers on " + setupServerEndpoint(serve) + ".",
      action: "start",
    };
  }
  if (hardware && hardware.state === "blocked") {
    return {
      title: "THIS BOX HAS NO GPU FOR BERNIE.",
      lede: "bernie needs an AMD card with /dev/kfd. The doctor report says what is missing.",
      action: "doctor",
    };
  }
  const rootClause = pending.some((s) => (s.root_commands || []).length)
    ? " Copy the command, run it as root in a terminal, then press Re-check."
    : "";
  const runnable = path.find((s) => s.state === "todo" && s.can_run);
  let lede = rootClause;
  if (runnable) {
    const verb = (stepActionLabel(runnable, false) || "").charAt(0).toLowerCase() + (stepActionLabel(runnable, false) || "").slice(1);
    lede = "Next: " + verb + ". bernie does it here." + rootClause;
  } else if (!rootClause) {
    lede = "The doctor report says what is missing.";
  }
  return {
    title: pending.length + (pending.length === 1 ? " STEP LEFT" : " STEPS LEFT") + " BEFORE THIS BOX CAN SERVE.",
    lede: lede,
    action: "setup",
  };
}

/** setupServerEndpoint is where the server answers. */
function setupServerEndpoint(serve) {
  if (serve && serve.detail && /^https?:\/\//.test(serve.detail)) return serve.detail;
  const plan = setupState.plan;
  if (plan && plan.port) return "port " + plan.port;
  return "the port of the plan";
}

/** setupHeadActions are the actions of the tab. The filled button names
 *  the step it starts: never only "Run" or "Next". */
function setupHeadActions(doc, say) {
  const out = [];
  if (say.action === "doctor") {
    out.push(h("button", { type: "button", onclick: focusSetupDoctor }, "Read the doctor report"));
    return out;
  }
  if (say.action === "continue") {
    out.push(h("button.primary", { type: "button", onclick: setupRunNextStep }, "Run the rest of the steps"));
    out.push(h("button", { type: "button", onclick: () => loadSetupDocument() }, "Re-check"));
    return out;
  }
  if (say.action === "start") {
    out.push(h("button.primary", { type: "button", onclick: startSetupServer }, "Start the server"));
    out.push(h("button", { type: "button", onclick: setupToggleAdvanced }, "Advanced"));
    return out;
  }
  if (say.action === "try") {
    out.push(h("button.primary", { type: "button", onclick: focusSetupVerify }, "Try it"));
    out.push(h("button", { type: "button", onclick: () => { location.hash = "#logs"; } }, "Read the logs"));
    return out;
  }
  const next = setupNextRunnable(doc);
  const rootNext = !next && setupNextRoot(doc);
  if (next) {
    out.push(h("button.primary", {
      type: "button",
      onclick: () => runSetupStep(next, { onRefresh: () => loadSetupDocument() }),
    }, stepActionLabel(next, false)));
  } else if (rootNext) {
    out.push(h("button.primary", {
      type: "button",
      onclick: () => { setupCopyText(rootNext.command); focusSetupStep(rootNext.id); },
    }, "Copy the root command"));
    out.push(h("button", { type: "button", onclick: setupCopyNextCommand }, "Copy the next command"));
    out.push(h("button", { type: "button", onclick: setupToggleAdvanced }, "Advanced"));
    return out;
  } else {
    out.push(h("button.primary", { type: "button", onclick: focusSetupDoctor }, "Read the doctor report"));
    out.push(h("button", { type: "button", onclick: setupToggleAdvanced }, "Advanced"));
    return out;
  }
  out.push(h("button", { type: "button", onclick: setupCopyNextCommand }, "Copy the command"));
  out.push(h("button", { type: "button", onclick: setupToggleAdvanced }, "Advanced"));
  return out;
}

/** setupNextRunnable is the next step bernie can do itself. */
function setupNextRunnable(doc) {
  const steps = (doc && doc.steps) || [];
  const path = steps.filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
  return path.find((s) => s.state === "todo" && s.can_run) || null;
}

/** setupNextRoot is the next step that needs root. */
function setupNextRoot(doc) {
  const steps = (doc && doc.steps) || [];
  const path = steps.filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
  return path.find((s) => s.state === "blocked" && (s.root_commands || []).length) || null;
}

/** setupRunNextStep starts the next step bernie can do. */
function setupRunNextStep() {
  const doc = setupState.doc;
  const next = setupNextRunnable(doc);
  if (!next) {
    toast("Nothing else can run right now.");
    return;
  }
  runSetupStep(next, { onRefresh: () => loadSetupDocument() });
}

/** setupExplainWait says what a later step waits for. */
function setupExplainWait(step) {
  const doc = setupState.doc;
  if (!doc) return;
  const currentID = doc.next || (pathSteps(doc)[0] || {}).id || "";
  const current = (doc.steps || []).find((s) => s.id === currentID);
  const name = current ? stepActionLabel(current, false) : "the step before it";
  toast(name + " first.");
}

/** pathSteps is the path of the checklist, with the extras removed. */
function pathSteps(doc) {
  return (doc && doc.steps || []).filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
}

/** setupCopyNextCommand copies the command of the next step. */
function setupCopyNextCommand() {
  const doc = setupState.doc;
  const next = doc && (doc.steps || []).find((s) => s.id === doc.next);
  const text = (next && next.command) || setupPlanCommand(setupState.plan || {});
  setupCopyText(text);
}

/** setupCopyText puts a command on the clipboard and says that it worked. */
async function setupCopyText(text) {
  if (!text) {
    toast("There is nothing to copy.");
    return;
  }
  let ok = false;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      ok = true;
    }
  } catch (e) { ok = false; }
  if (!ok) {
    try {
      const area = h("textarea");
      // The styles are set through the CSSOM, never as an inline style
      // attribute: the page may not carry inline styles.
      area.style.position = "fixed";
      area.style.left = "-9999px";
      area.style.top = "0";
      area.value = text;
      document.body.append(area);
      area.select();
      ok = document.execCommand("copy");
      area.remove();
    } catch (e) { ok = false; }
  }
  toast(ok ? "Copied the command." : "Copy failed: select the text and press Ctrl-C.", ok);
}

/** startSetupServer starts the server of the path. */
function startSetupServer() {
  const doc = setupState.doc;
  const serve = doc && (doc.steps || []).find((s) => s.id === "serve");
  if (serve && serve.can_run) {
    runSetupStep(serve, { onRefresh: () => loadSetupDocument() });
    return;
  }
  startSetupRecipe();
}

/** paintSetupStale shows the banner of a checklist that is older than its
 *  time to live. */
function paintSetupStale(doc) {
  const banner = setupState.banner;
  if (!banner) return;
  if (doc.stale) {
    put(banner, "bernie is checking the host again. The values below are the last known state.");
    banner.hidden = false;
  } else {
    put(banner);
    banner.hidden = true;
  }
}

/** paintSetupError says that the state of the machine is unknown. */
function paintSetupError(e) {
  setHead(setupState.head, "THE STATE OF THIS MACHINE IS UNKNOWN.", "The dashboard could not read /api/onboarding.");
  put(setupState.actions, h("button.primary", { type: "button", disabled: true, "aria-disabled": "true" }, "Try again"));
  put(setupState.hero);
  put(setupState.timeline, h("div.setup-error",
    h("p", "The dashboard could not read the checklist."),
    h("p", String(e.message || e)),
    h("button", { type: "button", onclick: () => loadSetupDocument() }, "Try again")));
}

/** paintSetupTimeline draws the checklist of the host. */
function paintSetupTimeline(doc) {
  if (!setupState.timeline) return;
  if (!doc) doc = setupState.doc;
  if (!doc) return;
  const steps = doc.steps || [];
  for (const step of steps) {
    if (step.progress && !setupProgress[step.id]) {
      const state = { phase: step.progress.phase || "", started: Date.now() };
      const total = step.progress.total || 0;
      if (total) {
        const p = { phase: step.progress.phase, files_total: total, files_done: step.progress.done || 0 };
        setupApplyJobProgress(state, p);
      }
      setupProgress[step.id] = state;
    }
  }
  put(setupState.timeline, stepTimeline(steps, {
    extraIDs: SETUP_EXTRA_STEPS,
    nextID: doc.next,
    onRun: runSetupStep,
    onRefresh: () => loadSetupDocument(),
    onWait: setupExplainWait,
    noteFor: setupStepNote,
    linkFor: setupStepLink,
    progress: setupProgress,
    logs: setupLogs,
    verify: setupVerifyBox,
  }));
}

/** setupStepLink is the link of an optional step. */
function setupStepLink(step) {
  if (step.id === "bench") return { href: "#benchmark", label: "the benchmark" };
  if (step.id === "tuning" || step.id === "power") return { href: "#dashboard", label: "the dashboard" };
  return null;
}

/** setupStepNote is the extra line under a step of the checklist. */
function setupStepNote(step) {
  if (step.id === "recipe") return setupRecipeNote();
  return null;
}

/** paintSetupFacts draws the plan of the host and the facts under it. */
async function paintSetupFacts(doc) {
  if (!setupState.hero) return;
  const facts = await setupFactList(doc);
  const sentence = setupPlanSentence();
  put(setupState.hero,
    h("div.setup-plan",
      h("p", "bernie picked the setup that fits this host."),
      h("p.small.muted", "Anything that needs a password is shown as a command to paste."),
      sentence ? h("p.small.muted", sentence) : null),
    facts.length ? h("dl.facts", facts.map(setupFact)) : null);
}

/** setupFactList is the facts of the host. The onboarding document carries
 *  them; the hardware and the doctor report fill them until it does. */
async function setupFactList(doc) {
  if (Array.isArray(doc.facts) && doc.facts.length) return doc.facts;
  const hw = await setupLoadHW();
  const doctor = setupState.doctor || await setupLoadDoctorReport();
  return setupFactsFromHost(hw, doctor);
}

/** setupFact is one fact: its name, its value and the reason under it. */
function setupFact(fact) {
  const row = factRow(fact.label || fact.id || "", fact.value, fact.why);
  row.classList.add("setup-fact");
  return row;
}

/** setupFactsFromHost reads the facts of the host from the hardware and the
 *  doctor report. */
function setupFactsFromHost(hw, doctor) {
  const facts = [];
  if (hw) {
    const gpus = hw.gpus || [];
    const names = {};
    for (const gpu of gpus) {
      const name = gpu.name || gpu.gfx || "GPU";
      names[name] = (names[name] || 0) + 1;
    }
    const archs = Array.from(new Set(gpus.map((g) => g.gfx).filter(Boolean)));
    const value = gpus.length
      ? Object.keys(names).map((name) => names[name] + " x " + name).join(", ")
      : "–";
    let why = "No AMD GPU is visible to bernie.";
    if (gpus.length === 1 && archs.length === 1) why = "one recipe family fits them all";
    if (gpus.length > 1 && archs.length === 1) why = "every card is " + archs[0] + ", so one recipe family fits them all";
    if (gpus.length && archs.length > 1) why = "the cards use " + archs.length + " architectures";
    facts.push({ id: "gpus", label: "GPUs", value: value, why: why });

    const ram = hw.host && hw.host.ram_bytes;
    facts.push({ id: "memory", label: "Memory", value: ram ? bytes(ram) : "–", why: setupMemoryWhy(hw) });

    const engines = (hw.host && hw.host.engines) || [];
    const usable = engines.filter((e) => e.usable);
    const engine = usable[0] || engines[0];
    facts.push({
      id: "engine",
      label: "Engine",
      value: engine ? (engine.name + (engine.version ? " " + engine.version : "")) : "–",
      why: usable.length ? "the image is used" : "the server runs on the host",
    });
  }
  if (doctor) {
    const rocm = (doctor.checks || []).find((c) => c.category === "rocm");
    facts.push({
      id: "rocm",
      label: "ROCm",
      value: rocm ? rocm.status : "–",
      why: rocm ? (rocm.detail || rocm.title) : "The doctor could not read the host.",
    });
  }
  return facts;
}

/** setupMemoryWhy says what the memory of the host is used for. */
function setupMemoryWhy(hw) {
  const gpus = (hw && hw.gpus) || [];
  const vram = gpus.reduce((sum, gpu) => sum + (gpu.vram_bytes || 0), 0);
  if (!vram) return "";
  const perCard = Math.round(vram / gpus.length / (1024 * 1024 * 1024));
  return perCard + " GiB per card, the rest is the KV cache";
}

/** setupPlanSentence names the recipe that fits this host. */
function setupPlanSentence() {
  const recipes = (setupState.recipes && setupState.recipes.recipes) || [];
  const best = recipes.find((r) => r.fit && r.fit.status === "fits");
  if (!best) return "";
  const gpus = (setupState.hw && setupState.hw.gpus) || [];
  const archs = Array.from(new Set(gpus.map((g) => g.gfx).filter(Boolean)));
  let cards = "";
  if (gpus.length && archs.length) cards = gpus.length + " cards of " + archs.join(", ");
  else if (best.gpus) cards = best.gpus + " cards";
  return "The recipe is " + (best.name || best.id) + (cards ? " with " + cards : "") + ".";
}

/** setupLoadHW reads the hardware of the host once. */
async function setupLoadHW() {
  if (setupState.hw) return setupState.hw;
  try {
    setupState.hw = await api("/api/hw");
  } catch (e) {
    setupState.hw = null;
  }
  return setupState.hw;
}

/** setupLoadDoctorReport reads the doctor report once. */
async function setupLoadDoctorReport() {
  if (setupState.doctor) return setupState.doctor;
  try {
    setupState.doctor = await api("/api/doctor");
  } catch (e) {
    setupState.doctor = null;
  }
  return setupState.doctor;
}

/** loadSetupDoctor reads the doctor report and paints the findings. */
async function loadSetupDoctor() {
  const box = setupState.doctorBox;
  if (!box) return;
  put(box, loading());
  let report = null;
  try {
    report = await api("/api/doctor");
  } catch (e) {
    put(box, h("p", "The doctor could not run."),
      h("p", String(e.message || e)),
      cmdBlock("bernie doctor", "Copy the command"),
      h("button", { type: "button", onclick: loadSetupDoctor }, "Try again"));
    return;
  }
  setupState.doctor = report;
  const checks = report.checks || [];
  const bad = checks.filter((c) => c.status === "fail" || c.status === "warn");
  const summary = report.summary || {};
  put(box,
    h("div.doctor-summary", ["fail", "warn", "ok", "skip"].map((s) =>
      h("span.badge", { class: "b-" + s }, (summary[s] || 0) + " " + s))),
    bad.length
      ? h("div.doctor-findings-list", bad.map(setupDoctorFinding))
      : h("div.doctor-clean",
        h("p", "The doctor found nothing to report."),
        h("p", "Every check of the host passed.")),
    h("details.doctor-all", h("summary", "All checks (" + checks.length + ")"), checks.map(setupDoctorFinding)));
}

/** setupDoctorFinding is one finding: its state, its rule, the cause and
 *  the command that fixes it. */
function setupDoctorFinding(check) {
  const status = check.status || "warn";
  return h("div.finding.doctor-finding",
    h("div.doctor-finding-head",
      h("span.badge", { class: "b-" + status }, status),
      h("span.doctor-finding-id", check.id || "")),
    h("p.doctor-finding-cause", check.title || ""),
    check.detail ? h("p.small.muted", check.detail) : null,
    check.fix ? cmdBlock(check.fix, "Copy the fix command") : null,
    check.doc ? h("a.small", { href: check.doc, target: "_blank", rel: "noopener noreferrer" }, "the rule") : null);
}

/** setupVerifyBox is the box that asks the server one question. It appears
 *  when the endpoint exists; until then the step has no box. */
function setupVerifyBox() {
  if (!SETUP_SMOKE_ENABLED) return null;
  const running = setupServerRunning();
  const answer = h("div.try-answer");
  const input = h("input", {
    type: "text",
    placeholder: "What is the capital of Portugal?",
    disabled: !running,
    "aria-label": "The question for the server",
  });
  const form = h("form.try-form", {
    onsubmit: (e) => {
      e.preventDefault();
      setupAskServer(input.value, answer);
    },
  }, input, h("button", { type: "submit", disabled: !running }, "Ask"));
  return h("div.try-box",
    h("p.try-label", "Ask the server something."),
    running ? form : h("p.try-note", "Start the server first."),
    answer);
}

/** setupServerRunning says whether a server answers on this host. */
function setupServerRunning() {
  const doc = setupState.doc;
  const serve = doc && (doc.steps || []).find((s) => s.id === "serve");
  return setupServerServing(serve);
}

/** setupServerServing says whether the server of this host is up. A run that
 *  still loads the model is not serving yet: it is setting up. */
function setupServerServing(serve) {
  if (!serve || serve.state !== "running") return false;
  const text = ((serve.progress && serve.progress.phase) || "") + " " + (serve.summary || "");
  return !/loading|compiling|capturing|starting/i.test(text);
}

/** setupAskServer sends one prompt to the server and shows the answer. */
async function setupAskServer(prompt, answer) {
  if (!prompt) return;
  put(answer, h("span.spin"), " asking…");
  try {
    const result = await api("/api/smoke", { method: "POST", body: { prompt: prompt } });
    const metrics = [];
    if (result.ttft_ms != null) metrics.push(result.ttft_ms + " ms to the first token");
    if (result.tok_s != null) metrics.push(result.tok_s + " tok/s");
    put(answer,
      h("p.try-reply", result.reply || ""),
      metrics.length ? h("p.try-metrics", metrics.join(" · ")) : null);
  } catch (e) {
    put(answer,
      h("p.try-error", "The server did not answer."),
      h("p.try-reason", String(e.message || e)));
  }
}

/** focusSetupStep scrolls to a step and puts the focus on its first
 *  control. */
function focusSetupStep(id) {
  if (!id) return;
  const row = document.querySelector('[data-step="' + id + '"]');
  if (!row) return;
  row.scrollIntoView({ block: "center" });
  const control = row.querySelector("button, a, input");
  if (control) control.focus();
}

/** focusSetupDoctor scrolls to the doctor report. */
function focusSetupDoctor() {
  const section = document.getElementById("setup-doctor");
  if (!section) return;
  section.scrollIntoView({ block: "start" });
  const control = section.querySelector("button, a");
  if (control) control.focus();
}

/** focusSetupVerify scrolls to the verify step. */
function focusSetupVerify() {
  focusSetupStep("verify");
}

/** setupToggleAdvanced opens or closes the raw documents of the tab. The
 *  page-wide Advanced switch owns the state, so the rail button and this
 *  button always agree. */
function setupToggleAdvanced() {
  if (typeof toggleAdvanced === "function") {
    toggleAdvanced();
    return;
  }
  const panel = setupState.advanced;
  if (!panel) return;
  panel.open = !panel.open;
  if (panel.open) panel.scrollIntoView({ block: "nearest" });
}
