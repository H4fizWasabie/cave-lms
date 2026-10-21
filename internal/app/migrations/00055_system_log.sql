-- +goose Up
-- The System log: errors and start-ups, kept 30 days. Only the Owner reads it.
CREATE TABLE system_log (
	id bigserial PRIMARY KEY,
	at timestamptz NOT NULL DEFAULT now(),
	level text NOT NULL CHECK (level IN ('error', 'info')),
	message text NOT NULL,
	method text NOT NULL DEFAULT '',
	path text NOT NULL DEFAULT ''
);
CREATE INDEX system_log_at ON system_log (at);

-- +goose Down
DROP TABLE system_log;
