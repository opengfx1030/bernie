"use strict";
// ---------- Home ----------
// Home answers one question: what state is this machine in, and what is the
// next thing to do? It renders the onboarding checklist of spec §3.3 and the
// doctor report. The page holds no state of its own: what the server says
// is what the page shows, and a re-check asks the server again.

// homeDoc is the onboarding document of the last read.
let homeDoc = null;

// checking is the state of a card whose answer takes a moment: it names the
// phase instead of showing nothing.
function checking(phase) {
  return h("div.empty", h("span.spin"), " " + phase);
}

async function viewHome(root) {
  const actions = h("div.head-actions");
  const head = stateHead("Home", actions);
  const factsCard = h("div.card.hero", cardHead("This machine", "/api/hw"), loading());
  const stepsCard = h("div.card", cardHead("The path to a serving model", "/api/onboarding"), checking("checking the host…"));
  const docCard = h("div.card", cardHead("Doctor", "/api/doctor"), loading());
  const runCard = h("div.card", cardHead("Runs", "/api/runs"), loading());
  const hostCard = h("div.card", cardHead("Host", "/api/hw"), loading());
  put(root, head, factsCard, stepsCard, advancedPanel(),
    h("div.cols", h("div", docCard, h("div.gap"), runCard), hostCard));

  // The checklist is the slow answer: the headline names the phase instead
  // of showing a dot.
  setHead(root.querySelector(".state-head"), "Checking this machine…",
    "bernie reads the host: the GPUs, the driver, the ROCm runtime, the Python environment and the runs.");

  loadDoctor(docCard);
  api("/api/runs").then((d) => put(runCard, cardHead("Runs", "/api/runs", h("a.small", { href: "#runs" }, "manage")), runsTable(runsOf(d), true)))
    .catch((e) => put(runCard, cardHead("Runs", "/api/runs"), failed(e)));
  loadHW(true).then((hw) => put(hostCard, cardHead("Host", "/api/hw"), hostSummary(hw)))
    .catch((e) => put(hostCard, cardHead("Host", "/api/hw"), failed(e)));

  const paint = () => refreshHome(root, stepsCard, factsCard, actions);
  await paint();
}

// refreshHome reads the onboarding checklist again and paints the headline,
// the facts and the timeline.
async function refreshHome(root, stepsCard, factsCard, actions) {
  let doc;
  try {
    doc = await api("/api/onboarding");
  } catch (e) {
    put(stepsCard, cardHead("The path to a serving model", "/api/onboarding"), failed(e));
    return;
  }
  homeDoc = doc;
  const say = homeHeadline(doc);
  if (doc.stale) {
    say.lede = "The checklist is " + doc.facts_age_s + " seconds old; bernie is checking the host again. " + say.lede;
  }
  setHead(root.querySelector(".state-head"), say.title, say.lede);
  put(factsCard, cardHead("This machine", "/api/hw"), homeFacts(doc));
  put(stepsCard, cardHead("The path to a serving model", "/api/onboarding",
    h("button.small", { type: "button", onclick: () => refreshHome(root, stepsCard, factsCard, actions) }, "Re-check")),
    homeSteps(doc, () => refreshHome(root, stepsCard, factsCard, actions)));
  put(actions, homeActions(doc, () => refreshHome(root, stepsCard, factsCard, actions)));
}

// homeHeadline says in one sentence what state the machine is in.
function homeHeadline(doc) {
  const steps = doc.steps || [];
  const pending = steps.filter((s) => s.state !== "done" && s.state !== "skipped" && !EXTRA_STEPS.includes(s.id));
  const next = steps.find((s) => s.id === doc.next);
  const root = pending.filter((s) => (s.root_commands || []).length).length;
  if (!pending.length) {
    return { title: "This machine is serving", lede: "Every step of the path is done. The runs below say what is serving and where it listens." };
  }
  if (root) {
    return {
      title: root + (root === 1 ? " step needs" : " steps need") + " your password",
      lede: homeLede(pending) + " The steps that need root are marked; the command of each one is on the card.",
    };
  }
  if (next && next.state === "blocked") {
    return {
      title: pending.length + (pending.length === 1 ? " step needs" : " steps need") + " an operator",
      lede: homeLede(pending) + " The card of each step says what holds it back and the command that fixes it.",
    };
  }
  return {
    title: pending.length + (pending.length === 1 ? " step left" : " steps left") + " before this box can serve",
    lede: homeLede(pending) + " bernie does the rest.",
  };
}

// STEP_LABELS are the short names of the steps of the checklist. The ids are
// stable (spec §3.3), so the page can name a step in a few words.
const STEP_LABELS = {
  hardware: "GPUs", access: "device access", rocm: "ROCm runtime", "python-env": "Python environment",
  model: "model files", recipe: "recipe", serve: "server", verify: "check",
};

// homeLede names the steps that are left, in words.
function homeLede(pending) {
  const names = pending.slice(0, 3).map((s) => "the " + (STEP_LABELS[s.id] || s.title.toLowerCase()));
  const more = pending.length > 3 ? " and " + (pending.length - 3) + " more" : "";
  return "What is left: " + names.join(", ") + more + ".";
}

// homeFacts turns the steps into the fact strip: what the machine is, in
// short, with the reason under every value.
function homeFacts(doc) {
  const steps = (doc.steps || []).filter((s) => STEP_LABELS[s.id]);
  if (!steps.length) {
    return emptyState("There is nothing to say about this machine yet.",
      "The facts come from the checklist: the cards, the driver, the runtime, the model and the server.");
  }
  return h("dl.facts", steps.map((s) => h("div",
    h("dt", STEP_LABELS[s.id]),
    h("dd", s.summary || "–", homeWhy(s)))));
}

// homeWhy is the reason under a fact: the detail of the step, or the cause
// of the first problem that holds it back.
function homeWhy(step) {
  if (step.detail) return h("span.why", step.detail);
  const p = (step.problems || [])[0];
  if (!p) return null;
  return h("span.why", p.detail || p.fix || "");
}

// homeActions are the actions of the Home tab: the one primary action and
// the re-check.
function homeActions(doc, onRefresh) {
  const next = (doc.steps || []).find((s) => s.id === doc.next);
  const out = [];
  if (next && next.can_run) {
    out.push(h("button.primary", { type: "button", onclick: () => runStep(next, onRefresh) }, "Run the next step"));
  } else if (next) {
    const label = (next.root_commands || []).length ? "Show the commands" : "Show what to fix";
    out.push(h("button.primary", { type: "button", onclick: () => focusStep(next.id) }, label));
  }
  out.push(h("button", { type: "button", onclick: onRefresh }, "Re-check"));
  return out;
}

// focusStep scrolls a step into view and puts the cursor on its first
// control.
function focusStep(id) {
  const el = document.querySelector('[data-step="' + id + '"]');
  if (!el) return;
  el.scrollIntoView({ block: "center" });
  const c = el.querySelector("button, a");
  if (c) c.focus();
}

// loadDoctor paints the doctor card: the counts, the checks that fail or
// warn and every check behind a disclosure.
async function loadDoctor(card) {
  const title = () => cardHead("Doctor", "/api/doctor", h("button.small", { onclick: () => { put(card, title(), loading()); loadDoctor(card); } }, "Re-run"));
  put(card, title(), loading());
  try {
    const rep = await api("/api/doctor");
    const sum = rep.summary || {};
    const checks = rep.checks || [];
    const bad = checks.filter((c) => c.status === "fail" || c.status === "warn")
      .sort((a, b) => (a.status === "fail" ? 0 : 1) - (b.status === "fail" ? 0 : 1));
    put(card, title(),
      h("div.counts", ["fail", "warn", "ok", "skip"].map((s) => h("span.badge", { class: "b-" + s }, (sum[s] || 0) + " " + s))),
      bad.length ? bad.map(checkRow) : emptyState("Every check passed.", "The doctor looks for the things that keep a server from running on this host: the GPUs, the driver, the ROCm runtime and the Python environment."),
      h("details", h("summary.muted.small", "All checks (" + checks.length + ")"), checks.map(checkRow)));
  } catch (e) {
    put(card, title(), failed(e));
  }
}

// hostSummary is the key and value list of the host.
function hostSummary(hw) {
  const ho = hw.host || {};
  const cpu = ho.cpu || {};
  const gpus = hw.gpus || [];
  const byName = {};
  for (const g of gpus) {
    const k = (g.name || "GPU") + " (" + g.gfx + ")";
    byName[k] = (byName[k] || 0) + 1;
  }
  const engines = (ho.engines || []).map((e) => e.name + " " + (e.version || "") + (e.usable ? "" : " (not usable)") + (e.rootless ? ", rootless" : ""));
  return h("dl.kv",
    h("dt", "Host"), h("dd", ho.hostname || "–"),
    h("dt", "OS"), h("dd", (ho.os && (ho.os.pretty || ho.os.id)) || "–", ho.pkg_manager ? h("span.muted", " · " + ho.pkg_manager) : null),
    h("dt", "Kernel"), h("dd", ho.kernel || "–"),
    h("dt", "CPU"), h("dd", cpu.model || "–", cpu.threads ? h("span.muted", ` · ${cpu.sockets || 1}× socket, ${cpu.cores} cores / ${cpu.threads} threads`) : null),
    h("dt", "RAM"), h("dd", bytes(ho.ram_bytes)),
    h("dt", "GPUs"), h("dd", gpus.length ? Object.entries(byName).map(([k, n]) => h("div", n + "× " + k)) : h("span.muted", "none found")),
    h("dt", "Engines"), h("dd", engines.length ? engines.join("; ") : h("span.muted", "none")),
    ho.selinux ? [h("dt", "SELinux"), h("dd", ho.selinux)] : null,
  );
}

// checkRow is one check of the doctor: its state, its cause and its fix.
function checkRow(c) {
  return h("div.check",
    h("div.t", badge(c.status), h("b", c.title), h("span.id", c.id)),
    c.detail ? h("div.muted.small", c.detail) : null,
    c.fix ? cmdBlock(c.fix, "Copy the fix command") : null,
    c.doc ? h("a.small", { href: c.doc, target: "_blank", rel: "noopener noreferrer" }, "docs") : null);
}
