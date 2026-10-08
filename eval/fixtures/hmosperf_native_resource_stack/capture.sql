-- Synthetic, redistributable native-hook stack capture; not a device recording.
-- Build a NEW SQLite file with sqlite3 capture.data < capture.sql.
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INT, end_ts INT);
INSERT INTO trace_range VALUES (10000000000, 10100000000);
CREATE TABLE process (ipid INT, pid INT, name TEXT);
INSERT INTO process VALUES (1, 101, 'Gallery'), (2, 202, 'Background');
CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT);
INSERT INTO thread VALUES (1, 101, 1, 'gallery-main', 10000000000, 1, 0), (2, 202, 2, 'background', 10000000000, 1, 0);
-- No scheduler witness: these are resource observations, not CPU execution.
CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT);
CREATE TABLE sched_slice (ts, dur, cpu, itid, end_state, priority);
CREATE TABLE callstack (id, ts, dur, callid, name, flag, cookie);
CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type);
CREATE TABLE syscall (ts, itid);
CREATE TABLE frame_slice (id, type, ts, itid);
CREATE TABLE native_hook (id INT, start_ts INT, end_ts INT, event_type TEXT, all_heap_size INT, itid INT, ipid INT, heap_size INT, callchain_id INT, addr INT, sub_type_id INT);
INSERT INTO native_hook VALUES (1, 10005000000, 10025000000, 'AllocEvent', 512, 1, 1, 512, 7, -1, NULL);
INSERT INTO native_hook VALUES (2, 10025000000, NULL, 'FreeEvent', 0, 1, 1, 512, 8, -1, NULL);
INSERT INTO native_hook VALUES (3, 10040000000, NULL, 'AllocEvent', 256, 1, 1, 256, 9, 4096, NULL);
INSERT INTO native_hook VALUES (4, 10015000000, NULL, 'AllocEvent', 128, 2, 2, 128, 7, 8192, NULL);
INSERT INTO native_hook VALUES (5, 10050000000, NULL, 'AllocEvent', 512, 1, 1, 256, 7, 12288, NULL);
CREATE TABLE data_dict (id INT, data TEXT);
INSERT INTO data_dict VALUES (100, 'malloc'), (101, 'ImageCache::reserve'), (102, 'UIFrame::render'), (103, 'free'), (104, 'buffer_cleanup');
INSERT INTO data_dict VALUES (200, '/system/lib/libc.so'), (201, '/app/libgallery.so');
INSERT INTO data_dict VALUES (300, 'ambiguous_A'), (300, 'ambiguous_B');
CREATE TABLE native_hook_frame (id INT, callchain_id INT, depth INT, ip INT, symbol_id INT, file_id INT, offset INT, symbol_offset INT, vaddr TEXT);
INSERT INTO native_hook_frame VALUES (1, 7, 0, -1, 100, 200, 16, 2, '0xffffffffffffffff');
INSERT INTO native_hook_frame VALUES (2, 7, 1, 4096, 101, 201, 32, 4, '0x1000');
INSERT INTO native_hook_frame VALUES (3, 7, 2, 8192, 102, 201, 48, 6, '0x2000');
-- Missing depth 1 and unresolved symbol are distinct from display truncation.
INSERT INTO native_hook_frame VALUES (4, 8, 0, 12288, 103, 200, 64, 8, '0x3000');
INSERT INTO native_hook_frame VALUES (5, 8, 2, 16384, NULL, 201, NULL, NULL, NULL);
-- Duplicate depth and dictionary conflict must not invent a unique stack.
INSERT INTO native_hook_frame VALUES (6, 9, 0, 20480, 100, 200, 80, 10, '0x5000');
INSERT INTO native_hook_frame VALUES (7, 9, 0, 24576, 104, 201, 96, 12, '0x6000');
INSERT INTO native_hook_frame VALUES (8, 9, 1, 28672, 300, 201, 112, 14, '0x7000');
