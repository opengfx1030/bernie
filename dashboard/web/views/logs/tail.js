// The tail of the log of one run: the stream, the levels, the filter, the
// search, the follow button and the download (design/logs.md).
"use strict";

// The lines kept in memory. The tail of a long run is trimmed from the top;
// the download writes what is in memory.
const LOG_MAX_LINES = 5000;
// The levels of the filter, in the order the filter shows them.
const LOG_FILTER_LEVELS = ["all", "info", "warn", "error"];

/** logCount writes a number with a space between the thousands: 1 240. */
function logCount(n) {
  return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

/** logTail is the widget with the tail of the log of the picked run.
 *  It owns the stream, the lines in memory, the filter and the search. */
function logTail(options) {
  const opts = options || {};
  const lines = [];      // {no, text, level, el}
  let follow = true;     // the tail follows the end of the log
  let level = "all";     // the level filter
  let query = "";        // the search, lower case
  let shown = 0;         // the lines the filter and the search let through
  let stream = null;
  let runName = "";
  let findings = [];     // the findings of the log doctor

  const count = h("span.lg-count", "0 lines");
  const paused = h("span.lg-paused", { hidden: true }, "Paused");
  const status = h("div.lg-status", { hidden: true });
  const empty = h("div.lg-empty", emptyState("No log yet.", "Start the server first, then follow its log here.", "bernie serve <recipe>"));
  const linesBox = h("div.log-lines");
  const box = h("div.log", { role: "log", "aria-live": "off", tabindex: "0" }, linesBox, empty);
  const followBtn = h("button.lg-follow", { type: "button", hidden: true, onclick: () => resume() }, "Follow");
  const firstErr = h("button.lg-first-error", { type: "button", hidden: true, onclick: () => jumpToError() }, "First error");
  const search = h("input.lg-search", {
    type: "search", placeholder: "Search the log", "aria-label": "Search the log", disabled: true,
    oninput: (ev) => { query = ev.target.value.trim().toLowerCase(); apply(); },
  });

  const filter = h("div.segmented-control.lg-filter", { role: "group", "aria-label": "Filter the log by level" },
    LOG_FILTER_LEVELS.map((lvl) => {
      const input = h("input", {
        type: "radio", name: "lg-level", value: lvl, checked: lvl === "all",
        onchange: (ev) => { if (ev.target.checked) { level = lvl; apply(); } },
      });
      return h("label.lg-seg", { class: lvl === "all" ? "on" : "" }, input, h("span", lvl));
    }));

  const el = h("div.lg-tailcard",
    h("div.lg-toolbar", count, paused, h("span.grow"), firstErr, filter, search),
    h("div.lg-tailwrap", box, followBtn),
    status);

  // ---- the lines ---------------------------------------------------------

  /** matches tells whether a line passes the filter and the search. */
  function matches(line) {
    if (level !== "all" && line.level !== level) return false;
    if (query && !line.text.toLowerCase().includes(query)) return false;
    return true;
  }

  /** apply re-reads the filter and the search and updates the counter. */
  function apply() {
    shown = 0;
    for (const line of lines) {
      const ok = matches(line);
      line.el.hidden = !ok;
      if (ok) shown++;
    }
    count.textContent = logCount(lines.length) + " lines" + (shown === lines.length ? "" : " · " + logCount(shown) + " shown");
    search.disabled = lines.length === 0;
    firstErr.hidden = !lines.some((l) => l.level === "error");
  }

  /** add puts one line of the log into the tail. */
  function add(text, no) {
    const t = text == null ? "" : String(text);
    const lvl = findingHit(t, no) ? "error" : logLevel(t);
    const row = h("div.log-line", { class: "lg-" + lvl },
      h("span.lg-no", no == null ? "" : String(no)),
      h("span.lg-lvl", logLevelWord(lvl)),
      h("span.lg-text", t));
    lines.push({ no: no == null ? null : no, text: t, level: lvl, el: row });
    linesBox.append(row);
    if (lines.length > LOG_MAX_LINES) lines.shift().el.remove();
    empty.hidden = true;
    const ok = matches(lines[lines.length - 1]);
    lines[lines.length - 1].el.hidden = !ok;
    if (ok) shown++;
    count.textContent = logCount(lines.length) + " lines" + (shown === lines.length ? "" : " · " + logCount(shown) + " shown");
    search.disabled = false;
    firstErr.hidden = !lines.some((l) => l.level === "error");
    if (follow) scrollEnd();
    if (opts.onCount) opts.onCount(lines.length);
  }

  /** findingHit tells whether a line is the one a finding points at. */
  function findingHit(text, no) {
    for (const f of findings) {
      if (no != null && f.line_no != null && f.line_no === no) return true;
      if (f.line && text.includes(f.line)) return true;
    }
    return false;
  }

  function scrollEnd() {
    box.scrollTop = box.scrollHeight;
  }

  function syncFollow() {
    followBtn.hidden = follow;
    paused.hidden = follow;
  }

  function resume() {
    follow = true;
    syncFollow();
    scrollEnd();
  }

  // The reader scrolls up: the tail stops following the end of the log.
  box.addEventListener("scroll", () => {
    const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 4;
    if (atEnd === follow) return;
    follow = atEnd;
    syncFollow();
  });

  // ---- the jump ----------------------------------------------------------

  /** findLine returns the line of the tail a finding points at. */
  function findLine(f) {
    if (f.line_no != null) {
      for (const l of lines) if (l.no === f.line_no) return l;
    }
    if (f.line) {
      for (const l of lines) if (l.text.includes(f.line)) return l;
    }
    return null;
  }

  /** jumpTo scrolls the tail to a line and marks it for a second. */
  function jumpTo(line) {
    if (!line) return false;
    line.el.scrollIntoView({ block: "center", behavior: "auto" });
    line.el.classList.add("lg-hit");
    setTimeout(() => line.el.classList.remove("lg-hit"), 1600);
    return true;
  }

  /** jumpToFinding scrolls to the line of a finding of the log doctor. It
   *  says no when the line is not in the tail. */
  function jumpToFinding(f) {
    return jumpTo(findLine(f));
  }

  /** jumpToError scrolls to the first error line of the tail. */
  function jumpToError() {
    for (const l of lines) {
      if (l.level === "error") return jumpTo(l);
    }
    return false;
  }

  // ---- the stream --------------------------------------------------------

  /** setStatus writes the state of the stream under the tail. */
  function setStatus(text, action) {
    put(status, text ? h("span", text) : null, action || null);
    status.hidden = !text && !action;
  }

  /** open follows the log of a run. */
  function open(name, known) {
    close();
    runName = name;
    findings = known || [];
    lines.length = 0;
    shown = 0;
    put(linesBox);
    empty.hidden = false;
    follow = true;
    syncFollow();
    apply();
    setStatus("");
    stream = openRunLogs(name, {
      open: () => setStatus("Following the log of " + name + "."),
      disconnect: () => setStatus("The stream stopped.", reconnect()),
      end: () => setStatus("The log of " + name + " ended."),
      error: (ev) => setStatus("The stream stopped: " + (ev && ev.data ? ev.data : "no reason"), reconnect()),
      message: (ev) => add(ev && ev.data ? ev.data : ""),
    });
  }

  /** reconnect is the button that starts the stream again. */
  function reconnect() {
    return h("button", { type: "button", onclick: () => open(runName, findings) }, "Reconnect");
  }

  /** close stops the stream. */
  function close() {
    if (stream) { stream.close(); stream = null; }
  }

  // ---- the download ------------------------------------------------------

  /** fileName is the name of the file the download writes. */
  function fileName() {
    const d = new Date();
    const p = (n) => String(n).padStart(2, "0");
    return runName + "-" + d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate()) +
      "-" + p(d.getHours()) + p(d.getMinutes()) + p(d.getSeconds()) + ".log";
  }

  /** save writes the lines in memory to a file. It asks nobody. */
  function save() {
    if (!lines.length) return false;
    const name = fileName();
    const text = lines.map((l) => l.text).join("\n") + "\n";
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain;charset=utf-8" }));
    const a = h("a", { href: url, download: name });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
    toast("Wrote " + name + ".", true);
    return true;
  }

  apply();
  syncFollow();

  return {
    el: el,
    open: open,
    close: close,
    save: save,
    apply: apply,
    setFindings: (f) => { findings = f || []; },
    jumpToFinding: jumpToFinding,
    jumpToError: jumpToError,
    count: () => lines.length,
    hasLines: () => lines.length > 0,
  };
}
