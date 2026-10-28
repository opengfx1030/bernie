# Third-party material and licenses

rdnactl copies text only from Apache-2.0 sources. Everything else is linked.

| Source | License | Use in rdnactl |
|---|---|---|
| [opengfx1030/vllm-rdna](https://github.com/opengfx1030/vllm-rdna) (recipes, `tunableop/` rows) | Apache-2.0 | Recipes and TunableOp rows are ported and embedded. Keep the SPDX notice. |
| [opengfx1030/vllm-rdna-docker](https://github.com/opengfx1030/vllm-rdna-docker) | GPL-3.0 | Link only. rdnactl references the published images, not the Dockerfiles. |
| [BlivionIaG/v620_toolbox](https://github.com/BlivionIaG/v620_toolbox) | GPL-3.0 | Link only. `tune power` points to it when the VBIOS floor blocks a cap. |
| [BlivionIaG/gfx1030-wiki](https://blivioniag.github.io/gfx1030-wiki/) | none stated | Link only, through the `doc` field of checks. No prose is copied. |
| [crispyging/vllm-rdna-dash](https://github.com/crispyging/vllm-rdna-dash) | see repo | Ideas only (benchmark method, log doctor). No code is copied. |
| [vllm-project/vllm](https://github.com/vllm-project/vllm) | Apache-2.0 | Target engine. rdnactl runs it; it does not vendor it. |

Rule for contributors: do not paste code or prose from a source marked
"link only". Re-express facts in your own words and link the source.
