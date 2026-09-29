"""Generate a checkpoint plus real uncommitted WAL; never overwrite captures."""
import pathlib
import shutil
import sqlite3
import tempfile

output = pathlib.Path(__file__).resolve().parent
for leaf in ("capture.data", "capture.data-wal"):
    if (output / leaf).exists():
        raise SystemExit(f"Refusing to overwrite {output / leaf}")
with tempfile.TemporaryDirectory(prefix="codrax-checkpoint-fixture-") as work:
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
        conn.execute("PRAGMA wal_checkpoint(TRUNCATE)")
        conn.execute("PRAGMA cache_size=1")
        conn.execute("PRAGMA cache_spill=1")
        conn.execute("BEGIN")
        conn.execute("UPDATE app_startup SET end_time=1052000000 WHERE start_time=1040000000")
        conn.execute("INSERT INTO app_startup VALUES (1073000000, 1079000000, 'PendingOnly', 1)")
        conn.execute("CREATE TABLE private_uncommitted (payload BLOB)")
        conn.execute("INSERT INTO private_uncommitted VALUES (zeroblob(1048576))")
        # Independent SQLite reader sees the checkpoint, not the writer's rows.
        with sqlite3.connect(f"file:{source}?mode=ro", uri=True) as reader:
            assert reader.execute("SELECT start_time,end_time FROM app_startup WHERE start_time>=1040000000 AND start_time<1080000000 ORDER BY start_time").fetchall() == [(1040000000, 1048000000), (1064000000, 1072000000)]
        assert pathlib.Path(str(source) + "-wal").stat().st_size > 32
        shutil.copyfile(source, output / "capture.data")
        shutil.copyfile(str(source) + "-wal", output / "capture.data-wal")
        conn.rollback()
    finally:
        conn.close()
