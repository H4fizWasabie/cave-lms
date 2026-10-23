-- +goose Up
-- A Promo is a lower price for one product between two dates (Malaysia), got with a code on that product's BCL
-- form. BCL holds the code and its limits; CAVELMS only accepts a Payment of at least price_sen while it runs.
-- ended_at is set when an Admin ends it early; ended Promos stay listed.
CREATE TABLE promos (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code text NOT NULL CHECK (code <> ''),
    product text NOT NULL REFERENCES prices (product),
    price_sen integer NOT NULL CHECK (price_sen > 0),
    starts_on date NOT NULL,
    ends_on date NOT NULL CHECK (ends_on >= starts_on),
    ended_at timestamptz
);
CREATE INDEX promos_product ON promos (product);

-- +goose Down
DROP TABLE promos;
