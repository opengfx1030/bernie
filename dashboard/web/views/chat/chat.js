"use strict";
// ---------- Chat (planned) ----------
// The Chat tab is planned and is not available yet. It shows one honest
// paragraph and no control: there is nothing to run behind it.

/** viewChat opens the placeholder of the Chat tab. */
function viewChat(root) {
  put(root, h("section.state-head",
    h("p.eyebrow", "Chat"),
    h("h1.display", "CHAT IS PLANNED."),
    h("p.lede", "Chat is planned and not available yet. Until then use the endpoint on the Dashboard tab with any OpenAI-compatible client.")));
}
