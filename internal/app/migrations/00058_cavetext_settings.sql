-- +goose Up
-- CaveText settings, one row. The first early_places places may keep early_cap Snippets free; later ones late_cap.
-- The Owner edits these three.
CREATE TABLE cavetext_settings (
	id smallint PRIMARY KEY CHECK (id = 1),
	early_places integer NOT NULL DEFAULT 1000 CHECK (early_places >= 0),
	early_cap integer NOT NULL DEFAULT 10 CHECK (early_cap >= 0),
	late_cap integer NOT NULL DEFAULT 5 CHECK (late_cap >= 0)
);
INSERT INTO cavetext_settings (id) VALUES (1);

-- +goose Down
DROP TABLE cavetext_settings;
