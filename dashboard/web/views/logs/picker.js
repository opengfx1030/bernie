// The picker of the run: one radio card per run. The run picker is the only
// selection of the tab; the tail follows it (design/logs.md).
"use strict";

/** logRunState is the state of a run as the tab names it. */
function logRunState(run) {
  if (run.state === "running") return { kind: "ok", word: "serving" };
  if (run.state === "starting") return { kind: "running", word: "starting" };
  return { kind: "skip", word: "exited" };
}

/** logRunPicker shows one radio card per run and picks the run to follow.
 *  The newest run is picked when the tab opens. */
function logRunPicker(runs, picked, onPick) {
  return h("div.lg-runs", runs.map((run) => {
    const st = logRunState(run);
    const input = h("input", {
      type: "radio", name: "lg-run", value: run.name, checked: run.name === picked,
      onchange: (ev) => { if (ev.target.checked) onPick(run); },
    });
    return h("label.lg-run", { class: run.name === picked ? "on" : "" },
      input,
      h("span.lg-run-body",
        h("span.lg-run-head", h("b", run.name), logPill(st.kind, st.word)),
        h("span.lg-run-age", run.started_at ? ago(run.started_at) : "–"),
        run.phase && run.phase.sentence ? h("span.lg-run-phase", run.phase.sentence) : null));
  }));
}
