# Deviations from the reference design

The Classic tab is the reference-parity port of the dashboard view of
vllm-rdna-dash (CrispyGinger, used with permission). It exists so the owner
of bernie and the author of the reference can put the two next to each other.
A row below says where the port leaves the reference on purpose. The wiring
rules of [dashboard/AGENTS.md](dashboard/AGENTS.md) win over the look of the
reference: a number that carries its unit and a gap that stays a gap are
wiring, not taste.

| Where | The reference | Here | Why |
|---|---|---|---|
| The clock of a card (Classic tab) | the bare number | the number with its unit ("2,330 MHz") | Every number carries its unit (data display rule 5). The reference reads the clock without one. |
