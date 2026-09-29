"""Generate a new, never-overwritten main/WAL fixture using real SQLite."""
import pathlib
import shutil
import sqlite3
import tempfile

output = pathlib.Path(__file__).resolve().parent
for leaf in ("capture.data", "capture.data-wal"):
    if (output / leaf).exists():
        raise SystemExit(f"Refusing to overwrite {output / leaf}")
with tempfile.TemporaryDirectory(prefix="codrax-wal-fixture-") as work:
    source = pathlib.Path(work) / "capture.db"
    conn = sqlite3.connect(source)
    try:
        conn.executescript((output.parent / "hmosperf_referenced_dictionary" / "capture.sql").read_text())
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA wal_autocheckpoint=0")
        conn.execute("UPDATE app_startup SET end_time=1048000000 WHERE start_time=1040000000")
        conn.execute("INSERT INTO data_dict VALUES (301, 'RestoreTabs')")
        conn.execute("INSERT INTO app_startup VALUES (1064000000, 1072000000, 301, 1)")
        conn.commit()
        # Force real uncommitted spill pages; readers must not see this table.
        conn.execute("PRAGMA cache_size=1")
        conn.execute("BEGIN")
        conn.execute("CREATE TABLE private_uncommitted (payload BLOB)")
        conn.execute("INSERT INTO private_uncommitted VALUES (zeroblob(100000))")
        shutil.copyfile(source, output / "capture.data")
        shutil.copyfile(str(source) + "-wal", output / "capture.data-wal")
        conn.rollback()
    finally:
        conn.close()
