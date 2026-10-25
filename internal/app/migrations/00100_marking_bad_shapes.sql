-- +goose Up
-- marking_bad_shapes counts the runs in which the AI's mark for one of the attempt's Written answers still came back
-- in the wrong shape after its second ask. After app.maxBadShapes the AI is asked no more about the attempt and its
-- unmarked answers wait for the Admin.
ALTER TABLE exam_attempts ADD COLUMN marking_bad_shapes int NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE exam_attempts DROP COLUMN marking_bad_shapes;
