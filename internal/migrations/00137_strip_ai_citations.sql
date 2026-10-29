-- +goose Up
-- The AI writer's web search made the model append source links to the questions it wrote, and they were saved in the
-- Question bank and copied into Exams. They are removed from every prompt, option and model answer, with the space
-- before each. Only links carrying utm_source=openai go: an Admin may have written a real link of their own.
-- A link is "[text](url)", alone or wrapped in parentheses.
UPDATE bank_questions SET
    prompt = btrim(regexp_replace(prompt, '\s*\(\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)\)|\s*\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)', '', 'g'), E' \t\r\n'),
    model_answer = btrim(regexp_replace(model_answer, '\s*\(\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)\)|\s*\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)', '', 'g'), E' \t\r\n')
WHERE prompt LIKE '%utm_source=openai%' OR model_answer LIKE '%utm_source=openai%';

UPDATE exam_questions SET
    prompt = btrim(regexp_replace(prompt, '\s*\(\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)\)|\s*\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)', '', 'g'), E' \t\r\n'),
    model_answer = btrim(regexp_replace(model_answer, '\s*\(\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)\)|\s*\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)', '', 'g'), E' \t\r\n')
WHERE prompt LIKE '%utm_source=openai%' OR model_answer LIKE '%utm_source=openai%';

UPDATE bank_options SET
    text = btrim(regexp_replace(text, '\s*\(\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)\)|\s*\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)', '', 'g'), E' \t\r\n')
WHERE text LIKE '%utm_source=openai%';

UPDATE exam_options SET
    text = btrim(regexp_replace(text, '\s*\(\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)\)|\s*\[[^]]*\]\(https?://[^)\s]*utm_source=openai[^)\s]*\)', '', 'g'), E' \t\r\n')
WHERE text LIKE '%utm_source=openai%';

-- +goose Down
-- Nothing: the removed links are not kept.
SELECT 1;
