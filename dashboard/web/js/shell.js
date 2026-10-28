// Dashboard shell helpers: the toast and the copy control. The shell owns
// these page-wide controls; views only call them.
"use strict";

let toastTimer;

function toast(msg, ok) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = ok ? "ok" : "";
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), ok ? 3000 : 8000);
}

/** copyText puts a text on the clipboard. It falls back to a hidden text area
 *  when the page is not a secure context (a plain http address on the LAN). */
async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (e) { /* fall through to the text area */ }
  try {
    const area = h("textarea", { style: "position:fixed;left:-9999px;top:0" });
    area.value = text;
    document.body.append(area);
    area.select();
    const ok = document.execCommand("copy");
    area.remove();
    return ok;
  } catch (e) { return false; }
}

/** copySource reads the text a copy button belongs to: the code of the command
 *  block around it. */
function copySource(btn) {
  const box = btn.closest(".cmd") || btn.parentElement;
  const code = box && box.querySelector("code, pre");
  return code ? code.textContent.trim() : "";
}

// One handler for every copy button on the page: the button carries the text
// in data-copy, or takes it from the command block it sits in.
document.addEventListener("click", async (ev) => {
  const target = ev.target instanceof Element ? ev.target : null;
  const btn = target && target.closest("[data-copy]");
  if (!btn) return;
  const text = btn.getAttribute("data-copy") || copySource(btn);
  if (!text) { toast("There is nothing to copy."); return; }
  const ok = await copyText(text);
  toast(ok ? "Copied to the clipboard." : "Copy failed: select the text and copy it.", ok);
  if (ok) {
    const label = btn.textContent;
    btn.textContent = "Copied";
    setTimeout(() => { btn.textContent = label; }, 1200);
  }
});
