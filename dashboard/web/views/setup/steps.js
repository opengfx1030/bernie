"use strict";
// ---------- Home: step actions ----------
// The timeline itself is a shared component. This file owns the actions that
// start a step and follow its background job.

// EXTRA_STEPS are the steps that never hold the path. They are listed after
// the path, under their own rule.
const EXTRA_STEPS = ["bench", "tuning", "power"];

// homeSteps renders the checklist of a host.
function homeSteps(doc, onRefresh) {
  return stepTimeline(doc.steps || [], {
    extraIDs: EXTRA_STEPS,
    onRefresh,
    onRun: runStep,
  });
}

// startStep asks the server to run a step. It answers with the job of the
// step, or with the commands of a step that needs root.
async function startStep(id) {
  const result = await request("/api/onboarding/" + encodeURIComponent(id) + "/run", {
    method: "POST",
    body: {},
  });
  const data = result.data;
  if (result.status === 200 && data && data.root_required) return { kind: "root", commands: data.commands || [] };
  if (result.status === 201) return { kind: "job", job: data };
  if (result.status === 409) return { kind: "busy", error: (data && data.error) || "another step is running" };
  throw new Error((data && data.error) || (result.response.status + " " + result.response.statusText));
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
  let es;
  const finish = (name, doc) => {
    es.close();
    put(box, h("p.progress-sentence", { "aria-live": "polite" },
      name === "done" ? "Done." : "Failed: " + ((doc && doc.error) || "the step did not finish")));
    if (onRefresh) onRefresh();
  };
  es = openJobEvents(job.id, {
    line: (ev) => {
      sentence.textContent = ev.data;
      lines.append(h("div", ev.data));
    },
    done: (ev) => finish("done", JSON.parse(ev.data)),
    failed: (ev) => finish("failed", JSON.parse(ev.data)),
  });
}
