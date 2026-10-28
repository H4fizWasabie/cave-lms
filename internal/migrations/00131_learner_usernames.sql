-- +goose Up
-- A Username stands for a Learner in links (ADR 0023), so their name, printed on Certificates, stays out of them. Every
-- Learner from before Usernames gets a random one, learner- and 6 lowercase letters or digits, which they can change.
ALTER TABLE learners ADD COLUMN username text;
-- +goose StatementBegin
DO $$
DECLARE
    who record;
    picked text;
BEGIN
    FOR who IN SELECT id FROM learners LOOP
        LOOP
            SELECT 'learner-' || string_agg(substr('abcdefghijklmnopqrstuvwxyz0123456789', 1 + floor(random() * 36)::int, 1), '')
                INTO picked FROM generate_series(1, 6);
            EXIT WHEN NOT EXISTS (SELECT 1 FROM learners WHERE username = picked);
        END LOOP;
        UPDATE learners SET username = picked WHERE id = who.id;
    END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE learners ALTER COLUMN username SET NOT NULL;
CREATE UNIQUE INDEX learners_username_key ON learners (username);

-- +goose Down
DROP INDEX learners_username_key;
ALTER TABLE learners DROP COLUMN username;
