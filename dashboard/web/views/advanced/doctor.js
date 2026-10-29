"use strict";
// ---------- The doctor report in full ----------
// Every check of every category of the host. The Setup tab shows the summary
// and the checks that fail; this is the whole report, with the cause and
// the fix of every check.

/** advDoctorReport is the card with the whole doctor report. */
function advDoctorReport() {
  const card = h("div.card");
  const view = {
    el: card,
    update: (report) => {
      if (!report) {
        put(card, cardHead("The doctor report", "/api/doctor"),
          emptyState("The doctor report could not be read.",
            "The doctor checks every part of the host: the cards, the groups, the tools, the environment and the tuning.",
            "bernie doctor"));
        return;
      }
      const checks = report.checks || [];
      const summary = report.summary || {};
      const groups = new Map();
      for (const check of checks) {
        const cat = check.category || "other";
        if (!groups.has(cat)) groups.set(cat, []);
        groups.get(cat).push(check);
      }
      put(card,
        cardHead("The doctor report", "/api/doctor",
          h("span.small.muted", checks.length + " checks in " + groups.size + " categories")),
        h("div.row", ["fail", "warn", "ok", "skip"].map((s) =>
          h("span.badge", { class: "b-" + s }, (summary[s] || 0) + " " + s))),
        [...groups.entries()].map(([cat, list]) => h("div.adv-doctor-group",
          h("h3.adv-doctor-cat", cat),
          h("div.adv-doctor-list", list.map(advDoctorCheck)))));
    },
  };
  return view;
}

/** advDoctorCheck is one check of the report: its state, its id, its title,
 *  the cause and the fix. The fix is a command with a copy button. */
function advDoctorCheck(check) {
  return h("div.finding.doctor-finding",
    h("div.doctor-finding-head",
      h("span.badge", { class: "b-" + (check.status || "warn") }, check.status || "warn"),
      h("span.doctor-finding-id", check.id || "")),
    h("p.doctor-finding-cause", check.title || ""),
    check.detail ? h("p.small.muted", check.detail) : null,
    check.fix ? cmdBlock(check.fix, "Copy the fix command") : null,
    check.doc ? h("a.small", { href: check.doc, target: "_blank", rel: "noopener noreferrer" }, "the rule") : null);
}
