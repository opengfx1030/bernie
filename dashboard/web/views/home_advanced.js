"use strict";
// ---------- Home: the advanced disclosure ----------
// The disclosure holds the raw document behind the page and the commands
// that produce it. It is closed by default: the beginner path is one button,
// the expert path is the same screen plus this panel, and the agent path is
// the same JSON.

// advancedPanel is the disclosure under the hero of the Home tab.
function advancedPanel() {
  const body = h("div.advanced-body", loading());
  let loaded = false;
  const det = h("details.advanced", h("summary", "Advanced"), body);
  det.addEventListener("toggle", async () => {
    if (!det.open || loaded) return;
    loaded = true;
    try {
      const doc = await api("/api/onboarding");
      const json = JSON.stringify(doc, null, 2);
      put(body,
        h("p.small.muted", "The document behind this page. An agent gets the same one from GET /api/onboarding."),
        h("div.row", jsonLink("/api/onboarding"),
          h("button.small.copy", { type: "button", "data-copy": json, "aria-label": "Copy the JSON" }, "Copy the JSON")),
        h("pre", json),
        h("h3", "The same thing in a terminal"),
        cmdBlock("bernie status --steps", "Copy the command"),
        cmdBlock("bernie status --json", "Copy the command"),
        cmdBlock("bernie doctor --json", "Copy the command"));
    } catch (e) {
      put(body, failed(e));
    }
  });
  return det;
}
