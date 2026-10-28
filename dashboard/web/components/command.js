// Shared command block and copy button.
"use strict";

// copyBtn is a copy button for a command shown inline (a fix, a flag).
function copyBtn(text) {
  return h("button.small.copy", { type: "button", "data-copy": text, "aria-label": "Copy the command" }, "Copy");
}

/** cmdBlock shows a command with its copy button. The text is never markup. */
function cmdBlock(text, label) {
  if (!text) return null;
  return h("div.cmd", h("code", text),
    h("button.copy", { type: "button", "data-copy": text, "aria-label": label || "Copy the command" }, "Copy"));
}
