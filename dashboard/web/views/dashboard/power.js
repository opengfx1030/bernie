"use strict";
// ---------- GPU power card ----------

async function loadPower() {
  state.power = await api("/api/power");
  return state.power;
}

function renderPower(root) {
  const p = state.power;
  if (!p) return;
  let card = root.querySelector(".card.power-card");
  if (!card) root.append(card = h("div.card.power-card"));
  const service = p.installed
    ? `installed, ${p.enabled ? "enabled" : "not enabled"}, ${p.active ? "watching changes" : "not watching changes"}`
    : "not installed";
  const children = [
    cardHead("Power caps", "/api/power", badge(p.state || "skip")),
    h("div.muted", "Service: " + service),
  ];
  if (!p.installed) {
    children.push(h("div.err", "The power service is not installed. Install it before changing a cap; the controls stay disabled."));
    for (const command of p.install_commands || []) {
      children.push(cmdBlock(command, "Copy the install command"));
    }
  } else if (!p.enabled || !p.active) {
    children.push(h("div.err", "The power service is not enabled or is not watching the desired-state file."));
  }
  const rows = (p.gpus || []).map((g) => powerRow(g, p.installed));
  if (rows.length) {
    children.push(h("div.scroll", h("table", h("thead", h("tr",
      ["GPU", "Current", "Allowed", "Desired", "Set", "State"].map((c) => h("th", c)))), h("tbody", rows))));
  } else {
    children.push(h("div.empty", "No GPU publishes a hwmon power cap."));
  }
  put(card, ...children);
}

function powerRow(g, installed) {
  const wanted = g.effective_w != null ? g.effective_w : (g.desired_w != null ? g.desired_w : g.cap_w);
  const min = g.min_w || 1;
  const max = g.max_w || Math.max(min, g.cap_w || min);
  const value = Math.max(min, Math.min(max, wanted || min));
  const range = h("input", { type: "range", min: String(min), max: String(max), step: "1", value: String(value), disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts" });
  const number = h("input", { type: "number", min: String(min), max: String(max), step: "1", value: String(value), disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts" });
  range.addEventListener("input", () => { number.value = range.value; });
  const apply = async () => {
    let watts = Number(number.value);
    if (!Number.isFinite(watts)) return toast("Enter a power cap in watts.");
    watts = Math.max(min, Math.min(max, watts));
    number.value = String(watts);
    range.value = String(watts);
    await savePowerDesired(g.hip_index, watts);
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

async function savePowerDesired(gpu, watts) {
  try {
    await api("/api/power", { method: "PUT", body: { gpu, watts } });
    toast("Desired power cap saved. The service will apply it.", true);
    await loadPower();
    renderPower(gpusRoot);
  } catch (e) {
    toast("Power cap not saved: " + e.message);
  }
}
