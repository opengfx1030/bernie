"use strict";
// ---------- Dashboard ----------
// Dashboard groups the live host view: the GPU telemetry, the power service,
// the PCIe topology and the runs that are serving.

async function viewDashboard(root) {
  const gpus = h("section.gpus-section");
  const topology = h("section.topology-section");
  const overview = h("section.runs-overview");
  put(root, gpus, topology, overview);
  await viewGPUs(gpus);
  await viewTopology(topology);
  await renderDashboardOverview(overview);
}
