#!/usr/bin/env bash
set -euo pipefail
fixture_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for side in baseline current; do
  if [[ -e "$fixture_dir/$side.data" ]]; then
    echo "Refusing to overwrite existing fixture: $fixture_dir/$side.data" >&2
    exit 1
  fi
done
cp "$fixture_dir/../hmosperf_dual_measurements/baseline.data" "$fixture_dir/baseline.data"
cp "$fixture_dir/../hmosperf_common_window_measurements/current.data" "$fixture_dir/current.data"
sqlite3 "$fixture_dir/baseline.data" < "$fixture_dir/baseline_metadata.sql"
sqlite3 "$fixture_dir/current.data" < "$fixture_dir/current_metadata.sql"
sqlite3 "$fixture_dir/baseline.data" 'PRAGMA integrity_check;'
sqlite3 "$fixture_dir/current.data" 'PRAGMA integrity_check;'
