ALTER TABLE embedded_items ADD COLUMN detail VARCHAR(50) NOT NULL DEFAULT 'migrated';
INSERT INTO embedded_items VALUES (2, 'second; embedded', 'added');
