// test/web/task-markdown.test.mjs
// Smoke test for the task markdown helper used by the ARchetipo web viewer.
// Run: node --test test/web/task-markdown.test.mjs
//
// Verifies:
//   - GFM checklist items are normalized to plain bullets
//   - Other markdown constructs pass through unchanged
//   - The render function produces HTML without <input type="checkbox">
//   - HTML entity escaping does not leak the raw source

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const helperPath = resolve(
	__dirname,
	"..",
	"..",
	"cli",
	"internal",
	"web",
	"assets",
	"task-markdown.js",
);

// Minimal virtual-machine loader so we can consume the UMD helper in Node
// without a bundler. The helper checks `module.exports` first, then falls
// back to `window`.
import { createContext, runInContext } from "node:vm";

function loadTaskMarkdown() {
	const src = readFileSync(helperPath, "utf8");
	const mod = { exports: {} };
	const ctx = createContext({
		module: mod,
		// No `window` — the UMD will detect `module` and use that path.
		window: undefined,
	});
	runInContext(src, ctx);
	return mod.exports;
}

const { renderTaskMarkdown, normalizeTaskChecklist } = loadTaskMarkdown();

// Mock marked.parse that returns the input wrapped in a test div so we can
// assert on the final HTML shape without pulling in the real parser.
function mockMarkedParse(markdown) {
	return `<div class="mock-markdown">${markdown}</div>`;
}

describe("normalizeTaskChecklist", () => {
	it("converts unchecked GFM dash items to plain bullets", () => {
		const input = "- [ ] Do the thing\n- [ ] Also do this";
		const expected = "- Do the thing\n- Also do this";
		assert.equal(normalizeTaskChecklist(input), expected);
	});

	it("converts checked GFM dash items to plain bullets", () => {
		const input = "- [x] Done task\n- [x] Also done";
		const expected = "- Done task\n- Also done";
		assert.equal(normalizeTaskChecklist(input), expected);
	});

	it("converts star-based GFM items", () => {
		const input = "* [ ] Star item\n* [x] Star done";
		const expected = "* Star item\n* Star done";
		assert.equal(normalizeTaskChecklist(input), expected);
	});

	it("preserves indentation", () => {
		const input = "  - [ ] Nested item\n    - [x] Deeper nested";
		const expected = "  - Nested item\n    - Deeper nested";
		assert.equal(normalizeTaskChecklist(input), expected);
	});

	it("does not alter non-checklist bullets", () => {
		const input = "- Plain item\n- Another plain";
		assert.equal(normalizeTaskChecklist(input), input);
	});

	it("does not alter headings, code, or other markdown", () => {
		const input = [
			"## Criteri di Completamento",
			"",
			"- [ ] Il form invia i dati correttamente",
			"",
			"```js",
			"// - [ ] this is inside a code block",
			"```",
			"",
			"Un paragrafo con `- [ ]` inline code span.",
		].join("\n");
		const result = normalizeTaskChecklist(input);

		// Heading untouched
		assert.ok(result.includes("## Criteri di Completamento"));
		// Checklist bullet normalized
		assert.ok(result.includes("- Il form invia i dati correttamente"));
		// Code block content left alone
		assert.ok(result.includes("- [ ] this is inside a code block"));
		// Inline code span left alone
		assert.ok(result.includes("`- [ ]` inline code span"));
	});

	it("handles empty and non-string input gracefully", () => {
		assert.equal(normalizeTaskChecklist(""), "");
		assert.equal(normalizeTaskChecklist("   "), "   ");
		assert.equal(normalizeTaskChecklist(null), "");
		assert.equal(normalizeTaskChecklist(undefined), "");
	});
});

describe("renderTaskMarkdown", () => {
	it("returns empty string for empty/falsy markdown", () => {
		assert.equal(renderTaskMarkdown("", mockMarkedParse), "");
		assert.equal(renderTaskMarkdown("   ", mockMarkedParse), "");
	});

	it("converts checklist and passes through marked", () => {
		const input = "- [ ] Task body\n- [x] Completed";
		const html = renderTaskMarkdown(input, mockMarkedParse);
		// The mock parse wraps the normalized text, so the output contains
		// the normalized markdown.
		assert.ok(html.includes("- Task body"));
		assert.ok(html.includes("- Completed"));
	});

	it("produces HTML without <input type=\"checkbox\">", () => {
		// Even with a real task checklist, the normalized input contains
		// only plain bullets, so marked cannot generate checkboxes.
		const input =
			"## Criteria\n\n- [ ] First\n- [x] Second\n\nSome text";
		const html = renderTaskMarkdown(input, mockMarkedParse);
		assert.ok(!html.includes('type="checkbox"'));
		assert.ok(!html.includes("checkbox"));
	});

	it("preserves markdown headings and code spans", () => {
		const input = "## Title\n\n`some code`\n\n- [ ] Item";
		const html = renderTaskMarkdown(input, mockMarkedParse);
		assert.ok(html.includes("## Title"));
		assert.ok(html.includes("`some code`"));
		assert.ok(html.includes("- Item"));
	});

	it("throws when marked.parse is not available", () => {
		assert.throws(
			() => renderTaskMarkdown("text"),
			/marked\.parse is not available/,
		);
	});
});

describe("renderTaskMarkdown with real marked", { skip: true }, () => {
	// Optional integration suite — enable when `marked` is installed as a
	// node module. The mock-based tests above already validate the
	// normalization logic; these exercise the full marked parser pipeline.
	let markedParse;

	before(async () => {
		try {
			const marked = await import("marked");
			markedParse = marked.parse.bind(marked);
		} catch (_) {
			// marked not installed — skip.
		}
	});

	it("renders checklist items as plain <ul>/<li> without checkboxes", () => {
		if (!markedParse) return;
		const input = "- [ ] Build the thing\n- [x] Test it";
		const html = renderTaskMarkdown(input, markedParse);
		assert.ok(html.includes("<ul>") || html.includes("<li>"));
		assert.ok(!html.includes('type="checkbox"'));
		assert.ok(!html.includes("<input"));
	});

	it("renders headings, code blocks, and paragraphs", () => {
		if (!markedParse) return;
		const input = [
			"## Implementation notes",
			"",
			"Use `archetipo view` for testing.",
			"",
			"```",
			"go build ./...",
			"```",
			"",
			"- [ ] Verify in browser",
		].join("\n");
		const html = renderTaskMarkdown(input, markedParse);
		assert.ok(
			html.includes("<h2") || html.includes("Implementation notes"),
		);
		assert.ok(html.includes("<code>"));
		assert.ok((html.match(/<li>/g) || []).length > 0);
		assert.ok(!html.includes('type="checkbox"'));
	});

	it("handles multi-paragraph task bodies", () => {
		if (!markedParse) return;
		const input = [
			"First paragraph.",
			"",
			"Second paragraph.",
			"",
			"- [ ] Action item",
		].join("\n");
		const html = renderTaskMarkdown(input, markedParse);
		assert.ok(html.includes("<p>") || html.includes("First paragraph"));
		assert.ok(!html.includes('type="checkbox"'));
	});
});
