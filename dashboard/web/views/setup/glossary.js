// The words of the glossary that stand on the Setup tab.
"use strict";

/** paintSetupGlossary lists the words of the glossary that stand on this
 *  page, with what each one means (spec §11). */
async function paintSetupGlossary(root) {
  const box = setupState.glossaryBox;
  if (!box || !root) return;
  let list = null;
  try { list = await api("/api/glossary"); } catch (e) { return; }
  const words = (list && list.words) || [];
  const text = root.textContent || "";
  const here = words.filter((w) => text.indexOf(w.term) >= 0);
  if (!here.length) {
    put(box, h("p.small.muted", "This page has no word that needs an explanation."));
    return;
  }
  put(box, h("dl.setup-glossary-list", here.map((w) =>
    h("div.setup-glossary-word",
      h("dt", w.term, tooltip(w.meaning, "What " + w.term + " means")),
      h("dd", w.meaning)))));
}
