// The "Needs you" section of the Setup tab: the items only the operator
// can do, above the path. The file is loaded lazily so the first paint
// does not carry it. See ia.md §5 R40.
"use strict";

/** paintSetupNeedsYouRender writes the "Needs you" section: the items only the
 *  operator can do, above the path. The section is hidden when there is
 *  nothing to do; the timeline below carries the routine steps. */
function paintSetupNeedsYouRender(doc) {
  const box = setupState.needsYou;
  if (!box) return;
  if (!doc) doc = setupState.doc;
  if (!doc) return;
  const path = (doc.steps || []).filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
  // A step belongs here when bernie cannot run it itself: it is blocked, or
  // it needs a root command only the operator can run.
  const items = path.filter((s) => s.state === "blocked"
    || (s.state !== "done" && s.state !== "skipped" && (s.root_commands || []).length));
  if (!items.length) {
    box.hidden = true;
    put(box);
    return;
  }
  put(box,
    sectionLabel("Needs you"),
    h("p.small.muted", "A step bernie cannot run itself. Copy the command, run it as root in a terminal, then press Re-check."),
    h("div.needs-you-list", items.map(setupNeedsYouItem)),
    h("div.needs-you-foot",
      h("button", { type: "button", onclick: () => loadSetupDocument() }, "Re-check"),
      h("a.small", { href: "#setup-doctor" }, "the doctor report")));
  box.hidden = false;
}

/** setupNeedsYouItem is one row of the "Needs you" section: the state
 *  word, the title, the reason, the root command (if any) with a copy
 *  button. The row never blames the reader (Setup rule 9). */
function setupNeedsYouItem(step) {
  const blocked = step.state === "blocked";
  const root = (step.root_commands || [])[0];
  return h("div.needs-you-item",
    h("div.needs-you-head",
      statePill(blocked ? "fail" : "warn", blocked ? "blocked" : "root"),
      h("span.needs-you-title", step.title || step.id || "")),
    step.summary ? h("p.needs-you-summary", step.summary) : null,
    root ? h("div.needs-you-command",
      cmdBlock(root, "Copy the command"),
      h("p.small.muted", "run it as root in a terminal.")) : null);
}
