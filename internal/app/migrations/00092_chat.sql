-- +goose Up
-- A Course's Chat is its Messages (spec #186). A Message goes with its writer's account, and with its Course.
-- hidden is set by the Admin, and the text kept so that the Admin can unhide it. deleted is set by the writer, and
-- the text is emptied then.
CREATE TABLE chat_messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    course_id bigint NOT NULL REFERENCES courses ON DELETE CASCADE,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    hidden boolean NOT NULL DEFAULT false,
    deleted boolean NOT NULL DEFAULT false,
    CHECK (CASE WHEN deleted THEN body = '' ELSE btrim(body) <> '' AND char_length(body) <= 1000 END)
);
CREATE INDEX chat_messages_course ON chat_messages (course_id, id);
CREATE INDEX chat_messages_learner ON chat_messages (learner_id);

-- +goose Down
DROP TABLE chat_messages;
