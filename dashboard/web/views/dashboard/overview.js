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
  const el = h("div.card", { style: "background:var(--surface);border:1px solid var(--border);border-radius:18px;padding:20px 22px" });
  section.append(el);
  const box = h("div", { style: "display:grid;grid-template-columns:minmax(0,1fr) minmax(260px,420px);gap:28px;align-items:start" });
  const left = h("div", { style: "min-width:0" });
  const right = h("div", { style: "min-width:0;border-left:1px solid var(--border);padding-left:28px" });
  el.append(box);
  box.append(left, right);

  const name = h("h3", { style: "font:900 22px/1.1 var(--font-display);margin:0;overflow-wrap:anywhere" }, "No run is serving.");
  const pill = h("span", { style: "display:inline-flex" });
  const title = h("div", { style: "display:flex;gap:12px;align-items:center;flex-wrap:wrap" }, name, pill);
  const phase = h("p", { style: "color:var(--text-2);font-size:14px;margin:8px 0 10px" }, "");
  const endpoint = h("div");
  const age = h("span", { style: "font-size:13px;color:var(--muted)" }, "");
  const stop = h("button.ghost", { type: "button", style: "background:transparent;border:1px solid var(--border-strong);border-radius:8px;padding:7px 12px;cursor:pointer;color:var(--text-2)" }, "Stop");
  const restart = h("button.ghost", { type: "button", style: "background:transparent;border:1px solid var(--border-strong);border-radius:8px;padding:7px 12px;cursor:pointer;color:var(--text-2)" }, "Restart");
  const actions = h("div", { style: "display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:10px" }, age, stop, restart);
  const findings = h("div", { style: "margin-top:12px" });
  left.append(title, phase, endpoint, actions, findings);

  const ask = h("div", { style: "font-size:13px;color:var(--text-2);margin-bottom:6px" }, "Ask the server something.");
  const prompt = h("input", { type: "text", value: "Hello", "aria-label": "The prompt to send to the server",
    style: "width:100%;padding:9px 11px;background:var(--surface-2);border:1px solid var(--border-strong);border-radius:8px" });
  const send = h("button", { type: "button", style: "background:var(--accent);border:1px solid var(--accent);color:var(--on-accent);font-weight:800;text-transform:uppercase;border-radius:8px;padding:9px 16px;cursor:pointer;box-shadow:3px 3px 0 color-mix(in srgb, var(--accent) 45%, #000)" }, "Try it");
  const answer = h("div", { style: "margin-top:10px;font-size:13px;color:var(--text-2)" });
  right.append(ask, prompt, h("div", { style: "margin-top:8px" }, send), answer);

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
        answer.replaceChildren(h("div", { style: "color:var(--muted)" }, "Start the server first."));
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
        answer.replaceChildren(h("div", { style: "color:var(--fail)" }, "The server did not answer."),
          h("div", { style: "color:var(--muted);margin-top:2px" }, live.error || "The process is alive but /health stays silent."));
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
    return h("div", { style: "font-size:13px;color:var(--muted)" }, "No known failure in this log.");
  }
  const box = h("div.findings");
  for (const f of list) {
    const kind = f.severity === "error" || f.severity === "fail" ? "fail" : f.severity === "warn" ? "warn" : "info";
    box.append(h("div.finding", { style: "border:1px solid var(--border);border-left:3px solid var(--fail);border-radius:8px;padding:8px 10px;margin:0 0 8px" },
      h("div", { style: "display:flex;gap:8px;align-items:center" }, dashPill(kind, f.severity || "warn"), h("span", { style: "font-family:var(--font-mono);font-size:12px;color:var(--muted)" }, f.id)),
      f.cause ? h("div", { style: "font-size:13px;color:var(--text-2);margin-top:4px" }, f.cause) : null,
      f.fix ? cmdBlock(f.fix, "Copy the fix") : null,
      f.doc ? h("a", { href: f.doc, target: "_blank", rel: "noopener noreferrer", style: "font-size:12px" }, "the rule in the documentation") : null));
  }
  return box;
}

// dashProblems is the section with the known problems of the host: the
// checks of the doctor that fail or warn.
function dashProblems(section) {
  const el = h("div.card", { style: "background:var(--surface);border:1px solid var(--border);border-radius:18px;padding:20px 22px" });
  section.append(el);
  const rerun = h("button", { type: "button", onclick: async () => {
      try { dash.doctor = await api("/api/doctor"); } catch (e) { dash.doctor = null; }
      api.update();
    }, style: "background:transparent;border:1px solid var(--border-strong);border-radius:8px;padding:6px 12px;cursor:pointer;color:var(--text-2);font-size:13px" }, "Re-run");
  const api = {
    update: () => {
      const doc = dash.doctor;
      const head = cardHead("Known problems", "/api/doctor", rerun);
      if (!doc) {
        const err = (dash.errors || {})["/api/doctor"];
        put(el, head, h("div", { style: "color:var(--fail);font-size:13px" }, "The doctor could not run."),
          h("div", { style: "color:var(--muted);font-size:13px;margin:4px 0 8px" }, err || "The endpoint did not answer."),
          cmdBlock("bernie doctor", "Copy the command"));
        return;
      }
      const bad = (doc.checks || []).filter((c) => c.status === "fail" || c.status === "warn");
      if (!bad.length) {
        put(el, head, h("div", { style: "font-size:13px;color:var(--text-2)" },
          h("b", { style: "color:var(--text)" }, "Nothing is wrong on this host."),
          h("div", { style: "color:var(--muted);margin-top:2px" }, "Every check of the host passed.")));
        return;
      }
      const box = h("div");
      for (const c of bad) {
        const kind = c.status === "fail" ? "fail" : "warn";
        box.append(h("div", { style: "display:grid;grid-template-columns:auto 1fr;gap:4px 12px;padding:10px 0;border-bottom:1px solid var(--border)" },
          h("div", { style: "display:flex;gap:8px;align-items:baseline" }, dashPill(kind, c.status), h("b", { style: "font-size:13px" }, c.title || c.id)),
          c.detail ? h("div", { style: "grid-column:2;color:var(--text-2);font-size:13px" }, c.detail) : null,
          c.fix ? h("div", { style: "grid-column:2;margin-top:6px" }, cmdBlock(c.fix, "Copy the fix")) : null,
          c.doc ? h("a", { href: c.doc, target: "_blank", rel: "noopener noreferrer", style: "grid-column:2;font-size:12px;margin-top:4px" }, "the rule in the documentation") : null));
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
    box.replaceChildren(h("div", { style: "color:var(--muted)" }, "Start the server first."));
    return;
  }
  button.disabled = true;
  const label = button.textContent;
  button.textContent = "Try it…";
  box.replaceChildren(h("div", { style: "color:var(--muted)" }, "The server is answering…"));
  try {
    const res = await api("/api/smoke", { method: "POST", body: { prompt: text || "Hello", run: run.name } });
    box.replaceChildren(
      h("div", { style: "color:var(--text);white-space:pre-wrap;overflow-wrap:anywhere" }, res.reply || "(no answer)"),
      h("div", { style: "color:var(--muted);margin-top:6px;font-variant-numeric:tabular-nums" },
        (res.ttft_ms != null ? num(res.ttft_ms, 0) + " ms to the first token · " : "") +
        (res.tok_s != null ? num(res.tok_s) + " tok/s" : "")));
  } catch (e) {
    box.replaceChildren(
      h("div", { style: "color:var(--fail)" }, "The server did not answer."),
      h("div", { style: "color:var(--muted);margin-top:2px" }, e.message || String(e)));
  } finally {
    button.disabled = false;
    button.textContent = label;
  }
}
