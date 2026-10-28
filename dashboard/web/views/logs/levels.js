// The level of a log line is decided here and nowhere else (design/logs.md).
// The rules are the words of the log: a line that says ERROR is an error
// line. The same rule picks the colour and the word in the gutter, so the
// meaning of a line never depends on the colour alone. The log doctor in the
// backend uses the same words, so the page and the doctor never disagree
// about a line.
"use strict";

// The words that make a line an error line.
const LOG_ERROR_MARKS = ["ERROR", "Traceback", "RuntimeError", "CUDA error", "Error:"];
// The words that make a line a warning line when it holds no error mark.
const LOG_WARN_MARKS = ["WARNING", "WARN"];
// The word the gutter writes for a level.
const LOG_LEVEL_WORDS = { error: "ERROR", warn: "WARN", info: "INFO" };

/** logLevel returns the level of one line of a log: error, warn or info. */
function logLevel(text) {
  const t = text == null ? "" : String(text);
  for (const mark of LOG_ERROR_MARKS) {
    if (t.includes(mark)) return "error";
  }
  for (const mark of LOG_WARN_MARKS) {
    if (t.includes(mark)) return "warn";
  }
  return "info";
}

/** logLevelWord is the word the gutter of the tail writes for a level. */
function logLevelWord(level) {
  return LOG_LEVEL_WORDS[level] || "INFO";
}
