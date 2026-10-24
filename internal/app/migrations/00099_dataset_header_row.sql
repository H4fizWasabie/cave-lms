-- +goose Up
-- header_row is the Excel row Clean-up read the column names from, so the preview can ask "Is row N your
-- header?" when most of the columns a Work Dashboard's blocks use are missing (spec #237).
ALTER TABLE datasets ADD COLUMN header_row integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE datasets DROP COLUMN header_row;
