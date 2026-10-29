"use strict";
// ---------- The power caps ----------
// The caps of the cards: the state of the service, the wanted value of every
// card and the control that writes it.
//
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.

// dashPower is the section with the caps of the cards.
function dashPower(section) {
  const el = h("div.card");
  section.append(el);
  const api = {
    update: () => {
      const p = dash.power;
      const kids = [];
      kids.push(cardHead("Power caps", "/api/power"));
      if (!p) {
        const err = (dash.errors || {})["/api/power"];
        kids.push(h("div.err", "The power caps could not be read."));
        kids.push(h("div", { class: "dash-note-13-gap" }, err || "The endpoint did not answer."));
        kids.push(h("button", { type: "button", onclick: () => dashLoad() }, "Try again"));
        put(el, ...kids);
        return;
      }
      const service = p.installed
        ? (p.enabled && p.active ? "installed, enabled and watching the changes" : (p.enabled ? "installed and enabled, but not watching the changes" : "installed, but not enabled"))
        : "not installed";
      kids.push(h("div", { class: "dash-power-service" }, "The power service is " + service + "."));
      if (!p.installed) {
        kids.push(h("div", { class: "dash-note-13" }, "The power service is not installed. Install it before changing a cap; the controls stay disabled."));
        for (const command of p.install_commands || []) kids.push(cmdBlock(command, "Copy the install command"));
      } else if (!p.enabled || !p.active) {
        kids.push(h("div", { class: "dash-warn-13" }, "The service does not watch the wanted values yet, so a cap may not reach the card."));
      }
      const rows = (p.gpus || []).map((g) => dashPowerRow(g, p.installed));
      if (rows.length) {
        kids.push(h("div.scroll", h("table",
          h("thead", h("tr", ["Card", "Cap", "Allowed", "Wanted", "In effect", "Set", "State"].map((c) => h("th", { class: c === "Card" || c === "Set" || c === "State" ? "" : "num" }, c)))),
          h("tbody", rows))));
        kids.push(dashPowerAll(p));
      } else {
        kids.push(h("div", { class: "dash-note-13" }, "No GPU publishes a hwmon power cap."));
      }
      put(el, ...kids);
    },
  };
  api.update();
  return api;
}

// dashPowerRow is one card of the table: the cap, the wanted value, the value
// in effect and the control that writes the new one.
function dashPowerRow(g, installed) {
  const wanted = g.effective_w != null ? g.effective_w : (g.desired_w != null ? g.desired_w : g.cap_w);
  const min = g.min_w || 1;
  const max = g.max_w || Math.max(min, g.cap_w || min);
  const value = Math.max(min, Math.min(max, wanted || min));
  const input = h("input", { type: "number", min: String(min), max: String(max), step: "1", value: String(Math.round(value)),
    disabled: !installed, "aria-label": "the wanted watts of GPU " + g.hip_index,
    class: "dash-cap-input" });
  const set = h("button", { type: "button", disabled: !installed }, "Set");
  set.addEventListener("click", () => dashSavePower(g.hip_index, Number(input.value), input));
  const drift = g.drift ? dashPill("warn", "drift") : dashPill("ok", "in sync");
  const cell = (text) => h("td.num", text);
  return h("tr", null,
    h("td", { class: "dash-strong" }, "GPU " + g.hip_index),
    cell(g.cap_w != null ? Math.round(g.cap_w) + " W" : "–"),
    cell(Math.round(min) + "–" + Math.round(max) + " W"),
    cell(g.desired_w != null ? Math.round(g.desired_w) + " W" : "–"),
    cell(g.effective_w != null ? Math.round(g.effective_w) + " W" : "–"),
    h("td", h("div.row", input, set)),
    h("td", drift));
}

// dashPowerAll writes one cap for every card of the host. One value has to
// be one every card can take: the range is the highest minimum and the
// lowest maximum of the cards. A value outside it is clamped, and the cards
// that could not take it are named.
function dashPowerAll(p) {
  const gpus = (p.gpus || []).filter((g) => (g.min_w || 0) > 0 || (g.max_w || 0) > 0);
  const mins = gpus.map((g) => g.min_w || 0).filter((v) => v > 0);
  const maxs = gpus.map((g) => g.max_w || 0).filter((v) => v > 0);
  const min = mins.length ? Math.max(...mins) : 1;
  const max = maxs.length ? Math.min(...maxs) : Math.max(min, 1);
  const input = h("input", { type: "number", min: String(min), max: String(max), step: "1", value: String(Math.round((min + max) / 2)),
    disabled: !p.installed, "aria-label": "the wanted watts of every card",
    class: "dash-cap-input" });
  const set = h("button", { type: "button", disabled: !p.installed }, "Set every card");
  set.addEventListener("click", () => dashSavePower("all", Number(input.value), input));
  return h("div", { class: "dash-power-all" },
    h("span", "One cap for every card:"), input, set,
    h("span", { class: "small muted" },
      "The range every card of this host takes: " + Math.round(min) + "–" + Math.round(max) +
      " W. A value outside it is clamped, and the cards that could not take it are named."));
}

// dashSavePower writes a wanted cap and says what happened. The server
// clamps the value to the range the cards share and names the cards that
// could not take it; the page says what was written.
async function dashSavePower(gpu, watts, input) {
  if (!isFinite(watts)) { toast("The cap was not saved: enter a number of watts."); return; }
  try {
    const res = await api("/api/power", { method: "PUT", body: { gpu: gpu, watts: Math.round(watts) } });
    const held = (res && res.clamped) || [];
    const written = res && res.watts != null ? Math.round(res.watts) : Math.round(watts);
    if (input) input.value = String(written);
    if (held.length) {
      toast("The cap was saved at " + written + " W. GPU " + held.join(", GPU ") +
        " could not take " + Math.round(watts) + " W.", true);
    } else {
      toast("The cap was saved at " + written + " W. The service applies it.", true);
    }
    dash.power = await api("/api/power");
    if (dash.live.power) dash.live.power.update();
  } catch (e) {
    toast("The cap was not saved: " + (e.message || e));
  }
}
