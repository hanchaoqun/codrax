-- Synthetic closed SQLite. Rebuild a new capture.data with sqlite3 < capture.sql.
PRAGMA journal_mode=DELETE;
CREATE TABLE trace_range (start_ts INTEGER, end_ts INTEGER);
INSERT INTO trace_range VALUES (0, 3000000000);
CREATE TABLE process (ipid INTEGER, pid INTEGER, name TEXT, start_ts INTEGER, end_ts INTEGER);
INSERT INTO process VALUES (1,100,'render_service',0,3000000000),(2,200,'app.video',0,3000000000);
CREATE TABLE process_measure_filter (id, name, ipid);
INSERT INTO process_measure_filter VALUES (10,'H:PreferredFrameRate',1),(11,'H:PreferredFrameRate',1),(20,'H:PreferredFrameRate',2);
CREATE TABLE process_measure (type, ts, dur, value, filter_id);
INSERT INTO process_measure VALUES
 ('process_measure',900000000,300000000,120.0,10),
 ('process_measure',1100000000,300000000,120,10),
 ('process_measure',1300000000,200000000,60,10),
 ('process_measure',1500000000,100000000,NULL,10),
 ('process_measure',1700000000,100000000,'90',10),
 ('process_measure',1800000000,100000000,119.88,10),
 ('process_measure',2000000000,100000000,240,10),
 ('process_measure',1000000000,1000000000,90,20),
 ('process_measure',1000000000,1000000000,75,11),
 ('process_measure',NULL,100000000,123,10),
 ('process_measure',1650000000,0,0,10),
 ('process_measure',1950000000,NULL,60,10);
