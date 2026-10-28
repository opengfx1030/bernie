// The onboarding step timeline. The Setup view owns the actions and the
// live job state; this component owns the order, the state words and the
// anatomy of one row.
"use strict";

// The words of the states of spec §3.3. The word carries the meaning; the
// colour only helps.
const TIMELINE_STATE_WORD = {
  done: "done",
  todo: "todo",
  running: "running",
  blocked: "blocked",
  skipped: "skipped",
  failed: "failed",
};

// TIMELINE_MARK is the glyph in the circle of a row. A todo row carries its
// number; the other states carry an icon.
const TIMELINE_MARK = {
  done: "✓",
  todo: "",
  running: "◐",
  blocked: "!",
  skipped: "–",
  failed: "✕",
};

// TIMELINE_PILL is the class of the pill of a state.
const TIMELINE_PILL = {
  done: "done",
  todo: "todo",
  running: "running",
  blocked: "blocked",
  skipped: "skipped",
  failed: "fail",
};

/**
 * stepTimeline renders the ordered steps of the onboarding checklist.
 *
 * options.extraIDs   the ids that come after the path of the host
 * options.onRun      starts a runnable step: function(step, options)
 * options.onRefresh  reads the checklist again
 * options.linkFor    the link of an optional step: function(step)
 * options.progress   the live progress of a step, by step id
 * options.logs       the output of a step, by step id
 * options.verify     the verify box of the server, when it is available
 */
function stepTimeline(steps, options = {}) {
  const list = Array.isArray(steps) ? steps : [];
  if (!list.length) {
    return emptyState("There is no checklist on this host.",
      "The checklist is the ordered list of steps that lead from a bare machine to a serving model. It comes from the doctor report, the setup plan and the runs.",
      "bernie status --steps");
  }
  const extraIDs = options.extraIDs || [];
  const path = list.filter((s) => !extraIDs.includes(s.id));
  const extra = list.filter((s) => extraIDs.includes(s.id));
  const progress = options.progress || {};
  const logs = options.logs || {};
  return h("div.setup-timeline",
    h("ol.step-timeline", path.map((s, i) => stepRow(s, i + 1, options, progress, logs, false))),
    extra.length ? h("p.section-label", "Optional steps") : null,
    extra.length ? h("ol.step-timeline", extra.map((s, i) => stepRow(s, i + 1, options, progress, logs, true))) : null);
}

/** stepRow renders one step of the checklist. */
function stepRow(step, index, options, progress, logs, optional) {
  const state = TIMELINE_STATE_WORD[step.state] ? step.state : "todo";
  const roots = step.root_commands || [];
  const body = h("div.step-body",
    h("h3", step.title || step.id, " ", stepPill(state)),
    h("p.why", stepSummary(step, state)),
    step.detail ? h("p.small.muted", step.detail) : null);
  if (roots.length) {
    body.append(h("div.root",
      h("p", "This step needs root. Run it in a terminal, then re-check."),
      roots.map((c) => cmdBlock(c, "Copy the command"))));
  }
  if ((step.problems || []).length) {
    body.append(h("div.problems", step.problems.map(stepProblem)));
  }
  body.append(stepProgressNode(step, state, progress[step.id], logs[step.id] || []));
  if (options.verify && step.id === "verify") {
    const box = options.verify(step);
    if (box) body.append(box);
  }
  const action = stepAction(step, state, options, optional);
  if (action) body.append(action);
  return h("li.step-row", { class: "step-row step-" + state, "data-step": step.id },
    h("div.step-mark", { "aria-hidden": "true" }, stepMark(state, index)),
    body);
}

/** stepPill is the state of a row: an icon and a word. */
function stepPill(state) {
  const icon = TIMELINE_MARK[state] || "•";
  return h("span.pill", { class: "pill " + (TIMELINE_PILL[state] || "todo") },
    h("span.pill-icon", { "aria-hidden": "true" }, icon),
    h("span.pill-word", TIMELINE_STATE_WORD[state]));
}

/** stepMark is the glyph in the circle: the number of a todo row, an icon
 *  for every other state. */
function stepMark(state, index) {
  if (state === "todo") return String(index);
  return TIMELINE_MARK[state] || "";
}

/** stepSummary is the one plain sentence under the title of a row. */
function stepSummary(step, state) {
  if (step.id === "model" && state === "todo") return "The model is not on this host yet.";
  if (step.summary) return step.summary;
  if (state === "done") return "This step is done.";
  if (state === "skipped") return "This step does not apply to this host.";
  return "bernie can do this step.";
}

/** stepAction is the control of a row: Run, Run again, the log of a running
 *  job, a link for an optional step. A step that needs root has its command
 *  and no button at all. */
function stepAction(step, state, options, optional) {
  if (optional) {
    const link = options.linkFor ? options.linkFor(step) : null;
    if (!link) return null;
    return h("div.step-actions", h("a", { href: link.href }, link.label));
  }
  if (state === "running") {
    return h("div.step-actions", h("button", { type: "button", onclick: () => stepToggleLog(step.id) }, "the log"));
  }
  if ((step.root_commands || []).length) return null;
  if (step.can_run && options.onRun) {
    const done = state === "done";
    return h("div.step-actions", h("button", {
      type: "button",
      class: done ? "" : "primary",
      onclick: () => options.onRun(step, options),
    }, done ? "Run again" : "Run"));
  }
  if (state === "blocked" && options.onRefresh) {
    return h("div.step-actions", h("button", { type: "button", onclick: options.onRefresh }, "Re-check"));
  }
  return null;
}

/** stepProgressNode is the progress of a row: the sentence, the bar, the
 *  numbers under it and the output of the job. */
function stepProgressNode(step, state, progress, lines) {
  const box = h("div.step-progress", { "data-progress": step.id });
  const live = progress || (state === "running" ? { phase: "Working…" } : null);
  if (!live && !lines.length) return box;
  const pct = typeof live.pct === "number" ? live.pct : 0;
  box.append(h("p.progress-sentence", { "aria-live": "polite" }, live.phase || ""));
  box.append(h("div.step-bar", {
    role: "progressbar",
    "aria-valuemin": "0",
    "aria-valuemax": "100",
    "aria-valuenow": String(pct),
    "aria-label": "Progress of " + (step.title || step.id),
  }, h("i", { style: "width:" + pct + "%" })));
  if (live.detail) box.append(h("p.small.muted", live.detail));
  if (lines.length) {
    box.append(h("details.step-log-box", { open: state === "running" },
      h("summary", "the log"),
      h("div.log-lines", { role: "log", "aria-live": "off" }, lines.map((l) => h("div", l)))));
  }
  return box;
}

/** stepProblem is one doctor check that holds a step back: its cause and
 *  the command that fixes it. */
function stepProblem(problem) {
  const status = problem.status || "warn";
  return h("div.check",
    h("div.t",
      h("span.badge", { class: "b-" + status }, status),
      h("b", problem.title || problem.id),
      h("span.id", problem.id || "")),
    problem.detail ? h("div.small.muted", problem.detail) : null,
    problem.fix ? cmdBlock(problem.fix, "Copy the fix command") : null,
    problem.doc ? h("a.small", { href: problem.doc, target: "_blank", rel: "noopener noreferrer" }, "the rule") : null);
}

/** stepToggleLog opens or closes the output of a step. */
function stepToggleLog(id) {
  const box = document.querySelector('[data-step="' + id + '"] .step-log-box');
  if (box) box.open = !box.open;
}
