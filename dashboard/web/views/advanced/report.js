"use strict";
// ---------- Advanced: the redacted share report ----------
// The report a person can paste into an issue, a bug or a chat: the facts
// that make a bug reproducible and nothing that identifies the host (ia.md
// R24). The document comes from GET /api/report, which is the same one
// `bernie doctor --report` prints and the MCP tool `report` returns.
//
// Two actions: Copy report puts it on the clipboard, Download report saves
// it as a file. The Setup tab's details carry the same two actions when a
// check failed, because that is the moment a person wants to share.

/** advReportCard is the card that holds the share report of this host. */
function advReportCard() {
  const card = h("div.card");
  const body = h("div.adv-report-body");
  const view = {
    el: card,
    /** reload reads /api/report and paints it. */
    reload: async () => {
      try {
        const doc = await api("/api/report");
        view.update(doc);
        return doc;
      } catch (e) {
        put(body, failed(e));
        return null;
      }
    },
    /** update paints the document the caller hands in. */
    update: (doc) => {
      const text = (doc && doc.report) || "";
      if (!text) {
        put(body, cardHead("The share report", "/api/report"),
          emptyState("No report was read.",
            "The report is built from the host and its checks; the call failed or the host answered nothing.",
            "bernie doctor --report"));
        return;
      }
      put(body,
        h("p.adv-report-note", "The report carries no hostname, no username, no token and no path with the name of the user."),
        h("pre.adv-report-text", text));
    },
  };
  put(card, cardHead("The share report", "/api/report",
    h("button.small", { type: "button", onclick: () => advCopyReport() }, "Copy report"),
    h("button.small", { type: "button", onclick: () => advDownloadReport() }, "Download report")), body);
  return view;
}

/** advReportText reads the report of the host for the copy and the
 *  download. It returns "" when the report cannot be read. */
async function advReportText() {
  try {
    const doc = await api("/api/report");
    return (doc && doc.report) || "";
  } catch (e) {
    return "";
  }
}

/** advCopyReport puts the report on the clipboard. */
async function advCopyReport() {
  const text = await advReportText();
  if (!text) {
    toast("The report could not be read.");
    return;
  }
  const ok = await copyText(text);
  toast(ok ? "The report is on the clipboard." : "Copy failed: select the text and copy it.", ok);
}

/** advDownloadReport saves the report as a file. The name carries the date
 *  so two reports of one day do not overwrite each other. */
async function advDownloadReport() {
  const text = await advReportText();
  if (!text) {
    toast("The report could not be read.");
    return;
  }
  const name = "bernie-report-" + new Date().toISOString().slice(0, 10) + ".md";
  const url = URL.createObjectURL(new Blob([text], { type: "text/markdown" }));
  const a = h("a", { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
  toast("The report was downloaded as " + name + ".");
}
