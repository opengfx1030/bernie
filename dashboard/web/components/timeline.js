// Shared onboarding step timeline. The view supplies the action that starts
// a runnable step; this component owns the order, state and presentation.
"use strict";

// STATE_WORDS are the words of the states of spec §3.3. The word carries the
// meaning; the colour only helps.
const STATE_WORDS = {
  done: "done", todo: "todo", running: "running", blocked: "blocked", skipped: "skipped",
};

/** stepTimeline renders the ordered steps and the optional extras. */
function stepTimeline(steps, options = {}) {
  if (!steps.length) {
    return emptyState("There is no checklist on this host.",
      "The checklist is the ordered list of steps that lead from a bare machine to a serving model. It comes from the doctor report, the setup plan and the runs.",
      "bernie status --steps");
  }
  const extraIDs = options.extraIDs || [];
  const path = steps.filter((s) => !extraIDs.includes(s.id));
  const extra = steps.filter((s) => extraIDs.includes(s.id));
  return h("div",
    h("ol.steps", path.map((s) => stepItem(s, options))),
    extra.length ? h("details", h("summary.muted.small", "Optional steps (" + extra.length + ")"),
      h("ol.steps", extra.map((s) => stepItem(s, options)))) : null);
}

/** stepItem renders one step: its state, sentence, command and action. */
function stepItem(step, options = {}) {
  const body = h("div.step-body",
    h("h3", step.title, " ", statusPill(STATE_WORDS[step.state] || step.state, pillClass(step.state))),
    step.summary ? h("p.why", step.summary) : null,
    step.detail ? h("p.small.muted", step.detail) : null,
    step.command ? cmdBlock(step.command, "Copy the command") : null);
  if ((step.root_commands || []).length) body.append(rootCommands(step));
  if (step.can_run && options.onRun) {
    body.append(h("div.step-actions",
      h("button.small", { type: "button", onclick: () => options.onRun(step, options.onRefresh) }, "Run this step"),
      h("span.small.muted", "bernie does this step for you. It runs in the background.")));
  }
  if ((step.problems || []).length) body.append(h("div.problems", step.problems.map(stepProblem)));
  body.append(h("div.progress"));
  return h("li.step", { class: "step " + step.state, "data-step": step.id },
    h("span.step-mark", { "aria-hidden": "true" }, stateMark(step.state)), body);
}

// pillClass maps a state of the checklist to the class of a pill.
function pillClass(state) {
  if (state === "done") return "done";
  if (state === "todo") return "todo";
  if (state === "running") return "running";
  if (state === "blocked") return "warn";
  return "skip";
}

// stateMark is the mark in the circle of a step: the word is on the pill
// next to the title, the mark only helps.
function stateMark(state) {
  if (state === "done") return "✓";
  if (state === "blocked") return "!";
  if (state === "running") return "●";
  if (state === "skipped") return "–";
  return "";
}

// rootCommands shows the commands the operator runs as root. bernie prints
// them and never runs them.
function rootCommands(step) {
  return h("div.root",
    h("p.small", "This step needs root. Run it in a terminal, then re-check."),
    step.root_commands.map((c) => cmdBlock(c, "Copy the command")));
}

// stepProblem shows one doctor check that holds a step back.
function stepProblem(p) {
  return h("div.check",
    h("div.t", badge(p.status), h("b", p.title), h("span.id", p.id)),
    p.detail ? h("div.muted.small", p.detail) : null,
    p.fix ? cmdBlock(p.fix, "Copy the fix command") : null,
    p.doc ? h("a.small", { href: p.doc, target: "_blank", rel: "noopener noreferrer" }, "docs") : null);
}
