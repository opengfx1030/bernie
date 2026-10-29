"use strict";
// ---------- The readiness checklist ----------
// The checklist of `bernie topo --check` (spec §4.5): one row per item, the
// status in words, the cause and the fix with its copy button.

/** topoChecklist is the readiness checklist of `bernie topo --check`. */
function topoChecklist(data) {
  if (!data) {
    const err = (topo.errors || {})["/api/topo/check"];
    return h("div",
      h("div.topo-note", "The checklist is not available."),
      err ? h("div.topo-note-small", err) : null,
      h("button", { type: "button", onclick: () => topoCheck() }, "Try again"));
  }
  const items = data.checklist || [];
  const sum = data.summary || {};
  const failed = sum.fail || 0;
  const skipped = sum.skip || 0;
  const ready = sum.ok || 0;
  const box = h("div");
  let verdict;
  let kind;
  if (failed) {
    kind = "fail";
    verdict = "Multi-GPU copies will not work: " + failed + (failed === 1 ? " check failed." : " checks failed.");
  } else if (!ready && skipped) {
    // Every check was skipped: there is nothing to copy on this host.
    kind = "skip";
    verdict = "There is nothing to copy on this host: every check was skipped.";
  } else {
    kind = "ok";
    verdict = "Multi-GPU copies will work on this host.";
  }
  box.append(h("div.topo-verdict",
    topoPill(kind, kind === "fail" ? "not ready" : kind === "skip" ? "nothing to check" : "ready"),
    h("span.topo-verdict-text", verdict)));
  if (!items.length) {
    box.append(h("div.topo-note", "Nothing to check on this host."));
    return box;
  }
  for (const it of items) {
    const kind = it.status === "ok" ? "ok" : it.status === "warn" ? "warn" : it.status === "fail" ? "fail" : "skip";
    box.append(h("div.topo-check-row",
      h("div.topo-check-head", topoPill(kind, it.status === "skip" ? "skip" : it.status),
        h("b.topo-check-title", it.title || it.id)),
      it.detail ? h("div.topo-check-detail", it.detail) : null,
      it.fix ? h("div.topo-check-fix", cmdBlock(it.fix, "Copy the fix")) : null,
      it.doc ? h("a", { href: it.doc, target: "_blank", rel: "noopener noreferrer", class: "topo-check-doc" },
        "the rule in the documentation") : null));
  }
  return box;
}
