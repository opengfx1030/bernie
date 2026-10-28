"use strict";
// ---------- Overview ----------

async function viewOverview(root) {
  const hostCard = h("div.card", cardHead("Host", "/api/hw"), loading());
  const docCard = h("div.card", cardHead("Doctor", "/api/doctor"), loading());
  const runCard = h("div.card", cardHead("Runs", "/api/runs"), loading());
  put(root, h("div.cols", h("div", hostCard, h("div.gap"), runCard), docCard));

  loadHW(true).then((hw) => put(hostCard, cardHead("Host", "/api/hw"), hostSummary(hw))).catch((e) => put(hostCard, cardHead("Host", "/api/hw"), failed(e)));
  api("/api/runs").then((d) => put(runCard, cardHead("Runs", "/api/runs", h("a.small", { href: "#runs" }, "manage")), runsTable(runsOf(d), true)))
    .catch((e) => put(runCard, cardHead("Runs", "/api/runs"), failed(e)));
  loadDoctor(docCard);
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

async function loadDoctor(card) {
  const title = () => cardHead("Doctor", "/api/doctor", h("button.small", { onclick: () => { put(card, title(), loading()); loadDoctor(card); } }, "Re-run"));
  put(card, title(), loading());
  try {
    const rep = await api("/api/doctor");
    const sum = rep.summary || {};
    const checks = rep.checks || [];
    const bad = checks.filter((c) => c.status === "fail" || c.status === "warn")
      .sort((a, b) => (a.status === "fail" ? 0 : 1) - (b.status === "fail" ? 0 : 1));
    put(card, title(),
      h("div.counts", ["fail", "warn", "ok", "skip"].map((s) => h("span.badge", { class: "b-" + s }, (sum[s] || 0) + " " + s))),
      bad.length ? bad.map(checkRow) : h("div.empty", "No failing or warning checks."),
      h("details", h("summary.muted.small", "All checks (" + checks.length + ")"), checks.map(checkRow)));
  } catch (e) {
    put(card, title(), failed(e));
  }
}

function checkRow(c) {
  return h("div.check",
    h("div.t", badge(c.status), h("b", c.title), h("span.id", c.id)),
    c.detail ? h("div.muted.small", c.detail) : null,
    c.fix ? cmdBlock(c.fix, "Copy the fix command") : null,
    c.doc ? h("a.small", { href: c.doc, target: "_blank", rel: "noopener noreferrer" }, "docs") : null);
}
