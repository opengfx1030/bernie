"use strict";
// ---------- Fleet (planned) ----------
// The Fleet tab is planned and is not available yet. It shows one honest
// paragraph and no control: bernie manages one host today.

/** viewFleet opens the placeholder of the Fleet tab. */
function viewFleet(root) {
  put(root, h("section.state-head",
    h("p.eyebrow", "Fleet"),
    h("h1.display", "FLEET IS PLANNED."),
    h("p.lede", "Fleet is planned and not available yet: bernie manages one host today.")));
}
