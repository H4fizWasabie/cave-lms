-- +goose Up
-- A Learner's User master prompt: their own context, written by another AI chatbot and pasted into CaveText. NULL
-- when none is saved. It is not a Snippet and never counts toward the cap.
ALTER TABLE cavetext_accounts ADD COLUMN user_master_prompt text;

-- The question shown beside the empty User master prompt box, for the Learner to ask that chatbot. Any Admin edits it.
ALTER TABLE cavetext_settings ADD COLUMN master_prompt_question text NOT NULL DEFAULT 'Based on everything you know about me from our conversations, write a profile of me that I can give another AI chatbot as context. Cover who I am, my work or studies, my goals, the tools I use, and how I like answers written. Write it in the first person, as plain text, in under 1,000 words.';

-- +goose Down
ALTER TABLE cavetext_settings DROP COLUMN master_prompt_question;
ALTER TABLE cavetext_accounts DROP COLUMN user_master_prompt;
