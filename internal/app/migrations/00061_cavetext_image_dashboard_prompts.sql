-- +goose Up
-- The Image and Dashboard master prompts: written for the client, the same for every CaveText Pro Learner, in CaveText
-- snippet syntax so they may carry form fields. Any Admin edits their names, shortcuts and bodies. No Snippet may take
-- or clash with either shortcut.
ALTER TABLE cavetext_settings
	ADD COLUMN image_prompt_name text NOT NULL DEFAULT 'Image prompt',
	ADD COLUMN image_prompt_shortcut text NOT NULL DEFAULT '/image',
	ADD COLUMN image_prompt_body text NOT NULL DEFAULT 'Write a detailed prompt for an AI image generator, then generate the image.

Subject: {formtext: name=subject; default=a cup of teh tarik on a wooden table}
Style: {formmenu: photorealistic; digital illustration; watercolour; 3D render; flat vector; name=style}
Aspect ratio: {formmenu: 1:1 square; 16:9 landscape; 9:16 portrait; 4:5 social post; name=aspect ratio}
Mood and lighting: {formtext: name=mood and lighting; default=warm, soft morning light}
Leave out: {formtext: name=leave out; default=text, logos and watermarks}

Describe the subject, setting, composition, camera angle, colours and lighting in concrete detail. Keep one clear focal point and a clean background. Keep hands, faces and any text in the image correct.',
	ADD COLUMN dashboard_prompt_name text NOT NULL DEFAULT 'Dashboard prompt',
	ADD COLUMN dashboard_prompt_shortcut text NOT NULL DEFAULT '/dashboard',
	ADD COLUMN dashboard_prompt_body text NOT NULL DEFAULT 'Build an analytics dashboard from my data.

The data: {formparagraph: name=data description}
Audience: {formmenu: management; operations team; clients; teachers or students; name=audience}
Charts wanted: {formtext: name=charts wanted; default=a trend over time, a comparison by category and a breakdown of the total}
Questions it should answer: {formtext: name=questions; default=what changed, what is best and worst, and what needs action}

Start with 3 to 5 headline numbers (KPIs) with their change from the last period. Give each chart a clear title, labelled axes and a one-line takeaway. Use one consistent colour scheme, with colour only for meaning. Suggest filters for date range and category. If anything in the data is unclear, ask me before you build it.';

-- +goose Down
ALTER TABLE cavetext_settings
	DROP COLUMN dashboard_prompt_body, DROP COLUMN dashboard_prompt_shortcut, DROP COLUMN dashboard_prompt_name,
	DROP COLUMN image_prompt_body, DROP COLUMN image_prompt_shortcut, DROP COLUMN image_prompt_name;
