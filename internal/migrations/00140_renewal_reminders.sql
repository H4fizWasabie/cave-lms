-- +goose Up
-- Renewal reminders (ADR 0026): one row for each reminder sent. A row is inserted before its email is sent, so a restart
-- can lose an email but never sends one twice. ends_at is the end the reminder was about; renewing moves the end, so the
-- next year gets its own reminders. item names what is ending: the Lab Plan, CaveText Pro (with Ebook access when its
-- year has it), or the Learner's Lab data, whose 90-day deletion the 'deletion' stage warns of. Deleting the Learner
-- deletes their rows.
CREATE TABLE renewal_reminders (
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    item text NOT NULL CHECK (item IN ('lab-plan', 'cavetext-pro', 'lab-data')),
    ends_at timestamptz NOT NULL,
    stage text NOT NULL CHECK (stage IN ('30-days', '7-days', 'ended', 'deletion')),
    sent_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (learner_id, item, ends_at, stage)
);

-- +goose Down
DROP TABLE renewal_reminders;
