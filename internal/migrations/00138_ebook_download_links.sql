-- +goose Up
-- An Ebook download link (ADR 0020): one is made for each ebook when the Ebook email is sent, and its address carries a
-- random token of which only the hash is kept. It downloads the ebook with no sign-in until expires_at (7 days), and only
-- while the Learner's Ebook access runs. Deleting the Learner or the ebook deletes its links.
CREATE TABLE ebook_download_links (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    gallery_item_id bigint NOT NULL REFERENCES gallery_items ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ebook_download_links_learner ON ebook_download_links (learner_id);
CREATE INDEX ebook_download_links_gallery_item ON ebook_download_links (gallery_item_id);

-- +goose Down
DROP TABLE ebook_download_links;
