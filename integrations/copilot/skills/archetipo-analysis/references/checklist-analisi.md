# Functional analysis — self-check checklist (run before delivering the document)

Applies to: every `Analisi-Funzionale_v<version>.docx` produced by this skill, at first generation and at every revision.
Keywords: checklist, quality gate, copertura, densità, anti-invenzione, punti aperti, swimlane, controllo, verifica, consegna.

## How to use this checklist

Run it in the same turn that generates the document, after writing the draft and before producing the Word file and applying **Deliver an artifact**. Go through every group. Each item is pass, fail or n/a. Fix every fail by regenerating the sections involved, then re-run the group. Report the result as one compact line in the final response, e.g. "A. Copertura 2/2 · B. Campi 3/3 · C. Controlli 2/2 · D. Integrazioni 2/2 · E. Pulsanti 1/1 · F. Anti-invenzione 2/2 · G. Densità 4/4 · H. Lingua 2/2 · I. Word 4/4 · J. Mockup 4/4 · K. Punti aperti 3/3 · L. Swimlane 4/4". A document with any fail is not delivered.

## A. Coverage of the PRD

1. Every functional requirement of the PRD is handled by at least one step of the map and described in its section (an internal check: the document does not show it).
2. The document cites no requirement code: no «RF» column in the map, no «RF coperti» line in the sections, no «RF-…» anywhere in the text.

## B. Fields

1. Every field row has a value in **every** one of the seven columns. An unknown length is `XX crt`; an unknown source is "da definire" or an open point; a field without controls says "Nessun controllo".
2. Every field has an explicit mandatory status: obbligatorio, opzionale, condizionale (with the condition) or sola lettura.
3. Every prefilled field names its source system or table.

## C. Controls

1. Every control (block 5, and the control column of block 3) has a message in «…».
2. Every control has a severity, **Bloccante** or **Forzabile**; every forceable control says how it is forced and what trace it leaves.

## D. Integrations

1. Every integration named in the PRD has a row in chapter 4, and every integration in chapter 4 is called by at least one section, or says "Nessuno".
2. Every integration table (block 6) has a row for every outcome the service can return, including "Non disponibile / timeout", each with a behaviour and a message.

## E. Buttons and actions

1. Every button of every screen has an enabling condition, an effect with the next step, and either a popup with verbatim text or "Nessun popup".

## F. Anti-invention

1. No numeric length, code, table name or event code appears that is not traceable to the Knowledge, the PRD or a user's answer. Everything else is `XX crt`, "da definire", a proposed message text, or an open point.
2. Questions the user skipped during the clarification phase were triaged: each became an open point or an adopted value written in its section; none was silently resolved in a way that contradicts the PRD.

## G. Uniform density

1. Every section opens with a functional description of two or three paragraphs (about 120–250 words) that covers purpose, context, what the user does, what the system does, the main alternatives and the result, as described in `template-sezione.md` — prose, not a list of fields, not a copy of the PRD text, with no codes, messages or markers. The last section's description is as full as the first's.
2. The **last** section of chapter 3 has the description and all nine blocks (eight if it is not a screen), with the field table, the controls table and the integrations table filled — compare it with the first section: same structure, same care.
3. No section replaced a table with a summary sentence ("i campi sono quelli dell'anagrafe", "controlli standard"). Every step has its own rows.
4. Every section that is a screen has a **Screen spec** block with fields, buttons, error states, banners and navigation.

## H. Language and terminology

1. Headings, table headers and labels are in the language of the PRD; `XX crt` and `[DA CONFERMARE]` are left untranslated.
2. Names of steps, fields, states and actors are the ones the PRD uses. A term the PRD does not use is introduced once, in chapter 1, with its meaning.

## I. Word rendering (from the `build_docx.py` report)

1. The report ends with `RESULT: OK`: `residual markdown markers: 0` and `images missing: 0`. A `CHECK FAILED` is fixed in the Markdown or in the specs and the script is run again; the document is never delivered with raw `**`, `` ` `` or `#` in it.
2. The heading counts of the report match the map: one Heading 2 per step of chapter 2, eight or nine Heading 3 per step, Heading 1 for the five chapters plus «Descrizione intervento».
3. The table count of the report is at least five per step (blocks 3, 4, 5, 6, 8) plus the header, history, map, integrations and events tables — no table of the templates was flattened into prose. The report shows one swimlane on landscape pages.
4. The `version:` line of the report equals the `Versione` row and the last row of *Cronistoria*; in a revision it is higher than the attached version (the script was run with `--previous`). The file attached is exactly the one on the `deliver:` line, and its name was not used before in this conversation.

## J. Mockup images (from the `render_screens.py` report)

1. Every step whose Tipo is Videata has a `screens/<section>.json` and a `-base.png`; the report lists no `ERROR`.
2. Every state listed in «Stati da renderizzare» of block 9 has its image, and block 2 references exactly those images, base first.
3. The images show the same content as the section: field labels of block 3 in the same order, buttons of block 4, error messages «…» of blocks 3 and 5 in the error state, banner texts of block 9. No field, button or message exists only in the image.
4. Data in the images is realistic Italian banking data marked as examples; no lorem ipsum.

## K. Open points

1. Every open point of block 8 has low confidence and medium or high impact, per the triage of `template-sezione.md`. Points about texts, labels, lengths or field order are not open points.
2. Every `[DA CONFERMARE]` of a section has its row in block 8 of that section or cites a point of an earlier step; every row of block 8 is referenced by at least one value of the section. The count per step in chapter 2 matches block 8.
3. Points that one answer would close are merged; no step has more than five open points without a reason stated in the final response. There is no chapter of open points.

## L. Swimlane (from the `render_process.py` report)

1. `processo.json` exists, the report ends with `RESULT: OK` and lists no `WARN … overlaps`; chapter 5 references `screens/swimlane-processo.png`.
2. The figure uses only the elements of the customer's notation listed in `process-spec.md`. The phases are the macro-processes of chapter 2, the lanes are the users named in the sections, the applications are those of the sections and of chapter 4, and every node corresponds to a step of chapter 3 or to a decision described there. Nothing exists only in the figure.
3. Every flow with a trigger or an event ID is a row of the *Eventi del processo* table, with the same texts; event IDs, application technologies and non-integrated passages appear only where the PRD, the Knowledge or the user state them (otherwise no ID, `da-definire`, integrated).
4. The smallest text reported is at least 6 pt, or the final response says why the figure stays dense.
