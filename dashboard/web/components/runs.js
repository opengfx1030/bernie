// Shared run table used by the Dashboard overview and the Logs tab.
"use strict";

// stateNote says in one plain line what the state of a run means. A run
// that is still starting says that the server is loading the model.
function stateNote(r) {
  if (r.state === "starting") return "the server is loading the model";
  if (r.state === "exited") return "the server is gone";
  return "";
}

function runsTable(runs, compact, logCard, root) {
  if (!runs.length) return emptyState("No run is serving.", "A run is a model server on this host: it loads the model and then answers requests. Start one from the Setup tab, or run it in a terminal:", "bernie serve <recipe>");
  const tele = new Map(((state.tele && state.tele.runs) || []).map((r) => [r.name, r]));
  return h("div.scroll", h("table",
    h("thead", h("tr", h("th", "Name"), h("th", "Recipe"), h("th", "State"), compact ? null : h("th", "Mode"), h("th", "Address"), compact ? null : h("th", "Started"), compact ? null : h("th", ""))),
    h("tbody", runs.map((r) => {
      const t = r.state !== "exited" ? tele.get(r.name) : null;
      const note = stateNote(r);
      return h("tr",
        h("td", h("b", r.name)),
        h("td", r.recipe),
        h("td", badge(r.state), note ? h("div.small", note) : null,
          r.phase && r.phase.sentence ? h("div.small", r.phase.sentence) : null,
          t && t.stats ? h("div.small.muted", `${num(t.stats.requests_running, 0)} running · ${num(t.gen_tok_s)} tok/s`) : null,
          r.findings && r.findings.length ? h("div.small.muted", "Known failure: " + r.findings.map((f) => f.id).join(", ")) : null),
        compact ? null : h("td", r.mode + (r.engine ? " (" + r.engine + ")" : "")),
        h("td.mono", r.port ? h("a", { href: runURL(r) + "/v1/models", target: "_blank", rel: "noopener noreferrer" }, (r.host || "127.0.0.1") + ":" + r.port) : "–"),
        compact ? null : h("td.small", r.started_at ? ago(r.started_at) : ""),
        compact ? null : h("td", h("div.row",
          h("button.small", { onclick: () => followLogs(logCard, r.name, r.findings) }, "Logs"),
          r.state !== "exited" ? h("button.small.danger", { onclick: (ev) => stopRun(ev, r.name, root) }, "Stop") : null)));
    }))));
}
