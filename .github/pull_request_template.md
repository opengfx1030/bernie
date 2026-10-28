# What this change does

<!-- One or two sentences. What is different after this pull request? -->

## Receipt

<!--
The receipt of a benchmark run. Fill it in, or write "not run, and why".

Run the suite on the target box:

    bernie bench --suite pr --url http://127.0.0.1:8000

Then compare the baseline with the candidate and paste the receipt:

    bernie bench compare <baseline-id> <candidate-id> --markdown

`bench ab --base ARM --cand ARM` runs both arms back to back and writes
the same receipt.
-->

| | |
|---|---|
| Environment | <!-- host, GPU, ROCm, vLLM or image, model and revision --> |
| Baseline | <!-- result id of the baseline run --> |
| Candidate | <!-- result id of the candidate run --> |
| Correctness | <!-- the recall gate: asked and passed per cell --> |
| Decode speed before / after | <!-- from the receipt --> |
| Prompt speed before / after | <!-- from the receipt --> |
| Verdict | <!-- better / same / worse, and the exit code of `bench compare` --> |
| Tests run | <!-- pass, fail and skip counts --> |
| Not run | <!-- what was not measured, and why --> |

## Checklist

- [ ] The change is on one topic.
- [ ] `docs/spec.md` follows the behavior.
- [ ] `make check` passes.
- [ ] The receipt above is filled in, or says why there is none.
- [ ] The description names the platforms it was tested on, and the ones it
      could not be tested on.
