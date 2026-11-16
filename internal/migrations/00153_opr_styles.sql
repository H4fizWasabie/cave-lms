-- +goose Up
-- The five OPR styles (see the glossary) a Teacher picks from when making an OPR prompt: slots 1 to 5, each a name and a prompt
-- text the Admin writes, and hidden when the Admin has not finished it. They start as placeholders, marked as such, based on the
-- layouts of the client's current OPR Image Prompt Builder, so that the OPR page works before the client writes its own.
CREATE TABLE opr_styles (
	position smallint PRIMARY KEY CHECK (position BETWEEN 1 AND 5),
	name text NOT NULL,
	prompt text NOT NULL,
	hidden boolean NOT NULL DEFAULT false
);
INSERT INTO opr_styles (position, name, prompt) VALUES
	(1, 'Activity (placeholder)', '[PLACEHOLDER, the client will rewrite this] Use a programme-report layout: hero title, two-column activity information table, objective, implementation summary, impact and reflection, follow-up actions, and two to four photo placeholders.'),
	(2, 'Achievement (placeholder)', '[PLACEHOLDER, the client will rewrite this] Use an achievement-report layout: large achievement title, event facts, objective, participation or result highlights, impact, improvement proposals, and a prominent vertical photo column.'),
	(3, 'Co-curricular (placeholder)', '[PLACEHOLDER, the client will rewrite this] Use a co-curricular activity layout: event profile with icon facts, objective, strengths, weaknesses, activity summary, improvement proposals, and a four-photo grid.'),
	(4, 'KPI dashboard (placeholder)', '[PLACEHOLDER, the client will rewrite this] Use a data-dashboard layout. Convert only verified numbers in the reference into KPI cards, tables, and bar, line or pie charts. Do not invent statistics. Include a compact executive summary and a data-source footer.'),
	(5, 'Inspection (placeholder)', '[PLACEHOLDER, the client will rewrite this] Use an inspection-report layout: profile table, clear assessment checklist or result table, findings, rating or conclusion, corrective action, and only verified charts or diagnostic bars.');

-- +goose Down
DROP TABLE opr_styles;
