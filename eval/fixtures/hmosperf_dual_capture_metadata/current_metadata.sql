CREATE TABLE device_info (physical_width, physical_height, physical_frame_rate);
INSERT INTO device_info VALUES (1080, NULL, 60.0);
CREATE TABLE meta (name, value);
INSERT INTO meta VALUES ('collector', 'capture-kit-B');
INSERT INTO meta VALUES ('workload_cmd', 'open-document --set beta');
INSERT INTO meta VALUES ('device_name', 'board-B-lab');
INSERT INTO meta VALUES ('revision', 'ambiguous-one'), ('revision', 'ambiguous-two');
