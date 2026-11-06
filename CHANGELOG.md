# Changelog

All notable changes are listed here. Versions follow
[semantic versioning](https://semver.org/); see "Versions and releases" in the
README.

## Unreleased

## v0.1.0 - 2026-11-06

First versioned release. It contains everything built so far:

- Terminal tables for `mongod` and `mongos` FTDC captures, with the views
  `summary`, `server`, `wt`, `system`, `network`, `repl` and `io`, and
  `--verbose` / `--pressure` metrics.
- `--columns` and `--list-columns` to pick columns from any view.
- `--avg` time buckets, `--from` / `--to` ranges, `--device` for `io`.
- `--stats` (per-column distribution) and `--findings`.
- `--json`, streamed one row at a time, with a compact metadata summary.
- `--web` dashboard and `--tui` web table view.
- Replication lag per member, oplog fetch/serve columns, and 7.0 path
  fallbacks.
- Claude and Codex agent skills.
- `--version`, and `toolVersion` in every JSON output.
