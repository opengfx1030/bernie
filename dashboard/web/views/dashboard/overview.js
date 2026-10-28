"use strict";
// ---------- The run card ----------
// The card of the run that serves: its state, its phase, its endpoint, its
// age, the buttons that stop or restart it, the box that asks it one
// question and the findings of the log doctor.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.

// dashRunState is the pill of a run: an icon and a word.
function dashRunState(run) {
  if (!run) return dashPill("skip", "idle");
  if (run.state === "running") return dashPill("ok", "serving");
  if (run.state === "starting") return dashPill("running", "starting");
  return dashPill("fail", "stopped");
}

// dashRunCard is the card of the run. It repaints itself with the stream.
function dashRunCard(section) {
  const el = h("div.card");
  section.append(el);
  const box = h("div", { class: "dash-run-grid" });
  const left = h("div", { class: "dash-min0" });
  const right = h("div", { class: "dash-run-right" });
  el.append(box);
  box.append(left, right);

  const name = h("h3", { class: "dash-run-name" }, "No run is serving.");
  const pill = h("span", { class: "dash-pill-wrap" });
  const title = h("div", { class: "dash-run-title" }, name, pill);
  const phase = h("p", { class: "dash-run-phase" }, "");
  const endpoint = h("div");
  const age = h("span", { class: "dash-note-13-muted" }, "");
  const stop = h("button", { type: "button" }, "Stop");
  const restart = h("button", { type: "button" }, "Restart");
  const actions = h("div", { class: "dash-run-actions" }, age, stop, restart);
  const findings = h("div", { class: "dash-mt12" });
  left.append(title, phase, endpoint, actions, findings);

  const ask = h("div", { class: "dash-ask-label" }, "Ask the server something.");
  const prompt = h("input", { type: "text", value: "Hello", "aria-label": "The prompt to send to the server",
    class: "dash-ask-input" });
  const send = h("button.primary", { type: "button" }, "Try it");
  const answer = h("div", { class: "dash-answer" });
  right.append(ask, prompt, h("div", { class: "dash-mt8" }, send), answer);

  stop.addEventListener("click", () => dashStopRun());
  restart.addEventListener("click", () => dashRestartRun());
  send.addEventListener("click", () => dashAsk(prompt.value, answer, send));

  const api = {
    update: () => {
      const run = dashLiveRun();
      const live = dash.tele && (dash.tele.runs || []).find((r) => run && r.name === r.name);
      if (!run) {
        name.textContent = "No run is serving.";
        pill.replaceChildren(dashRunState(null));
        phase.textContent = "";
        endpoint.replaceChildren(emptyState("No run is serving.",
          "A run is a model server on this host: it loads the model and then answers requests.",
          "bernie serve <recipe>"));
        age.textContent = "";
        stop.disabled = true;
        restart.disabled = true;
        findings.replaceChildren();
        prompt.disabled = true;
        send.disabled = true;
        answer.replaceChildren(h("div", { class: "dash-muted" }, "Start the server first."));
        return;
      }
      name.textContent = run.name;
      pill.replaceChildren(dashRunState(run));
      phase.textContent = dashPhase(run) || (run.state === "exited" ? "The run has ended." : "");
      const url = "http://" + (run.host || "127.0.0.1") + ":" + run.port;
      endpoint.replaceChildren(cmdBlock(url, "Copy the endpoint"));
      age.textContent = run.started_at ? "up " + ago(run.started_at) : "";
      stop.disabled = run.state === "exited";
      restart.disabled = run.state === "exited";
      prompt.disabled = false;
      send.disabled = false;
      if (live && !live.stats && run.state !== "exited") {
        answer.replaceChildren(h("div", { class: "dash-fail" }, "The server did not answer."),
          h("div", { class: "dash-muted-note" }, live.error || "The process is alive but /health stays silent."));
      } else if (answer.dataset.answer !== "1") {
        answer.replaceChildren(h("div", { class: "dash-muted" },
          "One prompt, one answer, the time to the first token and the tokens per second."));
      }
      findings.replaceChildren(dashFindings(run));
    },
  };
  api.update();
  return api;
}

// dashFindings are the findings of the log doctor of an exited run.
function dashFindings(run) {
  const list = (run && run.findings) || [];
  if (!list.length) {
    return h("div", { class: "dash-note-13-muted" }, "No known failure in this log.");
  }
  const box = h("div.findings");
  for (const f of list) {
    const kind = f.severity === "error" || f.severity === "fail" ? "fail" : f.severity === "warn" ? "warn" : "info";
    box.append(h("div.finding",
      h("div", { class: "dash-finding-head" }, dashPill(kind, f.severity || "warn"), h("span", { class: "dash-finding-id" }, f.id)),
      f.cause ? h("div", { class: "dash-finding-cause" }, f.cause) : null,
      f.fix ? cmdBlock(f.fix, "Copy the fix") : null,
      f.doc ? h("a", { href: f.doc, target: "_blank", rel: "noopener noreferrer", class: "dash-small-12" }, "the rule in the documentation") : null));
  }
  return box;
}

// dashProblems is the section with the known problems of the host: the
// checks of the doctor that fail or warn.
function dashProblems(section) {
  const el = h("div.card", { class: "dash-problems-card" });
  section.append(el);
  const rerun = h("button", { type: "button", onclick: async () => {
      try { dash.doctor = await api("/api/doctor"); } catch (e) { dash.doctor = null; }
      api.update();
    }, class: "dash-small-13" }, "Re-run");
  const api = {
    update: () => {
      const doc = dash.doctor;
      const head = cardHead("Known problems", "/api/doctor", rerun);
      if (!doc) {
        const err = (dash.errors || {})["/api/doctor"];
        put(el, head, h("div", { class: "dash-fail-13" }, "The doctor could not run."),
          h("div", { class: "dash-muted-13-gap" }, err || "The endpoint did not answer."),
          cmdBlock("bernie doctor", "Copy the command"));
        return;
      }
      const bad = (doc.checks || []).filter((c) => c.status === "fail" || c.status === "warn");
      if (!bad.length) {
        put(el, head, h("div", { class: "dash-note-13-text2" },
          h("b", { class: "chart-strong" }, "Nothing is wrong on this host."),
          h("div", { class: "dash-muted-note" }, "Every check of the host passed.")));
        return;
      }
      const box = h("div");
      for (const c of bad) {
        const kind = c.status === "fail" ? "fail" : "warn";
        box.append(h("div", { class: "dash-check-row" },
          h("div", { class: "dash-check-head" }, dashPill(kind, c.status), h("b", { class: "dash-small-13" }, c.title || c.id)),
          c.detail ? h("div", { class: "dash-check-detail" }, c.detail) : null,
          c.fix ? h("div", { class: "dash-check-fix" }, cmdBlock(c.fix, "Copy the fix")) : null,
          c.doc ? h("a", { href: c.doc, target: "_blank", rel: "noopener noreferrer", class: "dash-check-doc" }, "the rule in the documentation") : null));
      }
      put(el, head, box);
    },
  };
  api.update();
  return api;
}

// dashStopRun stops the run that serves.
async function dashStopRun() {
  const run = dashLiveRun();
  if (!run) { toast("There is no run to stop."); return; }
  try {
    await api("/api/runs/" + encodeURIComponent(run.name), { method: "DELETE" });
    toast("The run " + run.name + " was stopped.", true);
    await dashLoad();
  } catch (e) {
    toast("Stop failed: " + (e.message || e));
  }
}

// dashRestartRun starts the same recipe again.
async function dashRestartRun() {
  const run = dashLiveRun();
  if (!run) { toast("There is no run to restart."); return; }
  try {
    await api("/api/runs", { method: "POST", body: { recipe: run.recipe, mode: run.mode, name: run.name } });
    toast("The run " + run.name + " is starting again.", true);
    await dashLoad();
  } catch (e) {
    toast("Restart failed: " + (e.message || e));
  }
}

// dashAsk sends one prompt to the live server and shows the answer, the time
// to the first token and the tokens per second.
async function dashAsk(text, box, button) {
  const run = dashLiveRun();
  if (!run) {
    box.replaceChildren(h("div", { class: "dash-muted" }, "Start the server first."));
    return;
  }
  button.disabled = true;
  const label = button.textContent;
  button.textContent = "Try it…";
  box.replaceChildren(h("div", { class: "dash-muted" }, "The server is answering…"));
  try {
    const res = await api("/api/smoke", { method: "POST", body: { prompt: text || "Hello", run: run.name } });
    box.dataset.answer = "1";
    box.replaceChildren(
      h("div", { class: "dash-answer-text" }, res.reply || "(no answer)"),
      h("div", { class: "dash-answer-meta" },
        (res.ttft_ms != null ? num(res.ttft_ms, 0) + " ms to the first token · " : "") +
        (res.tok_s != null ? num(res.tok_s) + " tok/s" : "")));
  } catch (e) {
    box.replaceChildren(
      h("div", { class: "dash-fail" }, "The server did not answer."),
      h("div", { class: "dash-muted-note" }, e.message || String(e)));
  } finally {
    button.disabled = false;
    button.textContent = label;
  }
}
