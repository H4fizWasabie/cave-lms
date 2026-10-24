-- +goose Up
-- The Community Chat (spec #231) is the one Chat not tied to a Course: its Messages have no Course.
ALTER TABLE chat_messages ALTER COLUMN course_id DROP NOT NULL;
CREATE INDEX chat_messages_community ON chat_messages (id) WHERE course_id IS NULL;

-- Who has joined the Community Chat, one row per account. Leaving deletes the row; the Messages stay.
CREATE TABLE community_members (
    learner_id bigint PRIMARY KEY REFERENCES learners ON DELETE CASCADE,
    joined_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE community_members;
DROP INDEX chat_messages_community;
DELETE FROM chat_messages WHERE course_id IS NULL;
ALTER TABLE chat_messages ALTER COLUMN course_id SET NOT NULL;
