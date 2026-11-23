# Configuration

File: `~/.sentrix/config.json` (override with `sentrix -config PATH`). Missing file = defaults;
omitted fields keep defaults. Created with mode 0600.

| Key | Meaning |
|---|---|
| data_dir / output_dir | project storage / export location (`~` expands) |
| concurrency | parallel sources, 1-32 |
| rate_per_second | per-source request ceiling; the lower of this and the source's own limit applies |
| export_formats | any of json, csv, txt, html |
| log_level | debug, info, warn, error (stderr) |
| theme | `auto` or `mono`; `NO_COLOR` and `TERM=dumb` also disable colour |
| startup_animation | reveal the banner row by row (cosmetic; disabled with `-no-anim`) |
| proxy | explicit proxy URL; no proxy is used otherwise, not even from the environment |
| sources.NAME | enabled, api_key, rate_per_second, timeout_seconds |

Credentials: `SENTRIX_URLSCAN_API_KEY`, `SENTRIX_OTX_API_KEY` override `api_key`.
