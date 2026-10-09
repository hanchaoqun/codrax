-- Synthetic, redistributable TraceStreamer-shaped process measurement input.
-- Build a new capture.data with sqlite3 < capture.sql; no external data used.
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INTEGER, end_ts INTEGER);
INSERT INTO trace_range VALUES (0, 3000000000);
CREATE TABLE process (ipid INTEGER, pid INTEGER, name TEXT, start_ts INTEGER, end_ts INTEGER);
INSERT INTO process VALUES (1, 100, 'app.alpha', 0, 3000000000), (2, 200, 'app.beta', 0, 3000000000);
CREATE TABLE process_measure_filter (id, name, ipid);
INSERT INTO process_measure_filter VALUES (10, 'Resident', 1), (20, 'Resident', 2), (11, 'Page faults', 1), (12, 'Allocation sample', 1);
CREATE TABLE process_measure (type, ts, dur, value, filter_id);
INSERT INTO process_measure VALUES
 ('process_measure', 900000000, 200000000, 1000, 10),
 ('process_measure', 1100000000, 200000000, 1400, 10),
 ('process_measure', 1300000000, 200000000, NULL, 10),
 ('process_measure', 1700000000, 200000000, 1800, 10),
 ('process_measure', 2000000000, 100000000, 9900, 10),
 ('process_measure', 1000000000, 250000000, 2000, 20),
 ('process_measure', 1250000000, 250000000, 1900, 20),
 ('process_measure', 1500000000, 250000000, 2100, 20),
 ('process_measure', 1750000000, 250000000, 2200, 20),
 ('process_measure', 1050000000, 0, 0, 11),
 ('process_measure', 1200000000, NULL, 9007199254740993, 12),
 ('process_measure', 1600000000, 100000000, 'unavailable', 10),
 ('process_measure', NULL, 100000000, 123, 10);
