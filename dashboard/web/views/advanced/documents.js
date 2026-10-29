"use strict";
// ---------- The raw documents of the host ----------
// The JSON of every endpoint the Advanced tab reads. The disclosure reads
// them when the reader opens it: the tab does not pay for them on load.

/** advDocuments is the disclosure with the raw documents of the host. */
function advDocuments() {
  const body = h("div.adv-documents-body", loading());
  const det = h("details.advanced.adv-documents", h("summary", "Show details"), body);
  let loaded = false;
  det.addEventListener("toggle", async () => {
    if (!det.open || loaded) return;
    loaded = true;
    await advLoadDocuments(body);
  });
  return det;
}

/** advLoadDocuments reads every document of the tab and paints it with the
 *  command that answers the same. */
async function advLoadDocuments(body) {
  const entries = [];
  const read = async (path, doc) => {
    if (doc !== undefined) {
      entries.push({ path: path, doc: doc });
      return;
    }
    try {
      entries.push({ path: path, doc: await api(path) });
    } catch (e) {
      entries.push({ path: path, error: String(e.message || e) });
    }
  };
  await read("/api/hw");
  await read("/api/doctor");
  await read("/api/power");
  await read("/api/runs");
  await read("/api/jobs");
  put(body,
    h("p.small.muted", "The documents behind this tab. An agent reads the same ones from the API."),
    entries.map(advDocument),
    h("h3", "The same thing in a terminal"),
    cmdBlock("bernie hw", "Copy the command"),
    cmdBlock("bernie doctor", "Copy the command"),
    cmdBlock("bernie tune power status", "Copy the command"),
    cmdBlock("bernie ps", "Copy the command"));
}

/** advDocument is one raw document with its endpoint and a copy button. */
function advDocument(entry) {
  const text = entry.error
    ? JSON.stringify({ error: entry.error }, null, 2)
    : JSON.stringify(entry.doc, null, 2);
  return h("div.adv-document",
    h("div.adv-document-head",
      h("span.adv-document-path", entry.path),
      h("span.grow"),
      jsonLink(entry.path),
      copyBtn(text, "Copy the JSON of " + entry.path)),
    h("pre.adv-document-json", text));
}
