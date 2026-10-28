// Shared command block and copy button.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
// Every command of the page is text: it is never markup.
"use strict";

/** copyBtn is a copy button for a command that is shown inline: the fix of a
 *  finding, a flag, a URL. */
function copyBtn(text, label) {
  return h("button.small.copy", {
    type: "button",
    "data-copy": text,
    "aria-label": label || "Copy the command",
  }, "Copy");
}

/** cmdBlock shows a command with its copy button. The text is the command as
 *  it is run. */
function cmdBlock(text, label) {
  if (!text) return null;
  return h("div.cmd",
    h("code", text),
    h("button.copy", {
      type: "button",
      "data-copy": text,
      "aria-label": label || "Copy the command",
    }, "Copy"));
}
