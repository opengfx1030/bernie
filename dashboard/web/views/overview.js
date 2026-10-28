"use strict";
// ---------- Overview ----------

async function viewOverview(root) {
  const head = stateHead("Overview", h("button.primary", { type: "button", onclick: () => viewOverview(root) }, "Re-check this host"));
  const hostCard = h("div.card", cardHead("Host", "/api/hw"), loading());
  const docCard = h("div.card", cardHead("Doctor", "/api/doctor"), loading());
  const runCard = h("div.card", cardHead("Runs", "/api/runs"), loading());
  put(root, head, h("div.cols", h("div", hostCard, h("div.gap"), runCard), docCard));

  loadHW(true).then((hw) => put(hostCard, cardHead("Host", "/api/hw"), hostSummary(hw))).catch((e) => put(hostCard, cardHead("Host", "/api/hw"), failed(e)));
  api("/api/runs").then((d) => put(runCard, cardHead("Runs", "/api/runs", h("a.small", { href: "#runs" }, "manage")), runsTable(runsOf(d), true)))
    .catch((e) => put(runCard, cardHead("Runs", "/api/runs"), failed(e)));
  loadDoctor(docCard, head);
}

function hostSummary(hw) {
  const ho = hw.host || {};
  const cpu = ho.cpu || {};
  const gpus = hw.gpus || [];
  const byName = {};
  for (const g of gpus) {
    const k = (g.name || "GPU") + " (" + g.gfx + ")";
    byName[k] = (byName[k] || 0) + 1;
  }
  const engines = (ho.engines || []).map((e) => e.name + " " + (e.version || "") + (e.usable ? "" : " (not usable)") + (e.rootless ? ", rootless" : ""));
  return h("dl.kv",
    h("dt", "Host"), h("dd", ho.hostname || "–"),
    h("dt", "OS"), h("dd", (ho.os && (ho.os.pretty || ho.os.id)) || "–", ho.pkg_manager ? h("span.muted", " · " + ho.pkg_manager) : null),
    h("dt", "Kernel"), h("dd", ho.kernel || "–"),
    h("dt", "CPU"), h("dd", cpu.model || "–", cpu.threads ? h("span.muted", ` · ${cpu.sockets || 1}× socket, ${cpu.cores} cores / ${cpu.threads} threads`) : null),
    h("dt", "RAM"), h("dd", bytes(ho.ram_bytes)),
    h("dt", "GPUs"), h("dd", gpus.length ? Object.entries(byName).map(([k, n]) => h("div", n + "× " + k)) : h("span.muted", "none found")),
    h("dt", "Engines"), h("dd", engines.length ? engines.join("; ") : h("span.muted", "none")),
    ho.selinux ? [h("dt", "SELinux"), h("dd", ho.selinux)] : null,
  );
}

async function loadDoctor(card, head) {
  const title = () => cardHead("Doctor", "/api/doctor", h("button.small", { onclick: () => { put(card, title(), loading()); loadDoctor(card, head); } }, "Re-run"));
  put(card, title(), loading());
  try {
    const rep = await api("/api/doctor");
    const sum = rep.summary || {};
    const checks = rep.checks || [];
    const bad = checks.filter((c) => c.status === "fail" || c.status === "warn")
      .sort((a, b) => (a.status === "fail" ? 0 : 1) - (b.status === "fail" ? 0 : 1));
    if (head) setHead(head, doctorHeadline(sum), doctorLede(sum));
    put(card, title(),
      h("div.counts", ["fail", "warn", "ok", "skip"].map((s) => h("span.badge", { class: "b-" + s }, (sum[s] || 0) + " " + s))),
      bad.length ? bad.map(checkRow) : emptyState("Every check passed.", "The doctor looks for the things that keep a server from running on this host: the GPUs, the driver, the ROCm runtime and the Python environment."),
      h("details", h("summary.muted.small", "All checks (" + checks.length + ")"), checks.map(checkRow)));
  } catch (e) {
    put(card, title(), failed(e));
  }
}

// doctorHeadline says in one sentence what the doctor found.
function doctorHeadline(sum) {
  if (sum.fail) return sum.fail + (sum.fail === 1 ? " check needs" : " checks need") + " a fix";
  if (sum.warn) return "Nothing is broken, " + sum.warn + (sum.warn === 1 ? " check warns" : " checks warn");
  return "This host passes every check";
}

// doctorLede says what that means for the next step.
function doctorLede(sum) {
  if (sum.fail) return "The doctor checks this host for the things that keep a server from running. The fix and the command to run it are in the card below.";
  if (sum.warn) return "The doctor checks this host for the things that keep a server from running. Nothing failed; the warnings are worth a look.";
  return "The doctor checks this host for the things that keep a server from running: the GPUs, the driver, the ROCm runtime and the Python environment.";
}

function checkRow(c) {
  return h("div.check",
    h("div.t", badge(c.status), h("b", c.title), h("span.id", c.id)),
    c.detail ? h("div.muted.small", c.detail) : null,
    c.fix ? cmdBlock(c.fix, "Copy the fix command") : null,
    c.doc ? h("a.small", { href: c.doc, target: "_blank", rel: "noopener noreferrer" }, "docs") : null);
}
