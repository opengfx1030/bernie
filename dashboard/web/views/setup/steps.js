// The actions of the steps of the checklist: start a step, follow the job
// it starts and show the progress under the step.
"use strict";

// setupProgress is the live progress of a step, by step id.
const setupProgress = {};

// setupLogs is the output of a step, by step id.
const setupLogs = {};

// setupStreams holds the open job streams, by step id.
const setupStreams = {};

/**
 * runSetupStep starts one step of the checklist. The server answers with a
 * job; the job stream carries the output and the end of the work.
 */
async function runSetupStep(step, options = {}) {
  const id = step.id;
  setupProgress[id] = { phase: "Starting…", pct: 0 };
  setupLogs[id] = [];
  paintSetupProgress(id);
  let job = null;
  try {
    job = await api("/api/onboarding/" + encodeURIComponent(id) + "/run", { method: "POST", body: {} });
  } catch (e) {
    setupProgress[id] = { phase: "Failed: " + (e.message || e), pct: 0, failed: true };
    paintSetupProgress(id);
    return;
  }
  if (!job || job.root_required) {
    setupProgress[id] = { phase: "This step needs root. Run it in a terminal, then re-check.", pct: 0, failed: true };
    paintSetupProgress(id);
    return;
  }
  followSetupJob(step, job, options);
}

/** followSetupJob follows the output of a job and refreshes the checklist
 *  when the job ends. */
function followSetupJob(step, job, options) {
  const id = step.id;
  const stop = () => {
    const stream = setupStreams[id];
    if (stream) {
      stream.close();
      delete setupStreams[id];
    }
  };
  const finish = (name, raw) => {
    stop();
    let doc = null;
    try { doc = typeof raw === "string" ? JSON.parse(raw) : raw; } catch (e) { doc = null; }
    const previous = setupProgress[id] || {};
    if (name === "done") {
      setupProgress[id] = { phase: "Done.", pct: 100 };
    } else {
      setupProgress[id] = {
        phase: "Failed: " + ((doc && doc.error) || "the step did not finish."),
        pct: previous.pct || 0,
        failed: true,
      };
      if (doc && Array.isArray(doc.lines) && doc.lines.length) setupLogs[id] = doc.lines.slice();
    }
    paintSetupProgress(id);
    if (options.onRefresh) options.onRefresh();
  };
  const stream = openJobEvents(job.id, {
    line: (ev) => {
      const text = ev.data || "";
      if (!setupLogs[id]) setupLogs[id] = [];
      setupLogs[id].push(text);
      const current = setupProgress[id] || (setupProgress[id] = {});
      current.phase = text;
      paintSetupProgress(id);
      loadSetupJobProgress(job.id, id);
      if (options.onRefresh) options.onRefresh();
    },
    done: (ev) => finish("done", ev.data),
    failed: (ev) => finish("failed", ev.data),
  });
  setupStreams[id] = stream;
  loadSetupJobProgress(job.id, id);
}

/** loadSetupJobProgress reads the numbers of a running job: the phase, the
 *  fraction and the rate of the work. */
async function loadSetupJobProgress(jobID, stepID) {
  let job = null;
  try { job = await api("/api/jobs/" + encodeURIComponent(jobID)); } catch (e) { return; }
  const p = job && job.progress;
  if (!p) return;
  const current = setupProgress[stepID] || (setupProgress[stepID] = {});
  current.pct = setupProgressPercent(p);
  current.detail = setupProgressDetail(p, current.pct);
  if (p.phase) current.phase = p.phase;
  else if (!current.phase) current.phase = setupProgressSentence(p);
  paintSetupProgress(stepID);
}

/** setupProgressPercent is the fraction of the work that is done, in
 *  percent. Bytes come first; files are the fallback. */
function setupProgressPercent(p) {
  let done = 0;
  let total = 0;
  if (p.bytes_total) {
    done = p.bytes_done || 0;
    total = p.bytes_total;
  } else if (p.files_total) {
    done = p.files_done || 0;
    total = p.files_total;
  }
  if (!total) return 0;
  return Math.max(0, Math.min(100, Math.round((done * 100) / total)));
}

/** setupProgressSentence names the work in one plain sentence. */
function setupProgressSentence(p) {
  if (p.phase) return p.phase;
  if (p.files_total) {
    return "Downloading the model weights, " + (p.files_done || 0) + " of " + p.files_total + " files.";
  }
  if (p.bytes_total) {
    return "Downloading the model weights, " + bytes(p.bytes_done || 0) + " of " + bytes(p.bytes_total) + ".";
  }
  return "Working…";
}

/** setupProgressDetail is the line under the bar: the percent, the size,
 *  the rate and the time that is left. */
function setupProgressDetail(p, pct) {
  let text = pct + "%";
  if (p.bytes_total) {
    text += " - " + bytes(p.bytes_done || 0) + " of " + bytes(p.bytes_total);
    if (p.rate_bps) text += " at " + setupRate(p.rate_bps);
  } else if (p.files_total) {
    text += " - " + (p.files_done || 0) + " of " + p.files_total + " files";
  }
  if (p.eta_s) text += " - about " + setupDuration(p.eta_s) + " left";
  return text;
}

/** setupRate is a download rate in one unit. */
function setupRate(bps) {
  if (bps >= 1000 * 1000) return (bps / (1000 * 1000)).toFixed(1) + " MB/s";
  if (bps >= 1000) return (bps / 1000).toFixed(1) + " kB/s";
  return bps + " B/s";
}

/** setupDuration is a number of seconds in words. */
function setupDuration(seconds) {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return s + " s";
  return Math.round(s / 60) + " min";
}

/** paintSetupProgress writes the live progress into the row of a step. */
function paintSetupProgress(id) {
  const box = document.querySelector('[data-step="' + id + '"] .step-progress');
  if (!box) return;
  const p = setupProgress[id] || {};
  const sentence = box.querySelector(".progress-sentence");
  if (!sentence) {
    paintSetupTimeline();
    return;
  }
  sentence.textContent = p.phase || "";
  const bar = box.querySelector(".step-bar");
  if (bar) {
    const pct = typeof p.pct === "number" ? p.pct : 0;
    bar.setAttribute("aria-valuenow", String(pct));
    const fill = bar.querySelector("i");
    if (fill) fill.style.width = pct + "%";
  }
  const detail = box.querySelector(".step-progress-detail");
  if (detail) detail.textContent = p.detail || "";
  const lines = setupLogs[id] || [];
  let log = box.querySelector(".step-log");
  if (lines.length && !log) {
    paintSetupTimeline();
    return;
  }
  if (!log) return;
  while (log.childElementCount > lines.length) log.lastElementChild.remove();
  while (log.childElementCount < lines.length) log.append(h("div", ""));
  lines.forEach((line, i) => {
    if (log.children[i].textContent !== line) log.children[i].textContent = line;
  });
}

/** setupStopStreams closes the job streams of the tab. */
function setupStopStreams() {
  for (const id of Object.keys(setupStreams)) {
    const stream = setupStreams[id];
    if (stream) stream.close();
  }
  for (const id of Object.keys(setupStreams)) delete setupStreams[id];
}
