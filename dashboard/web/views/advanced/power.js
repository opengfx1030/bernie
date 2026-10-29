"use strict";
// ---------- The power caps of the host ----------
// The read of the cap of every card and the one write of the page: the
// desired cap of a card. The service clamps the value to the range the card
// allows; the row shows that range before the write.

/** advPowerCard is the card with the power caps of the host. It reads the
 *  document itself and reads it again after every write. */
function advPowerCard() {
  const card = h("div.card");
  const view = {
    el: card,
    power: null,
    update: (power) => {
      view.power = power;
      advPaintPower(card, view);
    },
    reload: async () => {
      try {
        view.power = await api("/api/power");
      } catch (e) {
        view.power = null;
      }
      advPaintPower(card, view);
    },
  };
  return view;
}

/** advPaintPower paints the power card: the state of the service, the cap of
 *  every card and the control that sets the desired cap. */
function advPaintPower(card, view) {
  const p = view.power;
  if (!p) {
    put(card, cardHead("The power caps", "/api/power"),
      emptyState("The power caps are not available.",
        "The caps of the cards come from the power service of the host.",
        "bernie tune power status"));
    return;
  }
  const service = p.installed
    ? "installed, " + (p.enabled ? "enabled" : "not enabled") + ", "
      + (p.active ? "watching changes" : "not watching changes")
    : "not installed";
  const children = [
    cardHead("The power caps", "/api/power", badge(p.state || "skip")),
    h("div.muted", "Service: " + service),
  ];
  if (!p.installed) {
    children.push(h("div.err",
      "The power service is not installed. Install it before changing a cap; the controls stay disabled."));
    for (const command of p.install_commands || []) {
      children.push(cmdBlock(command, "Copy the install command"));
    }
  } else if (!p.enabled || !p.active) {
    children.push(h("div.err",
      "The power service is not enabled or is not watching the desired-state file."));
  }
  const rows = (p.gpus || []).map((g) => advPowerRow(g, p.installed, view));
  if (rows.length) {
    children.push(h("div.scroll", h("table",
      tableHead(["GPU", "Current", "Allowed", "Desired", "Set", "State"]),
      h("tbody", rows))));
  } else {
    children.push(h("div.empty", "No GPU publishes a hwmon power cap."));
  }
  put(card, ...children);
}

/** advPowerRow is one row of the power table: the cap of the card, the
 *  range the service allows and the control that sets the desired cap. The
 *  value is clamped to the range of the card before it is written. */
function advPowerRow(g, installed, view) {
  const wanted = g.effective_w != null ? g.effective_w : (g.desired_w != null ? g.desired_w : g.cap_w);
  const min = g.min_w || 1;
  const max = g.max_w || Math.max(min, g.cap_w || min);
  const value = Math.max(min, Math.min(max, wanted || min));
  const range = h("input", {
    type: "range", min: String(min), max: String(max), step: "1", value: String(value),
    disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts",
  });
  const number = h("input", {
    type: "number", min: String(min), max: String(max), step: "1", value: String(value),
    disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts",
  });
  range.addEventListener("input", () => { number.value = range.value; });
  const apply = async () => {
    let watts = Number(number.value);
    if (!Number.isFinite(watts)) {
      toast("Enter a power cap in watts.");
      return;
    }
    watts = Math.max(min, Math.min(max, watts));
    number.value = String(watts);
    range.value = String(watts);
    await advSavePower(g.hip_index, watts, view);
  };
  range.addEventListener("change", apply);
  number.addEventListener("change", apply);
  const drift = g.drift ? h("span.badge.b-warn", "drift") : h("span.badge.b-ok", "in sync");
  const desired = g.desired_w == null ? "–" : Math.round(g.desired_w) + " W";
  return h("tr", { "data-power-gpu": String(g.hip_index) },
    h("td", "GPU " + g.hip_index),
    h("td", Math.round(g.cap_w) + " W"),
    h("td", Math.round(min) + "–" + Math.round(max) + " W"),
    h("td", desired),
    h("td", h("div.row", range, number)),
    h("td", drift));
}

/** advSavePower writes the desired cap of one card. The service clamps the
 *  value to the range of the card; the answer says what it did. */
async function advSavePower(gpu, watts, view) {
  try {
    const answer = await api("/api/power", { method: "PUT", body: { gpu, watts } });
    const clamped = answer && answer.clamped
      ? " The service clamped it to " + answer.watts + " W."
      : "";
    toast("Desired power cap saved. The service will apply it." + clamped, true);
    await view.reload();
  } catch (e) {
    toast("Power cap not saved: " + (e.message || e));
  }
}
