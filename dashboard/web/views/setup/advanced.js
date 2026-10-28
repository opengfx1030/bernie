// The Advanced panel of the Setup tab: the raw documents behind the page
// and the commands that produce them.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
"use strict";

/** setupAdvancedPanel is the disclosure at the end of the tab. */
function setupAdvancedPanel() {
  const body = h("div.setup-advanced-body", loading());
  const summary = h("summary", "Advanced");
  const panel = h("details.advanced.setup-advanced", summary, body);
  let loaded = false;
  panel.addEventListener("toggle", async () => {
    if (!panel.open || loaded) return;
    loaded = true;
    await loadSetupAdvanced(body);
  });
  // The panel follows the page-wide Advanced switch: the summary and the
  // rail button open the same panels.
  summary.addEventListener("click", (e) => {
    e.preventDefault();
    if (typeof toggleAdvanced === "function") toggleAdvanced();
    else panel.open = !panel.open;
  });
  return panel;
}

/** loadSetupAdvanced reads every document the tab used and paints it with
 *  the command that produces the same answer. */
async function loadSetupAdvanced(body) {
  const entries = [];
  const read = async (path) => {
    try {
      entries.push({ path: path, doc: await api(path) });
    } catch (e) {
      entries.push({ path: path, error: String(e.message || e) });
    }
  };
  await read("/api/onboarding");
  await read("/api/recipes");
  await read("/api/doctor");
  if (setupState.recipeID) {
    await read("/api/recipes/" + encodeURIComponent(setupState.recipeID) + "/plan");
  }
  put(body,
    h("p.setup-advanced-note", "The documents behind this page. An agent gets the same ones from the API."),
    entries.map(setupAdvancedDocument),
    h("h3", "The same thing in a terminal"),
    cmdBlock("bernie status --steps", "Copy the command"),
    cmdBlock("bernie status --json", "Copy the command"),
    cmdBlock("bernie recipe list", "Copy the command"),
    cmdBlock("bernie recipe plan " + (setupState.recipeID || "ID"), "Copy the command"),
    cmdBlock("bernie doctor", "Copy the command"));
}

/** setupAdvancedDocument is one raw document with its endpoint and a copy
 *  button. */
function setupAdvancedDocument(entry) {
  const text = entry.error
    ? JSON.stringify({ error: entry.error }, null, 2)
    : JSON.stringify(entry.doc, null, 2);
  return h("div.setup-advanced-document",
    h("div.setup-advanced-head",
      h("span.setup-advanced-path", entry.path),
      h("span.grow"),
      jsonLink(entry.path),
      copyBtn(text)),
    h("pre.setup-advanced-json", text));
}
