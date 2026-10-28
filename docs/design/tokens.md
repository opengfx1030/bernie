# Dashboard v3 — the tokens

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); this file holds §3 of it.

The tokens live in `dashboard/web/css/tokens.css`. The names are ours.
Every pair below was measured with the WCAG 2.1 relative luminance formula;
the numbers are the ratios, not a target.

## Light palette

| Token | Hex | Role |
|---|---|---|
| `--bg` | `#f4f5f7` | the page |
| `--surface` | `#ffffff` | cards |
| `--surface-2` | `#eef1f5` | command blocks, wells |
| `--surface-3` | `#e4e8ee` | hover, the open tab, banners |
| `--border` | `#dfe3e8` | hairlines between rows and sections (decorative) |
| `--border-strong` | `#828a93` | the edge of a control (3:1) |
| `--text` | `#14181d` | the text |
| `--text-2` | `#47505c` | the second level of text |
| `--muted` | `#5f6874` | labels, units, captions |
| `--accent` | `#b3122c` | the identity: the active tab, the links, the accent shadow |
| `--on-accent` | `#ffffff` | the text on the accent |
| `--key` | `#8a4f00` | the one number that matters on a screen |
| `--ok` `--warn` `--serious` `--fail` `--skip` `--info` | `#1a7a33` `#8a6100` `#c2410c` `#b42318` `#5f6874` `#2a5fb4` | the four-step status scale and the two quiet ones |
| `--series-1..6` | `#2a78d6` `#12795a` `#9a6200` `#c0392b` `#7c5cff` `#0f766e` | the categorical colours of the charts |
| `--seq-1..6` | `#dfe9f6` `#cfe0f7` `#9ec5f4` `#5598e7` `#256abf` `#104281` | the sequential ramp of the matrix |

| Pair | Hex | Ratio | Need |
|---|---|---|---|
| text on bg | `#14181d` on `#f4f5f7` | 16.34 | 4.5 |
| text on surface | `#14181d` on `#ffffff` | 17.82 | 4.5 |
| text on surface-2 | `#14181d` on `#eef1f5` | 15.73 | 4.5 |
| text on surface-3 | `#14181d` on `#e4e8ee` | 14.49 | 4.5 |
| text-2 on surface | `#47505c` on `#ffffff` | 8.17 | 4.5 |
| text-2 on surface-2 | `#47505c` on `#eef1f5` | 7.21 | 4.5 |
| muted on surface | `#5f6874` on `#ffffff` | 5.65 | 4.5 |
| muted on surface-2 | `#5f6874` on `#eef1f5` | 4.98 | 4.5 |
| muted on surface-3 | `#5f6874` on `#e4e8ee` | 4.59 | 4.5 |
| accent on surface | `#b3122c` on `#ffffff` | 6.91 | 4.5 |
| accent on surface-2 | `#b3122c` on `#eef1f5` | 6.10 | 4.5 |
| on-accent on accent | `#ffffff` on `#b3122c` | 6.91 | 4.5 |
| key on surface | `#8a4f00` on `#ffffff` | 6.56 | 4.5 |
| key on surface-2 | `#8a4f00` on `#eef1f5` | 5.79 | 4.5 |
| ok on surface | `#1a7a33` on `#ffffff` | 5.42 | 4.5 |
| warn on surface | `#8a6100` on `#ffffff` | 5.54 | 4.5 |
| serious on surface | `#c2410c` on `#ffffff` | 5.18 | 4.5 |
| fail on surface | `#b42318` on `#ffffff` | 6.57 | 4.5 |
| skip on surface | `#5f6874` on `#ffffff` | 5.65 | 4.5 |
| info on surface | `#2a5fb4` on `#ffffff` | 6.19 | 4.5 |
| ok on surface-2 | `#1a7a33` on `#eef1f5` | 4.78 | 4.5 |
| warn on surface-2 | `#8a6100` on `#eef1f5` | 4.89 | 4.5 |
| serious on surface-2 | `#c2410c` on `#eef1f5` | 4.57 | 4.5 |
| fail on surface-2 | `#b42318` on `#eef1f5` | 5.80 | 4.5 |
| skip on surface-2 | `#5f6874` on `#eef1f5` | 4.98 | 4.5 |
| info on surface-2 | `#2a5fb4` on `#eef1f5` | 5.46 | 4.5 |
| border-strong on surface | `#828a93` on `#ffffff` | 3.50 | 3.0 |
| border-strong on bg | `#828a93` on `#f4f5f7` | 3.21 | 3.0 |
| border-strong on surface-2 | `#828a93` on `#eef1f5` | 3.09 | 3.0 |
| series-1 on surface | `#2a78d6` on `#ffffff` | 4.42 | 3.0 |
| series-2 on surface | `#12795a` on `#ffffff` | 5.37 | 3.0 |
| series-3 on surface | `#9a6200` on `#ffffff` | 5.10 | 3.0 |
| series-4 on surface | `#c0392b` on `#ffffff` | 5.44 | 3.0 |
| series-5 on surface | `#7c5cff` on `#ffffff` | 4.35 | 3.0 |
| series-6 on surface | `#0f766e` on `#ffffff` | 5.47 | 3.0 |

`--border` is a decorative hairline and is exempt from the 3:1 rule.
`--seq-1` and `--seq-2` are tints of a ramp and carry no contrast promise:
every cell of a matrix also carries its value as text.

## Dark palette

| Token | Hex |
|---|---|
| `--bg` | `#0f1216` |
| `--surface` | `#171b21` |
| `--surface-2` | `#10141a` |
| `--surface-3` | `#21262e` |
| `--border` | `#2a3038` |
| `--border-strong` | `#5a6778` |
| `--text` | `#e6e8eb` |
| `--text-2` | `#b6bec8` |
| `--muted` | `#8b949e` |
| `--accent` | `#ff5c6c` |
| `--on-accent` | `#16181c` |
| `--key` | `#f5c451` |
| `--ok` `--warn` `--serious` `--fail` `--skip` `--info` | `#3fb950` `#d29922` `#ec835a` `#f85149` `#7d8590` `#58a6ff` |
| `--series-1..6` | `#3987e5` `#199e70` `#c98500` `#d03b3b` `#a78bfa` `#2dd4bf` |
| `--seq-1..6` | `#23242a` `#184f95` `#256abf` `#3987e5` `#6da7ec` `#9ec5f4` |

| Pair | Hex | Ratio | Need |
|---|---|---|---|
| text on bg | `#e6e8eb` on `#0f1216` | 15.30 | 4.5 |
| text on surface | `#e6e8eb` on `#171b21` | 14.08 | 4.5 |
| text on surface-2 | `#e6e8eb` on `#10141a` | 15.05 | 4.5 |
| text on surface-3 | `#e6e8eb` on `#21262e` | 12.38 | 4.5 |
| text-2 on surface | `#b6bec8` on `#171b21` | 9.21 | 4.5 |
| text-2 on surface-2 | `#b6bec8` on `#10141a` | 9.84 | 4.5 |
| text-2 on surface-3 | `#b6bec8` on `#21262e` | 8.10 | 4.5 |
| muted on surface | `#8b949e` on `#171b21` | 5.62 | 4.5 |
| muted on surface-2 | `#8b949e` on `#10141a` | 6.00 | 4.5 |
| muted on surface-3 | `#8b949e` on `#21262e` | 4.94 | 4.5 |
| accent on surface | `#ff5c6c` on `#171b21` | 5.76 | 4.5 |
| accent on surface-2 | `#ff5c6c` on `#10141a` | 6.16 | 4.5 |
| on-accent on accent | `#16181c` on `#ff5c6c` | 5.92 | 4.5 |
| key on surface | `#f5c451` on `#171b21` | 10.61 | 4.5 |
| key on surface-2 | `#f5c451` on `#10141a` | 11.34 | 4.5 |
| ok on surface | `#3fb950` on `#171b21` | 6.80 | 4.5 |
| warn on surface | `#d29922` on `#171b21` | 6.85 | 4.5 |
| serious on surface | `#ec835a` on `#171b21` | 6.55 | 4.5 |
| fail on surface | `#f85149` on `#171b21` | 5.16 | 4.5 |
| skip on surface | `#7d8590` on `#171b21` | 4.63 | 4.5 |
| info on surface | `#58a6ff` on `#171b21` | 6.84 | 4.5 |
| ok on surface-2 | `#3fb950` on `#10141a` | 7.27 | 4.5 |
| warn on surface-2 | `#d29922` on `#10141a` | 7.32 | 4.5 |
| serious on surface-2 | `#ec835a` on `#10141a` | 7.00 | 4.5 |
| fail on surface-2 | `#f85149` on `#10141a` | 5.51 | 4.5 |
| skip on surface-2 | `#7d8590` on `#10141a` | 4.95 | 4.5 |
| info on surface-2 | `#58a6ff` on `#10141a` | 7.31 | 4.5 |
| border-strong on surface | `#5a6778` on `#171b21` | 3.00 | 3.0 |
| border-strong on bg | `#5a6778` on `#0f1216` | 3.26 | 3.0 |
| border-strong on surface-2 | `#5a6778` on `#10141a` | 3.21 | 3.0 |
| series-1 on surface | `#3987e5` on `#171b21` | 4.75 | 3.0 |
| series-2 on surface | `#199e70` on `#171b21` | 5.07 | 3.0 |
| series-3 on surface | `#c98500` on `#171b21` | 5.63 | 3.0 |
| series-4 on surface | `#d03b3b` on `#171b21` | 3.60 | 3.0 |
| series-5 on surface | `#a78bfa` on `#171b21` | 6.35 | 3.0 |
| series-6 on surface | `#2dd4bf` on `#171b21` | 9.28 | 3.0 |

## Type, spacing, radii, elevation, motion

| What | Rule |
|---|---|
| Display | `font: 900 clamp(34px, 4.4vw, 64px)/1.02 var(--font-display); letter-spacing: -.025em; text-transform: uppercase;` with a hard shadow `3px 3px 0 color-mix(in srgb, var(--accent) 32%, transparent)` |
| Hero number | `900 64px/1 var(--font-display)`, `--key` colour |
| Lede | 18 px, `--text-2`, `max-width: 70ch` |
| Eyebrow | 12 px 800, uppercase, `letter-spacing: .14em`, `--muted` |
| Section label | 13 px 800, uppercase, `letter-spacing: .08em`, `--muted` |
| Body | 14 px / 1.5 `--font-body` |
| Small | 12 px, `--muted` |
| Mono | 12.5 px `--font-mono` for commands, logs and JSON |
| Tile number | `900 30px/1.1` with the unit in 13 px `--muted` |
| Fonts | `system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif` and `ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`. No web font, no CDN. |
| Spacing | 4 8 12 16 20 24 32 34 48 56 80 px |
| Radii | 6 8 9 10 16 18 999 px |
| Elevation | `--shadow-1` `0 1px 2px rgba(16,22,28,.08)` and `--shadow-2` `0 6px 20px rgba(16,22,28,.12)`; the dark palette uses black at .5 and .55. Only the toast and the tooltip have a shadow. |
| Layout | content `padding: 34px clamp(16px, 4vw, 56px) 80px`; a view is at most 1240 px wide; a section is a 1 px top border with 24 px of padding above it; a column grid is `repeat(auto-fit, minmax(320px, 1fr))` with a 32 px gap; a card has an 18 px radius and 20/22 px of padding |
| Motion | 120 ms on hover and focus, 200 ms on a meter, 400 ms on a progress bar, a 220 ms rise and fade for a card that appears (40 ms between cards), an 800 ms spin on the running pill |
| Reduced motion | `prefers-reduced-motion: reduce` sets every transition and animation to 0 ms, removes the entrance and replaces the spin with the word `running` |

## Chart colours

| Rule | Value |
|---|---|
| Categorical | `--series-1` … `--series-6`, in that order. A chart uses at most four series at once. |
| Sequential | `--seq-1` … `--seq-6`, light to dark, for a matrix (the link classes of the topology). Every cell also carries its value as text. |
| The two arms of a comparison | the baseline is `--series-1`, the candidate is `--series-2`. |
| The verdict of a metric | better `--ok`, same `--skip`, worse `--fail`, noisy `--warn`. The chip always carries the word. |
| The status of a card | `--ok` `--warn` `--serious` `--fail` `--skip` `--info`, always with an icon and a word. |
| A missing value | a dashed line at the baseline of the axis and the text `–`. Never a zero. |
