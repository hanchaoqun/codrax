-- Synthetic, redistributable, closed TraceStreamer-shaped input.
-- Regenerate a NEW file with: sqlite3 capture.data < capture.sql
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INT);
INSERT INTO trace_range VALUES (0);
CREATE TABLE process (ipid INT, pid INT, name TEXT);
INSERT INTO process VALUES (1, 27599, 'DocumentApp');
CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT);
INSERT INTO thread VALUES (1, 27599, 1, 'main', 0, 1, 1);
CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT);
INSERT INTO thread_state VALUES (1, 0, 3000000000, 2, 'Running');
CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT);
INSERT INTO callstack VALUES (1, 0, 0, 1, 'zero-time-marker', '', NULL);
INSERT INTO callstack VALUES (2, 1000000000, 50000000, 1, 'OpenDocument', '', NULL);
INSERT INTO callstack VALUES (3, 1041000000, 0, 1, 'jank_event_sync: start_ts=1020000000, end_ts=1040000000, jank_frames=2, appid=27599', '', NULL);
INSERT INTO callstack VALUES (4, 1100000000, 90000000, 1, 'UploadTexture', '', NULL);
INSERT INTO callstack VALUES (5, 1161000000, 0, 1, 'jank_event_sync: start_ts=1100000000, end_ts=1160000000, jank_frames=4, appid=27599', '', NULL);
INSERT INTO callstack VALUES (6, 2021000000, 0, 1, 'jank_event_sync: start_ts=2000000000, end_ts=2020000000, jank_frames=1, appid=27599', '', NULL);
INSERT INTO callstack VALUES (7, 2990000000, 5000000, 1, 'tail-business-marker', '', NULL);
CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type);
CREATE TABLE sched_slice (ts, dur, cpu, itid, end_state, priority);
CREATE TABLE syscall (ts, itid);
CREATE TABLE native_hook (start_ts, itid);
CREATE TABLE frame_slice (id, type, ts, itid);
-- Unknown/vendor data must survive fidelity export without gaining causality.
CREATE TABLE vendor_payload (sequence INT, payload TEXT);
INSERT INTO vendor_payload VALUES (9007199254740993, 'opaque annotation');
