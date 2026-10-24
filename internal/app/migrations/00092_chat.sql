-- +goose Up
-- A Course's Chat is its Messages (spec #186). A Message goes with its writer's account, and with its Course.
-- hidden is set by the Admin, and the text kept so that the Admin can unhide it. deleted is set by the writer, and
-- the text is emptied then.
CREATE TABLE chat_messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    course_id bigint NOT NULL REFERENCES courses ON DELETE CASCADE,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    body text NOT NULL,
    -- The Message this one replies to. No foreign key: a reply outlives its original, whose quote then shows as
    -- removed.
    reply_to bigint,
    created_at timestamptz NOT NULL DEFAULT now(),
    hidden boolean NOT NULL DEFAULT false,
    deleted boolean NOT NULL DEFAULT false,
    CHECK (CASE WHEN deleted THEN body = '' ELSE btrim(body) <> '' AND char_length(body) <= 1000 END)
);
CREATE INDEX chat_messages_course ON chat_messages (course_id, id);
CREATE INDEX chat_messages_learner ON chat_messages (learner_id);
CREATE INDEX chat_messages_reply_to ON chat_messages (reply_to);

-- One of six fixed emoji, at most one per person per Message.
CREATE TABLE chat_reactions (
    message_id bigint NOT NULL REFERENCES chat_messages ON DELETE CASCADE,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    emoji text NOT NULL CHECK (emoji IN ('👍', '❤️', '😂', '😮', '🙏', '👏')),
    PRIMARY KEY (message_id, learner_id)
);
CREATE INDEX chat_reactions_learner ON chat_reactions (learner_id);

-- A Learner's Report of a Message, at most one per Message, open until the Admin hides the Message or dismisses it.
CREATE TABLE chat_reports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    message_id bigint NOT NULL REFERENCES chat_messages ON DELETE CASCADE,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz,
    UNIQUE (message_id, learner_id)
);
CREATE INDEX chat_reports_learner ON chat_reports (learner_id);

-- +goose Down
DROP TABLE chat_reports;
DROP TABLE chat_reactions;
DROP TABLE chat_messages;
