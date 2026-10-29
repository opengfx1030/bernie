"use strict";
// ---------- The jobs of the host ----------
// The background actions bernie runs now: the steps of the setup path and
// the path itself. A job that runs shows its phase; a job that failed keeps
// its cause.

/** advJobList is the card with the jobs of the host. */
function advJobList() {
  const card = h("div.card");
  const view = {
    el: card,
    update: (jobs) => {
      const list = jobs || [];
      if (!list.length) {
        put(card, cardHead("The jobs", "/api/jobs"),
          emptyState("No job has run on this host.",
            "A job is one background action bernie runs now: a step of the setup path.",
            "bernie status --steps"));
        return;
      }
      const rows = list.map((job) => h("tr",
        h("td", job.id || "–"),
        h("td", job.step || "–"),
        h("td", advJobState(job.state)),
        h("td", job.phase || job.error || "–"),
        h("td", job.started_at ? ago(job.started_at) : "–")));
      put(card,
        cardHead("The jobs", "/api/jobs", h("span.small.muted", list.length + " jobs")),
        h("div.scroll", h("table",
          tableHead(["Job", "Step", "State", "Phase", "Since"]),
          h("tbody", rows))));
    },
  };
  return view;
}

/** advJobState is the state of a job: the word of the spec and the mark that
 *  helps. The word carries the meaning. */
function advJobState(state) {
  const kind = state === "running" ? "running" : state === "done" ? "done"
    : state === "failed" ? "fail" : "skip";
  return h("span.badge", { class: "b-" + kind }, state || "unknown");
}
