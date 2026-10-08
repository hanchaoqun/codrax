-- Offline fixture construction only; keep outside the model-visible fixture.
-- These are constructed data, not a recording from a device.
DELETE FROM native_hook WHERE id IN (2, 3, 4, 5);
UPDATE process SET name = 'ImageEditor' WHERE ipid = 1;
UPDATE thread SET name = 'editor-main' WHERE itid = 1;
UPDATE data_dict SET data = 'DecodeCache::reserve' WHERE id = 101;
UPDATE data_dict SET data = 'Preview::render' WHERE id = 102;
UPDATE data_dict SET data = '/app/libeditor.so' WHERE id = 201;
