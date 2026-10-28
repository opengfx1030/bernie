"use strict";
// ---------- Home: the step timeline ----------
// The timeline is the onboarding checklist of spec §3.3: the ordered steps
// that lead from a bare machine to a serving model. A step that bernie can
// do has a Run button; a step that needs root has the command and a copy
// button, and bernie never runs it.

// EXTRA_STEPS are the steps that never hold the path. They are listed after
// the path, under their own rule.
const EXTRA_STEPS = ["bench", "tuning", "power"];

// STATE_WORDS are the words of the states of spec §3.3. The word carries the
// meaning; the colour only helps.
const STATE_WORDS = {
  done: "done", todo: "todo", running: "running", blocked: "blocked", skipped: "skipped",
};

// homeSteps renders the checklist of a host.
function homeSteps(doc, onRefresh) {
  const steps = doc.steps || [];
  if (!steps.length) {
    return emptyState("There is no checklist on this host.",
      "The checklist is the ordered list of steps that lead from a bare machine to a serving model. It comes from the doctor report, the setup plan and the runs.",
      "bernie status --steps");
  }
  const path = steps.filter((s) => !EXTRA_STEPS.includes(s.id));
  const extra = steps.filter((s) => EXTRA_STEPS.includes(s.id));
  return h("div",
    h("ol.steps", path.map((s) => homeStep(s, onRefresh))),
    extra.length ? h("details", h("summary.muted.small", "Optional steps (" + extra.length + ")"),
      h("ol.steps", extra.map((s) => homeStep(s, onRefresh)))) : null);
}

// homeStep renders one step: its state, its sentence, its command and the
// action that fits the step.
function homeStep(step, onRefresh) {
  const body = h("div.step-body",
    h("h3", step.title, " ", h("span.pill", { class: "pill " + pillClass(step.state) }, STATE_WORDS[step.state] || step.state)),
    step.summary ? h("p.why", step.summary) : null,
    step.detail ? h("p.small.muted", step.detail) : null,
    step.command ? cmdBlock(step.command, "Copy the command") : null);
  if ((step.root_commands || []).length) body.append(rootCommands(step));
  if (step.can_run) {
    body.append(h("div.step-actions",
      h("button.small", { type: "button", onclick: () => runStep(step, onRefresh) }, "Run this step"),
      h("span.small.muted", "bernie does this step for you. It runs in the background.")));
  }
  if ((step.problems || []).length) body.append(h("div.problems", step.problems.map(problemRow)));
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

// problemRow shows one doctor check that holds a step back: the cause and
// the command that fixes it.
function problemRow(p) {
  return h("div.check",
    h("div.t", badge(p.status), h("b", p.title), h("span.id", p.id)),
    p.detail ? h("div.muted.small", p.detail) : null,
    p.fix ? cmdBlock(p.fix, "Copy the fix command") : null,
    p.doc ? h("a.small", { href: p.doc, target: "_blank", rel: "noopener noreferrer" }, "docs") : null);
}

// startStep asks the server to run a step. It answers with the job of the
// step, or with the commands of a step that needs root.
async function startStep(id) {
  const r = await fetch("/api/onboarding/" + encodeURIComponent(id) + "/run", {
    method: "POST",
    headers: { "X-Bernie": "1", "Content-Type": "application/json" },
    body: "{}",
    credentials: "same-origin",
  });
  const text = await r.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  if (r.status === 200 && data && data.root_required) return { kind: "root", commands: data.commands || [] };
  if (r.status === 201) return { kind: "job", job: data };
  if (r.status === 409) return { kind: "busy", error: (data && data.error) || "another step is running" };
  throw new Error((data && data.error) || (r.status + " " + r.statusText));
}

// runStep starts a step and shows its progress under it.
async function runStep(step, onRefresh) {
  const box = stepProgressBox(step.id);
  if (!box) return;
  put(box, h("span.spin"), " starting…");
  let res;
  try {
    res = await startStep(step.id);
  } catch (e) {
    put(box, h("div.err", String(e.message || e)));
    return;
  }
  if (res.kind === "root") {
    put(box, h("div.err", "This step needs root. Run the command above in a terminal, then re-check."));
    return;
  }
  if (res.kind === "busy") {
    put(box, h("div.err", res.error + ". Wait for it to finish, then run this step again."));
    return;
  }
  followJob(res.job, box, onRefresh);
}

// stepProgressBox returns the box under a step where its progress goes.
function stepProgressBox(id) {
  const el = document.querySelector('[data-step="' + id + '"] .progress');
  return el || null;
}

// followJob follows the output of a job: one line per event, then the job
// document when it ends.
function followJob(job, box, onRefresh) {
  const sentence = h("p.progress-sentence", { "aria-live": "polite" }, job.phase || "working…");
  const lines = h("div.log-lines");
  put(box, sentence, lines);
  const es = new EventSource("/api/jobs/" + encodeURIComponent(job.id) + "/events");
  const finish = (name, doc) => {
    es.close();
    put(box, h("p.progress-sentence", { "aria-live": "polite" },
      name === "done" ? "Done." : "Failed: " + ((doc && doc.error) || "the step did not finish")));
    if (onRefresh) onRefresh();
  };
  es.addEventListener("line", (ev) => {
    sentence.textContent = ev.data;
    lines.append(h("div", ev.data));
  });
  es.addEventListener("done", (ev) => finish("done", JSON.parse(ev.data)));
  es.addEventListener("failed", (ev) => finish("failed", JSON.parse(ev.data)));
  es.addEventListener("error", () => { /* the stream ended without a final event */ });
}
