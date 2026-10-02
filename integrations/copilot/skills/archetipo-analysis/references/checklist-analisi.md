# Functional analysis — self-check checklist (run before delivering the document)

Applies to: every `Analisi-Funzionale.docx` produced by this skill, at first generation and at every revision.
Keywords: checklist, quality gate, copertura, densità, anti-invenzione, controllo, verifica, consegna.

## How to use this checklist

Run it in the same turn that generates the document, after writing the draft and before producing the Word file and applying **Deliver an artifact**. Go through every group. Each item is pass, fail or n/a. Fix every fail by regenerating the sections involved, then re-run the group. Report the result as one compact line in the final response, e.g. "A. Copertura 3/3 · B. Campi 3/3 · C. Controlli 2/2 · D. Integrazioni 2/2 · E. Pulsanti 1/1 · F. Anti-invenzione 3/3 · G. Densità 3/3 · H. Lingua 2/2 · I. Word 3/3 · J. Mockup 4/4". A document with any fail is not delivered.

## A. Coverage of the PRD

1. Every RF of the PRD appears in the process map (chapter 2) in at least one row.
2. Every RF of the PRD appears in block 8 of at least one section of chapter 3, and in the coverage matrix (chapter 6) with the same section numbers.
3. The RF numbering is the PRD's own; no RF was renumbered, merged or dropped.

## B. Fields

1. Every field row has a value in **every** one of the seven columns. An unknown length is `XX crt`; an unknown source is `[DA CONFERMARE]`; a field without controls says "Nessun controllo".
2. Every field has an explicit mandatory status: obbligatorio, opzionale, condizionale (with the condition) or sola lettura.
3. Every prefilled field names its source system or table.

## C. Controls

1. Every control (block 5, and the control column of block 3) has a message in «…».
2. Every control has a severity, **Bloccante** or **Forzabile**; every forceable control says how it is forced and what trace it leaves.

## D. Integrations

1. Every integration named in the PRD has a row in chapter 4, and every integration in chapter 4 is called by at least one section, or says "Nessuno [DA CONFERMARE]".
2. Every integration table (block 6) has a row for every outcome the service can return, including "Non disponibile / timeout", each with a behaviour and a message.

## E. Buttons and actions

1. Every button of every screen has an enabling condition, an effect with the next step, and either a popup with verbatim text or "Nessun popup".

## F. Anti-invention

1. No numeric length, code, table name or message text appears that is not traceable to the Knowledge, the PRD or a user's answer. Everything else carries `[DA CONFERMARE]` or is `XX crt`.
2. Every `[DA CONFERMARE]` in blocks 1–7 of a section is repeated in block 9 of that section and in chapter 5.
3. Questions the user skipped during the clarification phase appear in chapter 5 as assumptions; none was silently resolved.

## G. Uniform density

1. The **last** section of chapter 3 has all ten blocks (nine if it is not a screen), with the field table, the controls table and the integrations table filled — compare it with the first section: same structure, same care.
2. No section replaced a table with a summary sentence ("i campi sono quelli dell'anagrafe", "controlli standard"). Every step has its own rows.
3. Every section that is a screen has a **Screen spec** block with fields, buttons, error states, banners and navigation.

## H. Language and terminology

1. Headings, table headers and labels are in the language of the PRD; `XX crt` and `[DA CONFERMARE]` are left untranslated.
2. Names of steps, fields, states and actors are the ones the PRD uses. A term the PRD does not use is introduced once, in chapter 1, with its meaning.

## I. Word rendering (from the `build_docx.py` report)

1. The report ends with `RESULT: OK`: `residual markdown markers: 0` and `images missing: 0`. A `CHECK FAILED` is fixed in the Markdown or in the screen specs and the script is run again; the document is never delivered with raw `**`, `` ` `` or `#` in it.
2. The heading counts of the report match the map: one Heading 2 per step of chapter 2, nine or ten Heading 3 per step, Heading 1 for the six chapters plus «Descrizione intervento».
3. The table count of the report is at least six per step (blocks 3, 4, 5, 6, 8, 9) plus the header, history, map, integrations, open points and coverage tables — no table of the templates was flattened into prose.

## J. Mockup images (from the `render_screens.py` report)

1. Every step whose Tipo is Videata has a `screens/<section>.json` and a `-base.png`; the report lists no `ERROR`.
2. Every state listed in «Stati da renderizzare» of block 10 has its image, and block 2 references exactly those images, base first.
3. The images show the same content as the section: field labels of block 3 in the same order, buttons of block 4, error messages «…» of blocks 3 and 5 in the error state, banner texts of block 10. No field, button or message exists only in the image.
4. Data in the images is realistic Italian banking data marked as examples; no lorem ipsum.
