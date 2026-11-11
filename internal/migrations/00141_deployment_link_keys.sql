-- +goose Up
-- A Deployment's link carries a Link key after its name (ADR 0027): 10 random lowercase letters and digits, set when it is
-- deployed and replaced by the Learner's New secret link. A Deployment made before Link keys has none (NULL) and keeps its
-- keyless link until its Learner replaces it. Two Deployments can share a key; the name is what is unique.
ALTER TABLE deployments ADD COLUMN link_key text CHECK (link_key ~ '^[a-z0-9]{10}$');

-- +goose Down
ALTER TABLE deployments DROP COLUMN link_key;
