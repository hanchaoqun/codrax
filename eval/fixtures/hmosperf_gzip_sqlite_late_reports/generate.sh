#!/usr/bin/env bash
set -euo pipefail
fixture_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for name in capture.data capture.data.gz capture.transport; do
  if [[ -e "$fixture_dir/$name" ]]; then
    echo "Refusing to overwrite existing fixture: $fixture_dir/$name" >&2
    exit 1
  fi
done
sqlite3 "$fixture_dir/capture.data" < "$fixture_dir/capture.sql"
sqlite3 "$fixture_dir/capture.data" 'PRAGMA integrity_check;'
gzip -n -k "$fixture_dir/capture.data"
mv "$fixture_dir/capture.data.gz" "$fixture_dir/capture.transport"
gzip -dc "$fixture_dir/capture.transport" | cmp - "$fixture_dir/capture.data"
