// bernie dashboard: the theme of the page.
// The reader picks "auto" (the page follows the system), "dark" or "light".
// The file is loaded in the head of the page: the palette is set before the
// stylesheets paint, so the page never shows the other one for a moment.
"use strict";

const THEME_KEY = "bernie.theme";
const THEME_CHOICES = ["auto", "dark", "light"];

/** themeChoice is the choice of the reader. A choice the page does not know
 *  is "auto": the page follows the system. */
function themeChoice() {
  try {
    const v = localStorage.getItem(THEME_KEY) || "";
    return THEME_CHOICES.indexOf(v) < 0 ? "auto" : v;
  } catch (e) { return "auto"; }
}

/** systemDark says what the system asks for right now. A browser that does
 *  not know the question gets the dark palette, like the reference. */
function systemDark() {
  const mq = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)");
  return mq ? mq.matches : true;
}

/** applyTheme shows the palette of the choice and marks the button of it.
 *  The dark palette is the base of the stylesheet; the light one is the
 *  override. */
function applyTheme(choice) {
  const c = THEME_CHOICES.indexOf(choice) < 0 ? "auto" : choice;
  const dark = c === "dark" || (c === "auto" && systemDark());
  document.documentElement.dataset.theme = dark ? "dark" : "light";
  for (const btn of document.querySelectorAll("[data-theme-choice]")) {
    btn.setAttribute("aria-pressed", String(btn.dataset.themeChoice === c));
  }
}

/** setTheme picks a choice, remembers it, shows it and says it. */
function setTheme(choice) {
  if (THEME_CHOICES.indexOf(choice) < 0) return;
  try { localStorage.setItem(THEME_KEY, choice); } catch (e) { /* the page still works */ }
  applyTheme(choice);
  const live = document.getElementById("themeLive");
  if (live) live.textContent = choice === "auto" ? "The theme follows the system." : "The theme is " + choice + ".";
}

/** watchSystemTheme follows the system while the choice is "auto": a laptop
 *  that changes its palette at sunset changes the page with it. */
function watchSystemTheme() {
  const mq = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)");
  if (!mq) return;
  const on = () => { if (themeChoice() === "auto") applyTheme("auto"); };
  if (mq.addEventListener) mq.addEventListener("change", on);
  else if (mq.addListener) mq.addListener(on);
}

// The palette, before the first paint.
applyTheme(themeChoice());
