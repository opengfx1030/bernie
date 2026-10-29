"use strict";
// ---------- Chat ----------
// One prompt, one answer. The tab asks the run that is serving and streams
// the answer back as it comes. It keeps no history: the prompt and the
// answer are not stored.

/** viewChat opens the Chat tab: the prompt, the button and the answer. */
async function viewChat(root) {
  const head = stateHead("Chat");
  const prompt = h("textarea.chat-prompt", {
    id: "chat-prompt", rows: "3", placeholder: "Ask the model one question",
    "aria-label": "Your prompt",
  });
  const send = h("button.primary", { type: "button", id: "chat-send" }, "Send");
  const stop = h("button", { type: "button", id: "chat-stop", hidden: true }, "Stop");
  const reasoning = h("div.chat-reasoning", { hidden: true });
  const reply = h("div.chat-reply");
  const metrics = h("p.chat-metrics");
  const note = h("p.chat-note", "No history is kept: the prompt and the answer are not stored.");
  const modelLine = h("p.chat-model", "Model: …");
  const box = h("div.chat-box", prompt, modelLine, h("div.chat-actions", send, stop));
  put(root, head, box, h("div.chat-answer", reasoning, reply, metrics), note);

  let controller = null;
  let busy = false;

  /** numbers writes the numbers of the answer: the time to the first token
   *  and the output speed, with their units. */
  function numbers(doc) {
    const parts = [];
    if (doc.ttft_ms != null) parts.push(doc.ttft_ms + " ms to the first token");
    if (doc.tok_s != null) parts.push(doc.tok_s + " tok/s");
    return parts.join(" · ");
  }

  /** event reads one frame of the stream: its name and its document. */
  function event(frame) {
    let name = "";
    let data = "";
    for (const line of frame.split("\n")) {
      if (line.startsWith("event: ")) name = line.slice(7).trim();
      else if (line.startsWith("data: ")) data += line.slice(6);
    }
    if (!name) return null;
    let doc = null;
    try { doc = JSON.parse(data); } catch { return null; }
    return { name: name, doc: doc };
  }

  /** stream sends the prompt and reads the answer as it comes. The handlers
   *  get the events of the stream. */
  async function stream(text, handlers) {
    controller = new AbortController();
    const response = await fetch("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Bernie": "1" },
      credentials: "same-origin",
      body: JSON.stringify({ prompt: text }),
      signal: controller.signal,
    });
    if (!response.ok) {
      let msg = response.status + " " + response.statusText;
      try {
        const doc = await response.json();
        if (doc && doc.error) msg = doc.error;
      } catch { /* the answer was not JSON */ }
      throw new Error(msg);
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    for (;;) {
      const chunk = await reader.read();
      if (chunk.done) break;
      buf += decoder.decode(chunk.value, { stream: true });
      let cut = buf.indexOf("\n\n");
      while (cut >= 0) {
        const frame = buf.slice(0, cut);
        buf = buf.slice(cut + 2);
        const ev = event(frame);
        if (ev && handlers[ev.name]) handlers[ev.name](ev.doc);
        cut = buf.indexOf("\n\n");
      }
    }
  }

  /** ask sends the prompt of the box and shows the answer as it comes. */
  async function ask() {
    if (busy) return;
    const text = prompt.value.trim();
    if (!text) {
      prompt.focus();
      return;
    }
    busy = true;
    send.disabled = true;
    stop.hidden = false;
    reply.textContent = "";
    reasoning.textContent = "";
    reasoning.hidden = true;
    metrics.textContent = "";
    try {
      await stream(text, {
        delta: (doc) => { reply.textContent += doc.text || ""; },
        reasoning: (doc) => {
          reasoning.hidden = false;
          reasoning.textContent += doc.text || "";
        },
        done: (doc) => { metrics.textContent = numbers(doc); },
        error: (doc) => { metrics.textContent = doc.error || "the answer failed"; },
      });
    } catch (e) {
      if (e && e.name === "AbortError") {
        metrics.textContent = "The answer was stopped.";
      } else {
        metrics.textContent = (e && e.message) || "the request failed";
      }
    } finally {
      busy = false;
      controller = null;
      send.disabled = false;
      stop.hidden = true;
    }
  }

  send.addEventListener("click", ask);
  stop.addEventListener("click", () => {
    if (controller) controller.abort();
  });
  prompt.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      ask();
    }
  });

  // The tab asks the run list: a box without a server under it would only
  // answer with an error. When a run is up, the model line shows the first
  // id the server reports (GET /api/models, ia.md R31); an older release
  // without /v1/models falls back to the recipe name.
  let serving = false;
  let modelName = "";
  try {
    const runs = runsOf(await api("/api/runs"));
    serving = runs.some((r) => r.state === "running");
    const live = runs.find((r) => r.state === "running") || runs[0];
    if (live) {
      try {
        const mod = await api("/api/models");
        const ids = mod && mod.running;
        if (Array.isArray(ids) && ids.length) modelName = ids[0];
      } catch { /* the live model id is best-effort */ }
      if (!modelName) {
        const plan = live.plan || {};
        modelName = plan.served_name || plan.model || live.recipe || live.name || "";
      }
    }
  } catch (e) { /* nothing serves: the empty state says so */ }
  modelLine.textContent = modelName ? "Model: " + modelName : "Model: –";
  if (!serving) {
    setHead(head, "NO MODEL IS SERVING.", "Start one on the Setup tab, then ask it a question.");
    put(box, emptyState("No model is serving on this host.",
      h("a", { href: "#setup" }, "Set up this box"),
      "bernie serve <recipe>"));
    send.disabled = true;
    prompt.disabled = true;
    return;
  }
  setHead(head, "THE MODEL IS SERVING.", "Ask one question: the answer comes back piece by piece.");
  prompt.focus();
}
