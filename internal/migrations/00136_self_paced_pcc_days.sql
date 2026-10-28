-- +goose Up
-- A self-paced Course may also print an Organisation's logo and a PCC Level badge on its Certificates. Its Level is
-- stored as the days that give it (1 to 5, as a Live Course's), NULL for none; a Live Course's Level still follows
-- from its days.
ALTER TABLE courses ADD COLUMN pcc_days integer CHECK (pcc_days BETWEEN 1 AND 5);

-- +goose Down
ALTER TABLE courses DROP COLUMN pcc_days;
