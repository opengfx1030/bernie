// The one action of the Setup tab: it runs the whole path that bernie can
// do on this host. The module comes when the reader presses the button, so
// the first paint does not carry it.
"use strict";

// setupPathLoaded waits for steps.js to be there before this module uses
// any of its functions. setupEnsureModule loads steps.js on the first call.
const setupPathLoaded = setupEnsureModule("views/setup/steps.js");

/** setupRunPath starts the whole path: the setup steps, the server and the
 *  check that proves it answers. It is one button and one job. */
async function setupRunPath() {
  await setupPathLoaded;
  setupProgress.path = { phase: "Starting\u2026", started: Date.now() };
  setupLogs.path = [];
  setupPaintPath();
  let job = null;
  try {
    job = await api("/api/onboarding/path/run", {
      method: "POST",
      body: { recipe: setupState.recipeID, mode: setupState.mode, set: setupState.sets },
    });
  } catch (e) {
    setupProgress.path = { phase: "Failed: " + (e.message || e), failed: true };
    setupPaintPath();
    return;
  }
  if (!job || job.root_required) {
    // The path stops at the step that needs root: the page shows the exact
    // command and the Re-check that reads the host again.
    setupProgress.path = {
      phase: "This step needs root. Run the command below as root, then re-check.",
      failed: true,
      commands: (job && job.commands) || [],
    };
    setupPaintPath();
    return;
  }
  setupState.pathJobID = job.id;
  followSetupJob({ id: "path" }, job, { onRefresh: () => { setupRefreshRun("path"); loadSetupDocument(); } });
}

/** setupStopPath stops the run of the path. The job ends and the tab says
 *  so; nothing is hidden. */
async function setupStopPath() {
  await setupPathLoaded;
  const id = setupState.pathJobID;
  if (!id) return;
  try {
    await api("/api/jobs/" + encodeURIComponent(id) + "/stop", { method: "POST", body: {} });
  } catch (e) {
    toast("The run could not be stopped: " + (e.message || e));
    return;
  }
  setupProgress.path = { phase: "Stopped.", failed: true };
  setupPaintPath();
}

/** setupAttachPath re-attaches to the job of the whole path after a
 *  reload. The page knows the id, so the run survives the browser. */
async function setupAttachPath(doc) {
  await setupPathLoaded;
  const path = doc && doc.path;
  if (!path || !path.id) return;
  setupState.pathJobID = path.id;
  setupProgress.path = { phase: path.phase || "Working\u2026" };
  setupPaintPath();
  if (path.state !== "running") return;
  setupLogs.path = [];
  followSetupJob({ id: "path" }, { id: path.id }, { onRefresh: () => { setupRefreshRun("path"); loadSetupDocument(); } });
}

/** setupRunRows draws the steps of the setup run with their state and the
 *  time they took. The record of the run is the one source (spec §6.4). */
function setupRunRows() {
  const run = setupState.run;
  if (!run || !run.steps || !run.steps.length) return null;
  return h("div.step-run", run.steps.map((s) => h("div.step-run-row",
    h("span", s.id + "  " + s.state + "  " + (s.duration_s > 0 ? setupDuration(s.duration_s) : "")))));
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
    setupRunRows(),
    commands.map((c) => cmdBlock(c, "Copy the command")),
    running ? h("button.small", { type: "button", onclick: setupStopPath }, "Stop") : null,
    commands.length ? h("button.small", { type: "button", onclick: () => loadSetupDocument() }, "Re-check") : null);
}
