# Tuned fused-MoE configs

vLLM's fused-MoE kernel reads one JSON file per MoE shape from the folder in
`VLLM_TUNED_CONFIG_FOLDER`. The folder must be flat. The files are specific
to a GPU architecture, so rdnactl keeps one folder per architecture:

```text
tuned-moe/
  gfx1030/
    E=128,N=640,device_name=AMD_Radeon_Pro_V620,dtype=int4_w4a16.json
```

## How rdnactl uses them

- Host mode copies the folder of the selected GPUs' architecture to
  `<data>/tuned-moe/<arch>/` and sets `VLLM_TUNED_CONFIG_FOLDER` to it.
- A folder name is a gfx name (`gfx1030`) or a prefix (`gfx103`). The longest
  name that is a prefix of the GPUs' gfx target wins.
- An architecture with no folder gets no `VLLM_TUNED_CONFIG_FOLDER`. vLLM then
  uses its built-in configs and the plan shows no warning.
- Container mode is unchanged: the recipe's value is a path inside the image.

## Add an architecture

1. Create `tuned-moe/<arch>/` (for example `tuned-moe/gfx1100/`).
2. Put the config files in it. vLLM requires this file name:

   ```text
   E=<experts>,N=<shard intermediate size>,device_name=<device>,dtype=<dtype>.json
   ```

   `device_name` is the GPU name with spaces replaced by `_`
   (`AMD_Radeon_Pro_V620`). Use a `dtype` such as `int4_w4a16` or `fp8_w8a8`.
   Add `,block_shape=[128,128]` after `dtype` for block-quantized models.
3. Keep the folder flat. Do not add sub-folders.
4. Run `go test ./internal/serving/tuning/...`.

No Go code changes. The embedded tree is discovered by walking it.
