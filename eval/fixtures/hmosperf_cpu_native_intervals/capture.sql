-- Synthetic, redistributable TraceStreamer-shaped explicit CPU intervals.
-- Regenerate a NEW file with: sqlite3 capture.data < capture.sql
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INT);
INSERT INTO trace_range VALUES (0);
CREATE TABLE process (ipid INT, pid INT, name TEXT);
INSERT INTO process VALUES (1, 101, 'IntervalDemo');
CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT);
INSERT INTO thread VALUES (1, 101, 1, 'main', 0, 1, 1);
CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT);
INSERT INTO thread_state VALUES (1, 0, 40000000, 0, 'Running');
CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT);
INSERT INTO callstack VALUES (1, 0, 40000000, 1, 'RenderScene', '', NULL);
CREATE TABLE cpu_measure_filter (id INT, name TEXT, cpu INT);
INSERT INTO cpu_measure_filter VALUES (1, 'cpu_frequency', 0), (2, 'cpu_idle', 0), (3, 'cpu_frequency', 1), (4, 'cpu_idle', 1);
CREATE TABLE measure (id INT, ts INT, dur INT, value INT, filter_id INT);
-- Carry-in, two genuine holes, and a tail extending past the requested end.
INSERT INTO measure VALUES (1, -10000000, 30000000, 1000000, 1);
INSERT INTO measure VALUES (2, 25000000, 25000000, 2000000, 1);
INSERT INTO measure VALUES (3, -5000000, 15000000, 0, 2);
INSERT INTO measure VALUES (4, 20000000, 10000000, 1, 2);
INSERT INTO measure VALUES (5, 20000000, 20000000, 800000, 3);
INSERT INTO measure VALUES (6, 0, 40000000, 2, 4);
CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type);
CREATE TABLE sched_slice (ts, dur, cpu, itid, end_state, priority);
CREATE TABLE syscall (ts, itid);
CREATE TABLE native_hook (start_ts, itid);
CREATE TABLE frame_slice (id, type, ts, itid);
