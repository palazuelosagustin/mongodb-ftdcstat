# JSON output and local web API

## `--json`

One JSON document on stdout. Top-level keys:

| Key | Content |
|---|---|
| `toolVersion` | mongodb-ftdcstat version that wrote it (`v0.1.0`, or `dev (commit)` for a source build) |
| `view` | Selected view (`summary`, `server`, `wt`, `system`, `io`, `network`, `repl`) |
| `avg` | `{enabled, bucket, datetime}`; with `--avg`, `datetime` is `bucket_start` |
| `metadata` | Compact summary, same facts as the CLI header: `process`, `host {hostname, os, kernel, arch, cpuCores, memoryMB, maxConnections}`, `build {version, gitVersion, modules, storageEngine, allocator, openssl, perconaFeatures}`, `cmdLineOpts` (flat dotted keys, typed values), `parameters` (`wtCache` + explicit `setParameter`). Missing fields are omitted. Raw FTDC metadata documents are not output |
| `rsInfo` | `{set, members}`; `members` maps `node1..nodeN` to `host:port`. On mongos: `{"set": "-", "members": {}}` |
| `warnings` | Reader/derivation warnings, may be empty |
| `rows` | Derived rows, one per display interval or `--avg` bucket |

Each row has `time` (RFC3339 UTC), an optional `processMarker` string (restart
or process boundary), and one object per section of the view. Section keys:

- mongod: `replication`, `server`, `network`, `system`, `io`, `wiredTiger`
- mongos: `router`, `server`, `network`, `system`, `io`, `connPool`

Inside a section, keys are the column names from the terminal table
(`wtCache%`, `appEvict/s`, `awaitS`, ...). Special shapes:

- `replication.lagS` is an object keyed `node1..nodeN`; map through `rsInfo.members`.
- In `--view io`, per-device values are under `io.devices.<device>.<column>`. In other views `io` is the single selected/aggregate device.

`--json` is streamed by the tool (flat memory on any capture), but it is one
document: `jq` loads all of it. On long captures narrow it with `--from/--to`
or `--columns` before piping to `jq`.

Value semantics differ from the table:

- Values are **unrounded** floats (the table rounds to 1 or 3 decimals). Round before quoting them.
- Missing / not computable is `null` (the table's `-`). `0` is a real zero.
- Rows near a restart may omit rate fields entirely; use `?` / `// null` in `jq`.

### `jq` recipes

Keys contain `%` and `/`, so quote them with `."key"` or `["key"]`.

```bash
F="mongodb-ftdcstat DIR --json --avg 5m"

# buckets where application threads did eviction
$F | jq -r '.rows[] | select((.wiredTiger["appEvict/s"] // 0) > 0) | [.time, .wiredTiger["appEvict/s"]] | @tsv'

# peak cache fill and when
$F | jq -r '[.rows[] | select(.wiredTiger["wtCache%"] != null)] | max_by(.wiredTiger["wtCache%"]) | [.time, .wiredTiger["wtCache%"]] | @tsv'

# lag per real host
$F --view repl | jq -r '.rsInfo.members as $m | .rows[] | .time as $t | .replication.lagS // {} | to_entries[] | [$t, $m[.key], .value] | @tsv'

# restart / process boundaries
$F | jq -r '.rows[] | select(.processMarker) | [.time, .processMarker] | @tsv'

# server version and configured WT cache size
$F | jq '{version: .metadata.build.version, wtCache: .metadata.parameters.wtCache}'

# any startup option (flat dotted keys)
$F | jq '.metadata.cmdLineOpts["storage.dbPath"]'
```

Which `metadata` fields exist depends on what FTDC captured; missing ones are
omitted, so use `// null` or check with `jq '.metadata | keys'`.

## `--stats` / `--findings` with `--json`

A different document: top-level `reports` (`["stats"]`, `["findings"]`, or
both), `toolVersion`, `view`, `metadata`, `rsInfo`, `warnings`, `range` (`{start, end}`), and
the requested reports.

| Key | Content |
|---|---|
| `percentileRelativeAccuracy` | `0.01`; only with `--stats` |
| `stats.rows`, `stats.restarts`, `stats.gaps` | Row count and boundary counts |
| `stats.columns[]` | `{section, subsection?, column, n, nullPct, min, avg, p50, p95, p99, max, maxAt}`; value fields are `null` when `n` is 0; `subsection` is the device in `--view io` |
| `stats.states[]` | Text columns such as `rsState`: `{section, column, values: [{value, rows, pct}], changes}` |
| `findings.events[]` | `{time, type, ...}`; `process`/`restart`: `process`, `pid`, `start`; `gap`: `seconds`; `rsState`: `from`, `to`. Fields for other types are omitted |

```bash
# columns that ever moved, with peak time
mongodb-ftdcstat DIR --stats --json | jq -r '.stats.columns[] | select((.max // 0) > 0) | [.section, .column, .p99, .max, .maxAt] | @tsv'

# restarts and state changes
mongodb-ftdcstat DIR --findings --json | jq -r '.findings.events[] | select(.type != "gap") | [.time, .type, .pid // "", .from // "", .to // ""] | @tsv'
```

## `--web` / `--tui` HTTP endpoints

The server binds `127.0.0.1:<random>` (or `--listen ADDR`), prints the URL in
the `webUI` header section, and serves precomputed, read-only responses. Only
`GET` is accepted, and paths must match exactly (a query string returns 404). To
change the window or view, restart the tool with different flags.

| Path | Content |
|---|---|
| `/` | Chart UI (`--web`) |
| `/tui` | Scrollable table UI (`--tui`) |
| `/app.js`, `/style.css`, `/tui.js`, `/tui.css` | Embedded static assets |
| `/api/metadata` | `view`, `avg`, `timeRange {from,to}` (empty without `--from/--to`), `headerText` (the CLI header as preformatted text), `metadata` (the same compact summary as `--json`), `warnings`, `sections` (`[{name, metrics: [{column, label, jsonName, format, default}]}]`), `rowCount` |
| `/api/data` | `view`, `avg`, `sections` (name → column list), `rows` keyed by `datetime` and chart-group names such as `"wiredTiger / Tickets"` |
| `/api/table` | `columns` (`[{key, label, section, subsection, fixed}]`) and `rows` (`[{cells: [string]}]`), already formatted like the terminal table |

Fetch these with `curl -s http://127.0.0.1:PORT/api/metadata | jq` when you need
the web UI's exact data. For one-off analysis, `--json` is simpler: it needs no
background server.

Large captures make big responses and a slow browser; add `--avg` or
`--from/--to` first.
