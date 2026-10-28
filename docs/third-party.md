# Third-party material and licenses

bernie copies text only from Apache-2.0 sources. Everything else is linked.

| Source | License | Use in bernie |
|---|---|---|
| [opengfx1030/vllm-rdna](https://github.com/opengfx1030/vllm-rdna) (recipes, `tunableop/` rows) | Apache-2.0 | Recipes and TunableOp rows are ported and embedded. Keep the SPDX notice. |
| [opengfx1030/vllm-rdna-docker](https://github.com/opengfx1030/vllm-rdna-docker) | GPL-3.0 | Link only. bernie references the published images, not the Dockerfiles. |
| [BlivionIaG/v620_toolbox](https://github.com/BlivionIaG/v620_toolbox) | GPL-3.0 | Link only. `tune power` points to it when the VBIOS floor blocks a cap. |
| [BlivionIaG/gfx1030-wiki](https://blivioniag.github.io/gfx1030-wiki/) | none stated | Link only, through the `doc` field of checks. No prose is copied. |
| [crispyging/vllm-rdna-dash](https://github.com/crispyging/vllm-rdna-dash) | see repo | Ideas only (benchmark method, log doctor). No code is copied. |
| [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) v1.8.0 | MIT and Apache-2.0 (the SDK's own license file) | Linked. The MCP server (`internal/mcp`) uses it. Modules it brings into the binary: `google/jsonschema-go` (MIT), `segmentio/encoding` and `segmentio/asm` (MIT), `yosida95/uritemplate` (BSD-3-Clause), `golang.org/x/oauth2`, `x/sync`, `x/sys` and `x/time` (BSD-3-Clause). |
| [vllm-project/vllm](https://github.com/vllm-project/vllm) | Apache-2.0 | Target engine. bernie runs it; it does not vendor it. |
| PCI id database, Linux kernel amdgpu/KFD sources, LLVM AMDGPU docs, ROCm docs | various | Facts only: PCI ids, product names, die-to-gfx maps, wave size. bernie states the facts in its own tables (`internal/machine/gpu/devices/*.yaml`). The header of each file links its sources. No text is copied. |

Rule for contributors: do not paste code or prose from a source marked
"link only". Re-express facts in your own words and link the source.
