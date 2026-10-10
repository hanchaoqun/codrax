-- Synthetic, redistributable TraceStreamer-shaped second capture.
-- Build a NEW current.data with: sqlite3 current.data < current.sql
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INTEGER, end_ts INTEGER);
INSERT INTO trace_range VALUES (3000000000, 6000000000);
CREATE TABLE measure_filter (id, name, type, source_arg_set_id);
INSERT INTO measure_filter VALUES
 (10, 'gpufreq', 'measure_filter', 7),
 (20, 'gpufreq', 'measure_filter', 8),
 (11, 'gpu_state', 'measure_filter', 7),
 (30, 'vendor.sample', 'vendor_filter', 9007199254740993);
CREATE TABLE measure (type, ts, dur, value, filter_id);
INSERT INTO measure VALUES
 ('measure', 3900000000, 200000000, 0, 10),
 ('measure', 4100000000, 100000000, NULL, 10),
 ('measure', 4200000000, NULL, 250000000.25, 10),
 ('measure', 4250000000, 50000000, '0', 11),
 ('measure', 4050000000, 100000000, 1, 11),
 ('measure', 4100000000, 200000000, 750000000, 20),
 ('vendor_measure', 4400000000, 0, 9007199254740995, 30),
 ('measure', 4500000000, 50000000, 999000000, 10),
 ('measure', NULL, 50000000, 55, 10),
 ('measure', 4450000000, -1, 500000000, 10);
