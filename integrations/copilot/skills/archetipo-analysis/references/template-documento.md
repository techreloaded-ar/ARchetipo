# Document template — `Analisi-Funzionale.docx`

The whole functional analysis is one Word document, regenerated in full at every revision. The template below is written in Markdown because it is the grammar of the content: each `#` level is a Word heading level, each Markdown table is a Word table with the same columns, and the text in between is a Word paragraph. Nothing in the template is delivered as Markdown.

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
| **Documenti di riferimento** | `PRD.md` v{{PRD_VERSION}} del {{PRD_DATE}}{{OTHER_REFERENCE_DOCUMENTS}} |

### Cronistoria

| Versione | Data | Autore | Modifiche |
|---|---|---|---|
| {{VERSION}} | {{DATE}} | ARchetipo | {{CHANGE_SUMMARY}} |

### Indice

1. Obiettivi del documento
2. Mappa di processo
3. Descrizione intervento
   {{ONE_LINE_PER_STEP: 3.N Step name}}
4. Integrazioni
5. Punti aperti
6. Matrice di copertura RF

---

## 1 Obiettivi del documento

{{TWO_TO_FOUR_LINES: what the intervention does, for whom, from which PRD, what this document adds to it (screen-by-screen, field-by-field detail), and what it does not cover.}}

**Perimetro:** {{IN_SCOPE_FROM_PRD_MVP}}
**Fuori perimetro:** {{OUT_OF_SCOPE_FROM_PRD}}
**Convenzioni:** `XX crt` = lunghezza da definire; `[DA CONFERMARE]` = assunzione da validare; «testo» = testo verbatim; **Bloccante** / **Forzabile** = severità del controllo.

---

## 2 Mappa di processo

{{ONE_LINE_PER_MACRO_PROCESS: the macro-processes in order, as a numbered list.}}

| N | Step | Tipo | Macro-processo | RF coperti | Punti aperti |
|---|---|---|---|---|---|
| {{N}} | {{STEP_NAME}} | {{Videata | Sotto-processo | Controllo}} | {{MACRO_PROCESS}} | {{RF_LIST}} | {{OPEN_POINTS_COUNT}} |

Every RF of the PRD appears in at least one row. The table is the resume point of the document: a revision starts by re-reading it.

**Stati della pratica:** {{STATE_LIST_WITH_ONE_LINE_EACH — only if the PRD or the Knowledge define states; otherwise "Non definiti nel PRD [DA CONFERMARE]"}}

---

## 3 Descrizione intervento

{{ONE_SECTION_PER_STEP, in the order of the map, each built with `template-sezione.md`, numbered 3.1, 3.2, …}}

---

## 4 Integrazioni

Riepilogo per servizio di tutte le integrazioni citate nelle sezioni del capitolo 3.

| Servizio | Sistema | Chiamato negli step | Input | Esiti gestiti | Timeout / non disponibile |
|---|---|---|---|---|---|
| {{SERVICE}} | {{SYSTEM}} | {{STEP_NUMBERS}} | {{INPUT_SUMMARY}} | {{OUTCOME_LIST}} | {{TIMEOUT_BEHAVIOUR}} |

Every integration named in the PRD (section *Integrations*) has a row here, even when no step calls it yet: in that case the "Chiamato negli step" cell says "Nessuno [DA CONFERMARE]".

---

## 5 Punti aperti

Elenco consolidato dei `[DA CONFERMARE]` di tutto il documento, ordinato per impatto.

| N | Sezione | Punto | Assunzione adottata | Impatto se diversa | Chi decide |
|---|---|---|---|---|---|
| {{PA_ID}} | {{SECTION_NUMBER}} | {{OPEN_POINT}} | {{ASSUMPTION}} | {{IMPACT}} | {{OWNER_OR_UNKNOWN}} |

Order: first what changes structure or flow, then field rules, then texts. This is the list the user answers at the next revision.

---

## 6 Matrice di copertura RF

| RF | Testo del requisito (dal PRD) | Sezioni che lo coprono | Stato |
|---|---|---|---|
| {{RF_ID}} | {{RF_TEXT_FROM_PRD}} | {{SECTION_NUMBERS}} | {{Coperto | Coperto con assunzioni | Da approfondire}} |

Every RF of the PRD is a row, in PRD order, with the PRD's own numbering preserved. "Da approfondire" is allowed only if block 9 of the covering sections says why.

---

_Analisi funzionale generata da ARchetipo — {{DATE}}_
_Generata a partire da `PRD.md` v{{PRD_VERSION}}; ogni revisione rigenera il documento intero._
```

## Rules for the whole document

- **One file, regenerated whole.** There is no patching. A revision rewrites `Analisi-Funzionale.docx` from the first line to the last, starting from the version the user attached.
- **The map is the spine.** Chapter 3 follows the order of chapter 2, one section per row, same names, same numbering. Chapters 4, 5 and 6 are derived from chapter 3: nothing appears in them that is not in a section, and nothing in a section is missing from them.
- **RF numbering is the PRD's.** Never renumber. If the user attaches a renumbered PRD, rebuild the map and say so.
- **Header first, always.** The header table, the change history and the index are present from version 1.0: they are what makes the document reviewable by the customer.
