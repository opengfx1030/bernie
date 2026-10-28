"use strict";
// ---------- Runs ----------

let logES = null;
function stopLogs() { if (logES) { logES.close(); logES = null; } }

async function viewRuns(root) {
  const head = stateHead("Runs", h("a.btn.primary", { href: "#recipes" }, "Start a run"));
  const list = h("div.card", cardHead("Runs", "/api/runs", h("button.small", { onclick: () => viewRuns(root) }, "Refresh")), loading());
  const logs = h("div.card", cardHead("Logs"), emptyState("No log yet.", "Start the server first, then follow its log here."));
  put(root, head, list, h("div.gap"), logs);
  try {
    const runs = runsOf(await api("/api/runs"));
    setHead(head, runsHeadline(runs), runsLede(runs));
    list.lastChild.replaceWith(runsTable(runs, false, logs, root));
    // A run that is still loading shows its log right away: the model load
    // takes minutes and the log says what the server is doing.
    const loading = runs.find((r) => r.state === "starting");
    if (loading) followLogs(logs, loading.name, loading.findings);
  } catch (e) { list.lastChild.replaceWith(failed(e)); }
}

// runsHeadline says in one sentence what is running.
function runsHeadline(runs) {
  const live = runs.filter((r) => r.state === "ready" || r.state === "starting").length;
  if (!runs.length) return "No run is serving";
  if (live) return live + (live === 1 ? " run is" : " runs are") + " serving";
  return runs.length + (runs.length === 1 ? " run has" : " runs have") + " exited";
}

// runsLede says what a run is and what to do next.
function runsLede(runs) {
  if (!runs.length) return "A run is a model server on this host: it loads the model and then answers requests. Start one from the Recipes tab.";
  return "A run is a model server on this host. Its log says what it is doing, and the findings name the rule that matches a known failure.";
}

// stateNote says in one plain line what the state of a run means. A run
// that is still starting says that the server is loading the model.
function stateNote(r) {
  if (r.state === "starting") return "the server is loading the model";
  if (r.state === "exited") return "the server is gone";
  return "";
}

function runsTable(runs, compact, logCard, root) {
  if (!runs.length) return emptyState("No run is serving.", "A run is a model server on this host: it loads the model and then answers requests. Start one from the Recipes tab, or run it in a terminal:", "bernie serve <recipe>");
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

async function stopRun(ev, name, root) {
  if (!confirm(`Stop run ${name}?`)) return;
  ev.target.disabled = true;
  try {
    await api("/api/runs/" + encodeURIComponent(name), { method: "DELETE" });
    toast(`Stopped ${name}.`, true);
  } catch (e) { toast("Stop failed: " + e.message); }
  viewRuns(root);
}

// findingsBox shows the known failures logdoc found in the log of an exited run (spec §5.3).
function findingsBox(fs) {
  if (!fs || !fs.length) return null;
  return h("div.findings", { role: "region", "aria-label": "Known failures in this log" },
    h("h3", `${fs.length} known failure${fs.length > 1 ? "s" : ""} in this log`),
    fs.map((f) => h("div.finding",
      h("div.row", badge(f.severity === "error" ? "fail" : f.severity), h("b.mono", f.id), h("span.small.muted", `line ${f.line_no}`)),
      h("div", h("b", "Cause: "), f.cause),
      h("div", h("b", "Fix: "), /^bernie /.test(f.fix) ? h("div.row", h("code", f.fix), copyBtn(f.fix)) : f.fix),
      /^https?:\/\//.test(f.doc) ? h("div.small", h("a", { href: f.doc, target: "_blank", rel: "noopener noreferrer" }, "Documentation")) : null,
      h("pre.small", f.line))));
}

function followLogs(card, name, findings) {
  stopLogs();
  const box = h("div.log");
  const status = h("span.small.muted", "connecting…");
  let follow = true;
  box.addEventListener("scroll", () => { follow = box.scrollTop + box.clientHeight >= box.scrollHeight - 4; });
  put(card, cardHead("Logs: " + name, "/api/runs/" + encodeURIComponent(name) + "/logs", status, h("button.small", { onclick: () => { stopLogs(); status.textContent = "stopped"; } }, "Close")), findingsBox(findings), box);
  logES = new EventSource(`/api/runs/${encodeURIComponent(name)}/logs`);
  logES.onopen = () => (status.textContent = "following");
  logES.onerror = () => (status.textContent = "disconnected");
  logES.addEventListener("end", () => { status.textContent = "log ended"; stopLogs(); });
  logES.addEventListener("error", (ev) => { if (ev.data) { box.append(h("div.err", ev.data)); } });
  let pending = [];
  let raf = 0;
  logES.onmessage = (ev) => {
    pending.push(ev.data);
    if (raf) return;
    raf = requestAnimationFrame(() => {
      raf = 0;
      box.append(pending.join("\n") + "\n");
      pending = [];
      while (box.childNodes.length > 4000) box.firstChild.remove();
      if (follow) box.scrollTop = box.scrollHeight;
    });
  };
}
