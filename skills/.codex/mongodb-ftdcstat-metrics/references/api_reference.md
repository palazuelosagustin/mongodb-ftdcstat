# mongodb-ftdcstat usage and local api reference

## cli usage

Build:

```bash
go build -o mongodb-ftdcstat ./cmd/ftdcstat
```

General form:

```bash
mongodb-ftdcstat <path-to-diagnostic-data-directory> [--view server|wt|system|network|repl|summary|all] [--interval N] [--avg DURATION] [--device DEVICE] [--from ISO_TIME] [--to ISO_TIME] [--json] [--web] [--listen ADDR] [--verbose] [--pressure]
```

The input must be a MongoDB FTDC `diagnostic.data` directory. The tool discovers `metrics.*`, `metrics.interim`, `interim*`, and exported JSON files and treats them as one chronological capture.

## command recipes

```bash
# default summary view
mongodb-ftdcstat diagnostic.data

# horizontally scroll the wide summary table
mongodb-ftdcstat diagnostic.data --view summary | less -S

# reduce long captures to fixed buckets
mongodb-ftdcstat diagnostic.data --view summary --avg 5m

# exact time window; from inclusive, to exclusive
mongodb-ftdcstat diagnostic.data --from "2026-06-04T19:00:00" --to "2026-06-04T20:00:00"

# json for programmatic analysis
mongodb-ftdcstat diagnostic.data --view summary --json

# local plotting ui
mongodb-ftdcstat diagnostic.data --web --view summary --avg 5m

# bind web ui to a specific local address
mongodb-ftdcstat diagnostic.data --web --listen 127.0.0.1:8080

# focused views
mongodb-ftdcstat diagnostic.data --view server
mongodb-ftdcstat diagnostic.data --view wt --verbose
mongodb-ftdcstat diagnostic.data --view system --verbose --pressure
mongodb-ftdcstat diagnostic.data --view network --verbose
mongodb-ftdcstat diagnostic.data --view repl --verbose

# disk-specific system metrics
mongodb-ftdcstat diagnostic.data --view system --device sda
```

## local web mode

`--web` starts a local HTTP server and still prints the terminal report. By default it binds to `127.0.0.1` on a random available port and prints the URL in a `webUI` header section.

Endpoints:

```text
GET /              -> embedded static index.html
GET /app.js        -> embedded javascript
GET /style.css     -> embedded css
GET /api/metadata  -> capture/header metadata
GET /api/data      -> selected derived rows or chart data
```

`/api/metadata` includes `headerText`, a terminal-style preformatted header that mirrors the CLI report header. `/api/data` returns the selected derived rows or chart data grouped by the same logical sections as the selected view.

Use `--avg` or `--from`/`--to` with large captures to keep browser rendering responsive.

## json mode

`--json` prints metadata, warnings, selected view, derived rows, and `rsInfo.members` mapping. Rows include grouped section objects such as `replication`, `server`, `network`, `system`, and `wiredTiger`. Missing values are `null`. `replication.lagS` contains per-node lag values and `replication.majLagS` contains majority commit lag.

`--json` cannot be combined with `--web`.
