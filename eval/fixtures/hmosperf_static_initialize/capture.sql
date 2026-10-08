-- Synthetic SQLite protocol fixture, not a device recording.
-- Build a NEW capture.data with sqlite3 capture.data < capture.sql.
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INT, end_ts INT);
INSERT INTO trace_range VALUES (10000000000, 10100000000);
CREATE TABLE process (ipid INT, pid INT, name TEXT);
INSERT INTO process VALUES (1, 100, 'Gallery'), (2, 200, 'Background');
CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT);
INSERT INTO thread VALUES (1, 101, 1, 'gallery-loader', 10000000000, 0, 1), (2, 202, 2, 'background', 10000000000, 0, 0);
CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT);
INSERT INTO thread_state VALUES (1, 10000000000, 20000000, 0, 'Running');
CREATE TABLE sched_slice (ts, dur, cpu, itid, end_state, priority);
CREATE TABLE callstack (id, ts, dur, itid, callid, name, flag, cookie, chainId, depth);
CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type);
CREATE TABLE syscall (ts, dur, syscall_number, itid);
CREATE TABLE frame_slice (id, type, ts, itid);
CREATE TABLE native_hook (id, start_ts, end_ts, event_type, all_heap_size, itid, ipid);
CREATE TABLE static_initalize (id INT, ipid INT, tid INT, call_id INT, start_time INT, end_time INT, so_name TEXT, depth INT);
INSERT INTO static_initalize(rowid,id,ipid,tid,call_id,start_time,end_time,so_name,depth) VALUES
 (-1,0,1,101,1,10002000000,10008000000,'dlopen:libimage.so',0),
 (0,1,1,101,1,10030000000,10045000000,'dlopen:librender.so',0),
 (1,2,2,202,2,10010000000,10018000000,'dlopen:libbackground.so',0),
 (2,3,1,101,1,10060000000,10065000000,'dlopen:liblater.so',0);
