"use strict";
// The preferences panel of the Advanced tab (ia.md R28).
function advPrefsCard() {
  const card = h("div.card");
  const body = h("div.adv-prefs-body");
  const view = {
    el: card,
    reload: async () => {
      try {
        const doc = await api("/api/prefs");
        view.update(doc);
        return doc;
      } catch (e) { put(body, failed(e)); return null; }
    },
    update: (doc) => { put(body, prefsList(doc)); },
  };
  put(card, cardHead("The preferences", "/api/prefs"), body);
  return view;
}
function prefsList(doc) {
  if (!doc || !doc.items || !doc.items.length) {
    return emptyState("No preferences were read.",
      "The preferences panel reads /api/prefs; the call failed or the config file is empty.",
      "bernie config show");
  }
  const wrap = h("div.adv-prefs-rows");
  for (const it of doc.items) wrap.append(prefRow(it));
  return h("div.adv-prefs-list", h("p.adv-prefs-path", "File: ", h("code", doc.path || "–")), wrap);
}
function prefRow(it) {
  const value = h("code.adv-prefs-value", it.value || "–");
  const source = h("span.adv-prefs-source", it.source || "default");
  const resolved = h("p.adv-prefs-resolved");
  setResolved(resolved, it);
  const choices = Array.isArray(it.choices) && it.choices.length
    ? h("p.adv-prefs-choices", "Valid: ", it.choices.join(", ")) : null;
  const hint = it.hint ? h("p.adv-prefs-hint", it.hint) : null;
  const input = h("input.adv-prefs-input", {
    type: "text", "aria-label": "New value for " + it.key,
    placeholder: it.value ? "" : "(empty)", value: "",
  });
  const send = h("button.small", { type: "button" }, "Save");
  const form = h("form.adv-prefs-form", input, send,
    h("p.adv-prefs-form-hint", "Save calls bernie config set ", it.key, " <value>."));
  form.addEventListener("submit", (e) => { e.preventDefault(); send.click(); });
  send.addEventListener("click", () => { savePref(it.key, input.value, send, value, source, resolved); });
  return h("div.adv-prefs-row", { "data-key": it.key },
    h("div.adv-prefs-head",
      h("span.adv-prefs-key", it.key), source, h("span.adv-prefs-value-wrap", value)),
    resolved, choices, hint, form);
}
function setResolved(el, it) {
  el.textContent = "";
  el.append(h("span", "Resolved: "));
  if (it.resolved && it.resolved !== it.value) el.append(h("code", it.resolved));
  else el.append(h("span.muted", "(the same as the value)"));
}
async function savePref(key, value, btn, valueEl, srcEl, resolvedEl) {
  btn.disabled = true; btn.textContent = "Saving…";
  try {
    const doc = await api("/api/prefs", { method: "PUT", body: { key: key, value: value } });
    const it = (doc && doc.items || []).find((x) => x.key === key);
    if (it) {
      valueEl.textContent = it.value || "–";
      srcEl.textContent = it.source || "default";
      setResolved(resolvedEl, it);
    }
    btn.textContent = "Saved"; btn.disabled = false;
  } catch (e) {
    btn.textContent = "Save"; btn.disabled = false;
    toast("Save failed: " + ((e && e.message) || String(e)), false);
  }
}
