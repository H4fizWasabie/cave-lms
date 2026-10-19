#!/usr/bin/env node
// Abah's rule for Cave LMS: every change goes through Matt Pocock's skills (AGENTS.md "Agent skills").
//   prompt  - UserPromptSubmit: remind the agent of the route, including for router hand-offs.
//   gate    - PreToolUse on Write|Edit|MultiEdit|NotebookEdit: deny edits to repo files until the
//             transcript shows a Matt Pocock SKILL.md was read or a mattpocock-skills skill was invoked.
// Fails open on unreadable input so a broken hook never wedges the session.
const fs = require("node:fs");
const path = require("node:path");

const SKILLS = "~/.claude/plugins/cache/mattpocock/mattpocock-skills/<highest version>/skills/*/<name>/SKILL.md";
// A Read of a SKILL.md, a Bash read of one (cat/sed/head), or a Skill-tool invocation. A path merely quoted in a
// prompt or brief does not count.
const LOADED =
	/"file_path":\s*"[^"]*mattpocock-skills\/[^"]*SKILL\.md"|"command":\s*"[^"]*mattpocock-skills\/[^"]*SKILL\.md|"skill":\s*"mattpocock-skills:/;
const UNGATED = [".claude/", "graft/"];

const mode = process.argv[2];
let input = {};
try {
	input = JSON.parse(fs.readFileSync(0, "utf8"));
} catch {
	process.exit(0);
}

if (mode === "prompt") {
	process.stdout.write(
		`[Cave LMS workflow] Any change to this repo's code, tests, config or docs goes through Matt Pocock's skills. ` +
			`Before the first edit, read ask-matt (${SKILLS.replace("<name>", "ask-matt")}), pick the stage, then read and follow that ` +
			`stage's SKILL.md (small fix: tdd, or diagnosing-bugs for a bug, then code-review; multi-file: grill-with-docs first). ` +
			`State the skill you are following. This holds for router hand-offs too: the subagent brief must name the skill and its ` +
			`SKILL.md path and tell the subagent to Read it and follow it. Edits are blocked until a Matt Pocock SKILL.md has been read.\n`,
	);
	process.exit(0);
}

if (mode === "gate") {
	const root = process.env.CLAUDE_PROJECT_DIR || input.cwd || process.cwd();
	const file = input.tool_input?.file_path || input.tool_input?.notebook_path;
	if (!file) process.exit(0);
	const rel = path.relative(root, path.resolve(root, file));
	if (rel.startsWith("..") || path.isAbsolute(rel) || UNGATED.some((p) => rel.startsWith(p))) process.exit(0);

	// A subagent must load the skill itself: check its own transcript, not the parent's.
	const main = input.transcript_path;
	if (!main) process.exit(0);
	const transcript = input.agent_id
		? path.join(main.replace(/\.jsonl$/, ""), "subagents", `agent-${input.agent_id}.jsonl`)
		: main;
	let text;
	try {
		text = fs.readFileSync(transcript, "utf8");
	} catch {
		process.exit(0);
	}
	if (LOADED.test(text)) process.exit(0);

	process.stdout.write(
		JSON.stringify({
			hookSpecificOutput: {
				hookEventName: "PreToolUse",
				permissionDecision: "deny",
				permissionDecisionReason:
					`Cave LMS rule: route this change through Matt Pocock's skills first. Read ${SKILLS.replace("<name>", "ask-matt")} ` +
					`with the Read tool, pick the stage, Read that stage's SKILL.md and follow it, then retry this edit.`,
			},
		}),
	);
}
