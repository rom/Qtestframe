-- Folder colours: '' or #rrggbb. Requirements and tests show the colour of
-- their folder, or of the nearest coloured parent folder.
ALTER TABLE folders ADD COLUMN color TEXT NOT NULL DEFAULT '';
