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

/** advToolsCard is the card with the two operator tools of the reference:
 *  the ECC counters and the power cap. It carries the exact commands in
 *  the words of bernie: what bernie does itself (the cap it sets without
 *  root) and what the operator runs in a terminal (what needs root).
 *  bernie never runs a root command. See ia.md §5 R36. */
function advToolsCard() {
  const card = h("div.card");
  const view = {
    el: card,
    update: (report) => {
      const checks = (report && report.checks) || [];
      const ecc = checks.filter((c) => (c.id || "").indexOf("ecc.") === 0);
      const power = checks.find((c) => c.id === "tuning.power");
      put(card,
        cardHead("The ECC and the power tools", "/api/doctor",
          h("span.small.muted", "the exact commands; bernie never runs them as root")),
        h("div.adv-doctor-group",
          h("h3.adv-doctor-cat", "The ECC counters"),
          ecc.length ? ecc.map(advToolsEcc) : h("p.small.muted", "This host has no card that counts memory errors."),
          cmdBlock("cat /sys/class/drm/card*/device/ras/umc_err_count", "Copy the read command"),
          h("p.small.muted", "Turn ECC off with amdgpu.ras_enable=0 in the kernel command line, then reboot twice; turn it back on with amdgpu.ras_enable=1. The operator edits the kernel command line and reboots: bernie never changes the ECC state."),
          h("a.small", { href: "https://blivioniag.github.io/gfx1030-wiki/tuning/ecc.html", target: "_blank", rel: "noopener noreferrer" }, "the method")),
        h("div.adv-doctor-group",
          h("h3.adv-doctor-cat", "The power cap"),
          power ? h("div",
            power.detail ? h("p.small.muted", power.detail) : null,
            power.fix ? cmdBlock(power.fix, "Copy the command") : null) : h("p.small.muted", "The cap of every card is in the table above."),
          cmdBlock("cat /sys/class/drm/card*/device/hwmon/hwmon*/power1_cap", "Copy the read command"),
          cmdBlock("sudo bernie tune power apply", "Copy the command"),
          h("p.small.muted", "bernie tune power set writes the cap it wants without root; the service applies it, and that needs root. A kernel with the 250 W VBIOS floor refuses a lower cap until the powerfix is applied."),
          h("a.small", { href: "https://github.com/blivioniag/v620_toolbox", target: "_blank", rel: "noopener noreferrer" }, "the powerfix")));
    },
  };
  return view;
}

/** advToolsEcc is one card's ECC line: its state, the words of the doctor
 *  and the fix command. */
function advToolsEcc(check) {
  return h("div.finding.doctor-finding",
    h("div.doctor-finding-head",
      h("span.badge", { class: "b-" + (check.status || "warn") }, check.status || "warn"),
      h("span.doctor-finding-id", check.id || "")),
    h("p.doctor-finding-cause", check.title || ""),
    check.detail ? h("p.small.muted", check.detail) : null,
    check.fix ? cmdBlock(check.fix, "Copy the fix command") : null);
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
