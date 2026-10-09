-- Synthetic, redistributable TraceStreamer-shaped generic measurements.
-- Build a NEW capture.data with: sqlite3 capture.data < capture.sql
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INTEGER, end_ts INTEGER);
INSERT INTO trace_range VALUES (0, 3000000000);
CREATE TABLE measure_filter (id, name, type, source_arg_set_id);
INSERT INTO measure_filter VALUES
 (10, 'gpufreq', 'measure_filter', 7),
 (20, 'gpufreq', 'measure_filter', 8),
 (11, 'gpu_state', 'measure_filter', 7),
 (12, 'gpuload', 'measure_filter', NULL),
 (30, 'vendor.sample', 'vendor_filter', 9007199254740993),
 (40, 'ambiguous.first', 'measure_filter', 7),
 (40, 'ambiguous.second', 'measure_filter', 8);
CREATE TABLE measure (type, ts, dur, value, filter_id);
INSERT INTO measure VALUES
 ('measure', 900000000, 300000000, 334200000.5, 10),
 ('measure', 1200000000, 200000000, 480000000, 10),
 ('measure', 1450000000, 100000000, NULL, 10),
 ('measure', 1600000000, NULL, 600000000, 10),
 ('measure', 2000000000, 100000000, 999000000, 10),
 ('measure', 1000000000, 500000000, 800000000, 20),
 ('measure', 1000000000, 250000000, 1, 11),
 ('measure', 1250000000, 250000000, 0, 11),
 ('measure', 1500000000, 100000000, '1', 11),
 ('measure', 1100000000, 100000000, 42.5, 12),
 ('vendor_measure', 1750000000, 0, 9007199254740993, 30),
 ('measure', 1800000000, 50000000, X'0031', 999),
 ('measure', 1850000000, 50000000, 7, 40),
 ('measure', NULL, 100000000, 123, 10),
 ('measure', 1950000000, -1, 500000000, 10);
