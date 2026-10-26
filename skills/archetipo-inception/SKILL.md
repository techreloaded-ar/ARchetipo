---
name: archetipo-inception
description: Create or revise a product document when someone explicitly wants to define product direction, people, goals and constraints.
---

# Product document

Read `.archetipo/shared-runtime.md` exactly once and use the person's language.
This is optional documentation work, never a prerequisite for trying an idea in
the app. Use the shared runtime's named personalities and intent discovery, one
question at a time with suggested answers. Apply the shared inception-style
brainstorming: Andrea challenges value and scope, Costanza uses what-if,
assumption challenge, audience flip or anti-problem techniques, and Livia explores
journeys and inclusion. Let relevant voices build on or challenge each other's
ideas; explore distinct directions before converging. Keep discovery proportionate
to the request.

Read `archetipo config show` (`kind: setup`) for configured paths, the existing
product document and relevant code/documentation. Describe whose problem matters,
what outcome is sought, current evidence, assumptions, constraints, priorities,
exclusions and open questions. Distinguish proposed behavior from implemented
behavior and technical completion from observed product value.

Explore the intended outcome, people, examples and material trade-offs before
drafting; reuse known answers and proceed with explicit implementation assumptions.
Preserve earlier decisions when revising an existing product document. Do not invent user interviews, metrics or research.

Use `references/prd-template.md` for the supported document structure, adapting
content to the actual product. Persist Markdown with `archetipo prd write --file -`.
It returns the standard `archetipo/v1`, `kind: write_result`, `data.path`.
Run `archetipo validate prd` (`kind: validation_result`) and inspect `data.ok` and
`data.findings`. Repair structural errors with at most three bounded attempts;
report remaining findings honestly. A structurally valid document is not business
approval. Do not overwrite a document in response to a request merely to discuss it.

Branch on `error.code`: `E_INVALID_INPUT` means correct the payload;
`E_PRECONDITION` means resolve the missing artifact/configuration; `E_CONFLICT`
means reread the current state. Never persist connector artifacts directly.

Conclude with a short explanation and, after persistence and validation, the
receipt line `{"artifact":"prd","status":"WRITTEN","path":"actual configured path"}`.
No backlog, implementation plan or Wiki refresh is required to complete this
operation. Use the dedicated optional skills only when those results are requested.
