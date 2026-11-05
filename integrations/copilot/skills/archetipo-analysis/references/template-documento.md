# Document template — `Analisi-Funzionale_v<version>.docx`

The whole functional analysis is one Word document, regenerated in full at every revision under a new version. The `Versione` row of the header table is what `build_docx.py` reads to name the file: write it as `MAJOR.MINOR` only (`1.0`, `1.2`), with no prefix or suffix, and keep the row label as `Versione` (or `Version` in an English document). The template below is written in Markdown because it is the grammar of the content: each `#` level is a Word heading level, each Markdown table is a Word table with the same columns, and the text in between is a Word paragraph. Nothing in the template is delivered as Markdown.

> **Language:** English scaffolding. Translate every static element (headings, table headers, bold labels) into the detected language, per the **Language policy** in the agent instructions. Keep `{{PLACEHOLDER}}` tokens unchanged. Keep `XX crt` and `[DA CONFERMARE]` untranslated.

```markdown
# {{PRODUCT_NAME}} — Analisi funzionale

## Descrizione intervento

| | |
|---|---|
| **Prodotto** | {{PRODUCT_NAME}} |
| **Intervento** | {{INTERVENTION_TITLE}} (nuovo prodotto oppure evolutiva di {{EXISTING_FUNCTION}}) |
| **Autore** | ARchetipo — 🔎 Emanuele, Requirements Analyst |
| **Classificazione** | {{CLASSIFICATION}} |
| **Stato** | {{DRAFT | IN_REVIEW | APPROVED}} |
| **Versione** | {{VERSION}} |
| **Data creazione** | {{CREATION_DATE}} |
| **Data ultima modifica** | {{LAST_MODIFIED_DATE}} |
| **Documenti di riferimento** | `PRD_v{{PRD_VERSION}}.md` del {{PRD_DATE}}{{OTHER_REFERENCE_DOCUMENTS}} |

### Cronistoria

| Versione | Data | Autore | Modifiche |
|---|---|---|---|
| {{VERSION}} | {{DATE}} | ARchetipo | {{CHANGE_SUMMARY}} |

{{ONE_ROW_PER_VERSION: the rows of the attached version are kept as they were and the new version is added as the last row.}}

### Indice

1. Obiettivi del documento
2. Mappa di processo
3. Descrizione intervento
   {{ONE_LINE_PER_STEP: 3.N Step name}}
4. Integrazioni
5. Vista di processo

---

## 1 Obiettivi del documento

{{TWO_TO_FOUR_LINES: what the intervention does, for whom, from which PRD, what this document adds to it (screen-by-screen, field-by-field detail), and what it does not cover.}}

**Perimetro:** {{IN_SCOPE_FROM_PRD_MVP}}
**Fuori perimetro:** {{OUT_OF_SCOPE_FROM_PRD}}
**Convenzioni:** `XX crt` = lunghezza da definire; `[DA CONFERMARE]` = punto aperto rilevante, elencato nel blocco «Punti aperti» dello step; «testo» = testo verbatim; **Bloccante** / **Forzabile** = severità del controllo. I testi dei messaggi non presenti nella documentazione as-is sono proposte, da validare in revisione insieme al documento.

---

## 2 Mappa di processo

{{ONE_LINE_PER_MACRO_PROCESS: the macro-processes in order, as a numbered list.}}

| N | Step | Tipo | Macro-processo | Punti aperti |
|---|---|---|---|---|
| {{N}} | {{STEP_NAME}} | {{Videata | Sotto-processo | Controllo}} | {{MACRO_PROCESS}} | {{OPEN_POINTS_COUNT}} |

The table is the resume point of the document: a revision starts by re-reading it.

**Stati della pratica:** {{STATE_LIST_WITH_ONE_LINE_EACH — only if the PRD or the Knowledge define states; otherwise "Non definiti nel PRD"}}

---

## 3 Descrizione intervento

{{ONE_SECTION_PER_STEP, in the order of the map, each built with `template-sezione.md`, numbered 3.1, 3.2, …}}

---

## 4 Integrazioni

Riepilogo per servizio di tutte le integrazioni citate nelle sezioni del capitolo 3.

| Servizio | Sistema | Chiamato negli step | Input | Esiti gestiti | Timeout / non disponibile |
|---|---|---|---|---|---|
| {{SERVICE}} | {{SYSTEM}} | {{STEP_NUMBERS}} | {{INPUT_SUMMARY}} | {{OUTCOME_LIST}} | {{TIMEOUT_BEHAVIOUR}} |

Every integration named in the PRD (section *Integrations*) has a row here, even when no step calls it yet: in that case the "Chiamato negli step" cell says "Nessuno" and the step where it should be called carries an open point.

---

## 5 Vista di processo

La figura seguente rappresenta il processo {{PROCESS_NAME}} in una vista di tipo swimlane: i blocchi funzionali in colonna, gli utenti coinvolti in riga, gli applicativi su cui si svolgono le principali attività operative e le interazioni tra {{ORCHESTRATOR_OR_FRONT_END}} e gli applicativi impattati, con i trigger e gli identificativi degli eventi.

![Swimlane — {{PROCESS_NAME}}](screens/swimlane-processo.png)

{{ONE_TO_THREE_LINES: how to read the figure for this process — where it starts, the main branches, how it ends (completion, cancellation, expiry).}}

### Eventi del processo

| ID evento | Trigger evento | Da → A | Fase di origine → destinazione | Tipo | Passaggio |
|---|---|---|---|---|---|
| {{EVENT_ID_OR_DASH}} | {{TRIGGER}} | {{SOURCE_APPLICATION}} → {{TARGET_APPLICATION}} | {{SOURCE_PHASE}} → {{TARGET_PHASE}} | {{Avanzamento | Ritorno a fase precedente | Annullo}} | {{Integrato | Non integrato, a carico dell'utente}} |

One row per flow of the figure that carries a trigger or an event ID, in the order of the process, with the same texts as the figure. Event IDs appear only when the PRD or the Knowledge name them; otherwise the cell is "—".

---

_Analisi funzionale generata da ARchetipo — {{DATE}}_
_Versione {{VERSION}}, generata a partire da `PRD_v{{PRD_VERSION}}.md`; ogni revisione rigenera il documento intero con una nuova versione._
```

## Rules for the whole document

- **One file, regenerated whole, one version per delivery.** There is no patching. A revision rewrites the document from the first line to the last, starting from the version the user attached, and is delivered as the next version (`Analisi-Funzionale_v1.1.docx` → `Analisi-Funzionale_v1.2.docx`): `Versione` raised by one minor step, `Data ultima modifica` updated, one new row in *Cronistoria*.
- **The map is the spine.** Chapter 3 follows the order of chapter 2, one section per row, same names, same numbering. Chapters 4 and 5 are derived from chapter 3: nothing appears in them that is not in a section.
- **Open points live in their step.** There is no chapter of open points: each one is in block 8 of the step it concerns, and the count per step is in the last column of the map. Only the points that pass the triage of `template-sezione.md` are written.
- **The swimlane is drawn, not written.** Chapter 5 holds one image, `screens/swimlane-processo.png`, produced by `render_process.py` from `processo.json` (see `process-spec.md`). `build_docx.py` puts chapter 5 on landscape pages because the image name starts with `swimlane`; keep that name.
- **No requirement codes.** The customer does not use the PRD's functional requirement codes (RF): the document never cites them — no coverage column, no coverage line, no matrix, no «RF-…» in the text. The steps are named and described in business terms.
- **Header first, always.** The header table, the change history and the index are present from version 1.0: they are what makes the document reviewable by the customer.
