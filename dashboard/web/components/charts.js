// Shared chart primitives. They use canvas or inline SVG and no chart library.
"use strict";

/** spark draws a small canvas sparkline. */
function spark(canvas, values, max) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth, hgt = canvas.clientHeight;
  if (!w || !hgt) return;
  if (canvas.width !== w * dpr || canvas.height !== hgt * dpr) {
    canvas.width = w * dpr;
    canvas.height = hgt * dpr;
  }
  const c = canvas.getContext("2d");
  c.setTransform(dpr, 0, 0, dpr, 0, 0);
  c.clearRect(0, 0, w, hgt);
  if (values.length < 2) return;
  const top = max || Math.max(1, ...values);
  const step = w / (HISTORY - 1);
  const x0 = w - (values.length - 1) * step;
  const color = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#b3122c";
  c.beginPath();
  values.forEach((v, i) => {
    const y = hgt - 1 - (Math.min(v, top) / top) * (hgt - 2);
    i ? c.lineTo(x0 + i * step, y) : c.moveTo(x0, y);
  });
  c.strokeStyle = color;
  c.lineWidth = 1.5;
  c.stroke();
  c.lineTo(w, hgt);
  c.lineTo(x0, hgt);
  c.closePath();
  c.globalAlpha = 0.15;
  c.fillStyle = color;
  c.fill();
  c.globalAlpha = 1;
}

/** sparkSvg builds a sparkline of the last values as plain SVG: a line and
 *  the area under it. */
function sparkSvg(values, max, label) {
  const ns = "http://www.w3.org/2000/svg";
  const w = 120, hgt = 34, pad = 1;
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("class", "spark-svg");
  svg.setAttribute("viewBox", "0 0 " + w + " " + hgt);
  svg.setAttribute("preserveAspectRatio", "none");
  svg.setAttribute("role", "img");
  if (label) svg.setAttribute("aria-label", label);
  const area = document.createElementNS(ns, "path");
  area.setAttribute("class", "spark-area");
  const line = document.createElementNS(ns, "path");
  line.setAttribute("class", "spark-line");
  svg.append(area, line);
  if (!values || values.length < 2) return svg;
  const top = max || Math.max(1, ...values);
  const step = w / (values.length - 1);
  let d = "";
  values.forEach((v, i) => {
    const x = (i * step).toFixed(1);
    const y = (hgt - pad - (Math.min(v, top) / top) * (hgt - 2 * pad)).toFixed(1);
    d += (i ? "L" : "M") + x + " " + y;
  });
  line.setAttribute("d", d);
  area.setAttribute("d", d + " L " + w + " " + hgt + " L 0 " + hgt + " Z");
  return svg;
}
