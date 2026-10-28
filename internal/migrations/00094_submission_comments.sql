-- +goose Up
-- The Admin's one comment on a Learner's Submission to an Assignment (spec #196), shown to the Learner.
CREATE TABLE submission_comments (
    assignment_id bigint NOT NULL REFERENCES assignments ON DELETE CASCADE,
    learner_id bigint NOT NULL REFERENCES learners ON DELETE CASCADE,
    body text NOT NULL CHECK (body <> ''),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (assignment_id, learner_id)
);
CREATE INDEX submission_comments_learner ON submission_comments (learner_id);

-- +goose Down
DROP TABLE submission_comments;
