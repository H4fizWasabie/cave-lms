-- +goose Up
-- A Certificate No. is PCC-<year>-<number>: the year the Certificate was issued, in UTC as its date is shown, and its
-- place in that year. certificate_numbers holds each year's last number, so no number is given twice, even after its
-- Certificate is revoked or its Learner deleted. Certificates from before this are numbered by issued_at.
CREATE TABLE certificate_numbers (
    year int PRIMARY KEY,
    last int NOT NULL
);
ALTER TABLE certificates ADD COLUMN year int, ADD COLUMN number int;
UPDATE certificates c SET year = n.year, number = n.number
FROM (SELECT id, extract(year FROM issued_at AT TIME ZONE 'UTC')::int AS year,
        row_number() OVER (PARTITION BY extract(year FROM issued_at AT TIME ZONE 'UTC') ORDER BY issued_at, id) AS number
    FROM certificates) n
WHERE c.id = n.id;
INSERT INTO certificate_numbers (year, last) SELECT year, max(number) FROM certificates GROUP BY year;
ALTER TABLE certificates ALTER COLUMN year SET NOT NULL, ALTER COLUMN number SET NOT NULL,
    ADD CONSTRAINT certificates_number UNIQUE (year, number);

-- +goose Down
ALTER TABLE certificates DROP COLUMN number, DROP COLUMN year;
DROP TABLE certificate_numbers;
