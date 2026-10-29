"use strict";
// ---------- Fleet ----------
// The Fleet tab answers one question: what do the other bernie hosts do?
// It lists this box and every configured peer, with the state word, the
// serving model, the output speed, the cards and their VRAM. A peer that
// does not answer keeps its row and says why: the gap stays a gap.
//
// The rows come from GET /api/fleet, which reads the peers server-side.
// The token of a peer stays in the config file: it never reaches this page
// and it is never in the "Open" link — the reader logs in on that host.

/* ---------- the peers ---------- */

/** FLEET_STATE_WORD maps the state of a host to the word the page shows.
 *  The words are the run states of spec §8.3. */
const FLEET_STATE_WORD = {
  serving: "Serving",
  running: "Running",
  starting: "Starting",
  stopped: "Stopped",
  exited: "Exited",
};

/** fleetStatePill is the state of one host: a mark and a word. */
function fleetStatePill(row) {
  if (!row.reachable) return statePill("fail", "Unreachable");
  const word = FLEET_STATE_WORD[row.state] || row.state || "Unknown";
  const kind = word === "Serving" ? "ok" : word === "Stopped" ? "idle" : "running";
  return statePill(kind, word);
}

/** fleetGPUs is the line of the cards of one host: the names and the VRAM
 *  of each. A host without cards says so in words. */
function fleetGPUs(gpus) {
  if (!gpus || !gpus.length) return h("span.muted", "no cards");
  return h("span", gpus.map((g) => {
    const name = g.name || "card";
    const vram = g.vram_total_gb == null ? null : num(g.vram_total_gb) + " GiB";
    return h("span.fleet-gpu", name, vram ? h("span.muted", " · " + vram) : null);
  }));
}

/** fleetRow is one host of the list: the name, the state, the serving
 *  model, the output speed, the cards, and the link to that host's own
 *  dashboard. */
function fleetRow(row, onRemove) {
  const el = h("div.fleet-row");
  const name = h("div.fleet-name");
  name.append(h("b", row.name || "unnamed"));
  if (row.me) {
    name.append(h("span.fleet-me", "this box"));
  } else {
    name.append(h("span.muted", row.url || ""));
  }
  const state = h("div.fleet-state");
  state.append(fleetStatePill(row));
  if (!row.reachable && row.error) {
    state.append(h("p.small.fleet-why", row.error));
  }
  const serving = h("div.fleet-serving");
  serving.append(h("span.muted", "model: "), h("span", row.recipe || "–"));
  const speed = h("div.fleet-speed");
  speed.append(h("span.fleet-num", row.gen_tok_s == null ? "–" : num(row.gen_tok_s)));
  speed.append(h("span.muted", " tok/s"));
  const cards = h("div.fleet-cards");
  cards.append(fleetGPUs(row.gpus));
  const open = h("div.fleet-open");
  if (!row.me) {
    // The link carries no token: the reader logs in on that host.
    open.append(h("a", { href: row.url || "#", target: "_blank", rel: "noopener" }, "Open"));
    const rm = h("button.small", { type: "button" }, "Forget");
    rm.addEventListener("click", () => onRemove(row));
    open.append(rm);
  }
  el.append(name, state, serving, speed, cards, open);
  return el;
}

/* ---------- the share report ---------- */

/** fleetShare copies the redacted report of this box: the facts of the
 *  host and its checks, with no hostname, no username and no token. The
 *  server writes it that way (GET /api/report). */
async function fleetShare() {
  try {
    const doc = await api("/api/report");
    const text = (doc && doc.report) || "";
    if (!text) {
      toast("The report could not be read.");
      return;
    }
    const ok = await copyText(text);
    toast(ok ? "The report is on the clipboard." : "Copy failed: select the text and copy it.", ok);
  } catch (e) {
    toast("The report could not be read: " + e.message, false);
  }
}

/* ---------- the tab ---------- */

/** viewFleet opens the Fleet tab: the list of the hosts, the share button
 *  and the form that adds a host. */
async function viewFleet(root) {
  const head = stateHead("Fleet");
  const list = h("div.fleet-list");
  const share = h("button", { type: "button" }, "Share this host");
  share.addEventListener("click", () => fleetShare());
  const add = h("form.fleet-add");
  const nameIn = h("input", { name: "name", placeholder: "gpu-box-2", autocomplete: "off" });
  const urlIn = h("input", { name: "url", placeholder: "http://gpu-box-2:7860", autocomplete: "off" });
  const tokenIn = h("input", { name: "token", type: "password", placeholder: "the token that box printed", autocomplete: "off" });
  const addBtn = h("button.primary", { type: "submit" }, "Add a host");
  add.append(
    h("div.fleet-add-row", h("label", "Name"), nameIn),
    h("div.fleet-add-row", h("label", "Dashboard URL"), urlIn),
    h("div.fleet-add-row", h("label", "Token"), tokenIn),
    h("div.fleet-add-row", h("label", ""), addBtn));
  const note = h("p.small.muted", "The token stays in the config file of this box. It is never sent to this page and never shown again.");
  const box = h("section.card", sectionLabel("The other hosts"), list, add, note);
  put(root, head, box);

  setHead(head, "READING THE OTHER HOSTS.", "Each host is asked over its own dashboard API. A host that does not answer keeps its row and says why.");

  /** paint draws the rows of the list. */
  function paint(rows) {
    const peers = (rows || []).filter((r) => !r.me);
    if (!peers.length) {
      put(list, emptyState("No other host is configured yet.",
        "Add one with its dashboard URL; bernie reads its state from there.",
        "bernie fleet add gpu-box-2 http://gpu-box-2:7860"));
      return;
    }
    put(list, rows.map((r) => fleetRow(r, forget)));
  }

  /** load reads the list again. */
  async function load() {
    try {
      const doc = await api("/api/fleet");
      paint(doc && Array.isArray(doc.hosts) ? doc.hosts : (doc || []));
    } catch (e) {
      put(list, failed(e));
    }
  }

  /** forget removes one host from the fleet, after the reader confirms it. */
  async function forget(row) {
    if (!window.confirm("Forget " + row.name + "? Its URL and its token are dropped.")) return;
    try {
      await api("/api/fleet/" + encodeURIComponent(row.name), { method: "DELETE" });
      await load();
      toast("Forgot " + row.name + ".");
    } catch (e) {
      toast(e.message, false);
    }
  }

  add.addEventListener("submit", async (e) => {
    e.preventDefault();
    const name = nameIn.value.trim();
    const url = urlIn.value.trim();
    if (!name || !url) {
      toast("Give the host a name and its dashboard URL.");
      return;
    }
    try {
      await api("/api/fleet", { method: "POST", body: { name, url, token: tokenIn.value } });
      nameIn.value = "";
      urlIn.value = "";
      tokenIn.value = "";
      await load();
      toast("Added " + name + ".");
    } catch (err) {
      toast(err.message, false);
    }
  });

  await load();
}
