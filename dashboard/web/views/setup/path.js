// The one action of the Setup tab: it runs the whole path that bernie can
// do on this host. The module comes when the reader presses the button, so
// the first paint does not carry it.
"use strict";

/** setupRunPath starts the whole path: the setup steps, the server and the
 *  check that proves it answers. It is one button and one job. */
async function setupRunPath() {
  setupProgress.path = { phase: "Starting\u2026", started: Date.now() };
  setupLogs.path = [];
  paintSetupPath();
  let job = null;
  try {
    job = await api("/api/onboarding/path/run", {
      method: "POST",
      body: { recipe: setupState.recipeID, mode: setupState.mode, set: setupState.sets },
    });
  } catch (e) {
    setupProgress.path = { phase: "Failed: " + (e.message || e), failed: true };
    paintSetupPath();
    return;
  }
  if (!job || job.root_required) {
    // The path stops at the step that needs root: the page shows the exact
    // command and the Re-check that reads the host again.
    setupProgress.path = {
      phase: "This step needs root. Run the command below as root, then press Re-check.",
      failed: true,
      commands: (job && job.commands) || [],
    };
    paintSetupPath();
    return;
  }
  setupState.pathJobID = job.id;
  followSetupJob({ id: "path" }, job, { onRefresh: () => { paintSetupPath(); loadSetupDocument(); } });
}

/** setupStopPath stops the run of the path. The job ends and the tab says
 *  so; nothing is hidden. */
async function setupStopPath() {
  const id = setupState.pathJobID;
  if (!id) return;
  try {
    await api("/api/jobs/" + encodeURIComponent(id) + "/stop", { method: "POST", body: {} });
  } catch (e) {
    toast("The run could not be stopped: " + (e.message || e));
    return;
  }
  setupProgress.path = { phase: "Stopped.", failed: true };
  paintSetupPath();
}

/** setupAttachPath re-attaches to the job of the whole path after a
 *  reload. The page knows the id, so the run survives the browser. */
function setupAttachPath(doc) {
  const path = doc && doc.path;
  if (!path || !path.id) return;
  setupState.pathJobID = path.id;
  setupProgress.path = { phase: path.phase || "Working\u2026" };
  paintSetupPath();
  if (path.state !== "running") return;
  setupLogs.path = [];
  followSetupJob({ id: "path" }, { id: path.id }, { onRefresh: () => { paintSetupPath(); loadSetupDocument(); } });
}

/** setupPaintPath paints the run of the path: the step it is on, the phase
 *  and the Stop control. */
function setupPaintPath() {
  const box = setupState.pathBox;
  if (!box) return;
  const live = setupProgress.path;
  if (!live || !live.phase) {
    put(box);
    return;
  }
  const running = !live.failed && live.phase !== "Done." && live.phase !== "Stopped.";
  const commands = live.commands || [];
  put(box,
    h("p.small.muted", running ? "bernie is setting up this box." : (live.failed ? "The setup stopped." : "The setup is done.")),
    h("p.step-progress-sentence", live.phase),
    live.detail ? h("p.small.muted", live.detail) : null,
    commands.map((c) => cmdBlock(c, "Copy the command")),
    running ? h("button.small", { type: "button", onclick: setupStopPath }, "Stop") : null,
    commands.length ? h("button.small", { type: "button", onclick: () => loadSetupDocument() }, "Re-check") : null);
}
