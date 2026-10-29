// "Show the exact launch command" disclosure of the Setup tab's recipe
// plan. The file is loaded lazily so the first paint does not carry it.
// See ia.md §5 R42.
"use strict";

/** setupPlanLaunchDisclosure is the "Show the exact launch command"
 *  block: the host command always; the compose.yml when the plan is
 *  container mode. The headline names what is inside (ia.md §5 R42). */
function setupPlanLaunchDisclosure(plan) {
  const launch = h("div.recipe-plan-launch",
    h("h4", "The launch command"),
    cmdBlock(setupPlanCommand(plan), "Copy the launch command"));
  if (plan.mode !== "container" || !plan.compose) {
    return h("details.advanced.recipe-plan-details",
      h("summary", "Show the exact launch command"),
      launch);
  }
  const composeBox = h("div.recipe-plan-compose",
    h("h4", "The generated compose.yml"),
    cmdBlock(plan.compose, "Copy the compose file"),
    h("p.small.muted",
      "Save the file, then start it with the engine of the host: ",
      h("code", "docker compose -f FILE up -d"),
      " or ",
      h("code", "podman compose -f FILE up -d"),
      "."));
  return h("details.advanced.recipe-plan-details",
    h("summary", "Show the exact launch command"),
    launch,
    composeBox);
}
