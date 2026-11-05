# Section template — one screen or one sub-process

Use this template for **every** step of the process map, in the order of the map. One step, one section, always with the opening description and all nine blocks: a block that does not apply says so explicitly ("Nessuna integrazione chiamata in questo step"), it is never omitted. The last section of the document must be as dense as the first.

> **Language:** English scaffolding. Translate every static element (headings, table headers, bold labels) into the detected language, per the **Language policy** in the agent instructions. Keep `{{PLACEHOLDER}}` tokens unchanged. Do not translate the conventions `XX crt` and `[DA CONFERMARE]`: they are markers the reader searches for.

## Conventions used inside a section

| Convention | Meaning | When to use it |
|---|---|---|
| `XX crt` | Length not known | The field length is not in the Knowledge, not in the PRD and not given by the user. Never guess a number. It is not an open point. |
| `[DA CONFERMARE]` | Relevant open point | Only on a value that is an open point of block 8 (see **Which gaps become open points**). Put the marker right after the value. |
| `«…»` | Verbatim text | Messages, popup texts, labels and button captions exactly as they appear or will appear. A text not found in the Knowledge is a proposal; chapter 1 says so once for the whole document, so no marker is needed. |
| **Bloccante** / **Forzabile** | Control severity | *Bloccante*: the user cannot continue. *Forzabile*: the user may continue after an explicit action (reason, flag, authorisation); say which one and what it produces. |
| **Fonte** | Where a value comes from | Name the system or table (Anagrafe, NDG, Rapporti, Tabella `{{TABLE}}`), or "inserimento manuale". A table whose name is not known is "tabella da definire" — never an invented name. |

## Template

```markdown
### {{SECTION_NUMBER}} {{STEP_NAME}}

{{FUNCTIONAL_DESCRIPTION: three to five sentences of plain, readable prose that say what this step is for the business — what the user does here and why, what the system does on its own, where the step starts and what it leads to. No field lists, no codes, no table; written so that someone who reads only this paragraph understands the step.}}

**Tipo:** {{SCREEN | SUB_PROCESS | CONTROL}} · **RF coperti:** {{RF_LIST}} · **Mockup:** {{MOCKUP_REF}}

#### {{SECTION_NUMBER}}.1 Innesco e prerequisiti

- **Da dove si arriva:** {{ORIGIN}} (pulsante, voce di menu, esito di uno step precedente, evento)
- **Stato della pratica all'ingresso:** {{PRACTICE_STATE}}
- **Controlli già superati:** {{PASSED_CONTROLS}}
- **Chi può eseguirlo:** {{ACTOR_AND_PROFILE}}

#### {{SECTION_NUMBER}}.2 Mockup

![{{PAGE_TITLE}} — stato base (dati esemplificativi)](screens/{{SECTION_DASHED}}-base.png)
![{{PAGE_TITLE}} — stato {{STATE}} (dati esemplificativi)](screens/{{SECTION_DASHED}}-{{STATE}}.png)

{{MOCKUP_HTML_REF — only when a mockup HTML produced by the design work exists for this product: "Mockup HTML: `#{{SCREEN_ID}}`"; omit the line otherwise}}

#### {{SECTION_NUMBER}}.3 Campi

| Nome | Formato e lunghezza | Obbligatorietà | Prevalorizzazione e fonte | Dominio | Regole condizionali | Controlli e messaggio |
|---|---|---|---|---|---|---|
| {{FIELD_NAME}} | {{FORMAT}} {{LENGTH}} crt | {{MANDATORY | OPTIONAL | CONDITIONAL}} | {{PREFILL_VALUE}} da {{SOURCE}} | {{VALUES_OR_TABLE}} | {{CONDITIONAL_RULES}} | {{CONTROL}} → **{{BLOCKING | FORCEABLE}}** «{{MESSAGE}}» |

One row per field shown on the screen or handled by the sub-process, in reading order (top to bottom, left to right). Read-only fields are rows too, with "sola lettura" in the mandatory column. A field with no control says "Nessun controllo".

#### {{SECTION_NUMBER}}.4 Pulsanti e azioni

| Pulsante / azione | Quando è attivo | Cosa fa | Popup e testo verbatim |
|---|---|---|---|
| {{BUTTON_LABEL}} | {{ENABLING_CONDITION}} | {{EFFECT_AND_NEXT_STEP}} | «{{POPUP_TEXT}}» · pulsanti: {{POPUP_BUTTONS}} — oppure "Nessun popup" |

#### {{SECTION_NUMBER}}.5 Controlli automatici

| N | Controllo | Quando scatta | Esito | Severità | Messaggio | Effetti sul sistema |
|---|---|---|---|---|---|---|
| C{{N}} | {{CONTROL_DESCRIPTION}} | {{TRIGGER}} | {{KO_CONDITION}} | **{{BLOCKING | FORCEABLE}}** ({{HOW_TO_FORCE}}) | «{{MESSAGE}}» | {{FLAGS_NOTES_LOCKS}} |

Every control has a message and a severity. A forceable control says how it is forced (reason field, flag, second-level authorisation) and what trace it leaves (flag on the practice, note, event).

#### {{SECTION_NUMBER}}.6 Integrazioni chiamate

| Servizio | Quando viene chiamato | Input | Esito | Comportamento |
|---|---|---|---|---|
| {{SERVICE}} | {{WHEN}} | {{INPUT_FIELDS}} | Positivo | {{BEHAVIOUR_ON_OK}} |
| | | | Negativo ({{REASON}}) | {{BEHAVIOUR_ON_KO}} → «{{MESSAGE}}» |
| | | | Non disponibile / timeout | {{BEHAVIOUR_ON_TIMEOUT}} → «{{MESSAGE}}» |
| | | | {{OTHER_OUTCOME}} | {{BEHAVIOUR}} |

Every outcome the service can return has a row. "Non disponibile / timeout" is always present. Write "Nessuna integrazione chiamata in questo step" when there are none.

#### {{SECTION_NUMBER}}.7 Casi particolari

- {{EDGE_CASE}}: {{BEHAVIOUR}}

Concurrency, resume after suspension, data changed by another operator, cancelled practice, expired data, multiple co-holders, minors, foreign residents — whichever apply. Write "Nessun caso particolare individuato" if none.

#### {{SECTION_NUMBER}}.8 Punti aperti

| N | Punto | Assunzione adottata | Impatto se diversa | Chi decide |
|---|---|---|---|---|
| PA-{{SECTION_NUMBER}}-{{N}} | {{OPEN_POINT}} | {{ASSUMPTION}} | {{IMPACT}} | {{OWNER_OR_UNKNOWN}} |

Only the gaps that pass the triage below, ordered by impact. Every `[DA CONFERMARE]` in this section has its row here, or cites a point of an earlier step ("vedi PA-3.2-1"). Write "Nessun punto aperto" if none.

#### {{SECTION_NUMBER}}.9 Screen spec

_Only when Tipo = SCREEN. Omit the block otherwise._

- **Titolo pagina:** {{PAGE_TITLE}} · **Tipo pagina FREE:** {{FORM | WIZARD_STEP | LIST | DETAIL | MODAL}}
- **Campi, in ordine:** {{FIELD_LIST_WITH_WIDTHS}}
- **Pulsanti:** primario «{{PRIMARY_CTA}}» · secondari «{{SECONDARY_CTAS}}»
- **Stati di errore:** {{ERROR_STATES}} (per campo e banner di riepilogo)
- **Banner e feedback:** {{BANNERS}} (info, warning, errore, successo, con testo)
- **Stati asincroni:** caricamento durante {{LOADING_MOMENTS}}; errore servizio {{SERVICE_ERROR_STATE}}
- **Navigazione:** avanti verso {{NEXT_SCREEN}}, indietro verso {{PREVIOUS_SCREEN}}
- **Stati da renderizzare:** base{{, errore}}{{, caricamento}}{{, vuoto}}{{, modale-<nome>}} — only the states this screen actually has
```

For a screen (Tipo = SCREEN) block 2 is a list of image lines, one per state listed in block 9, in that order: base first. `{{SECTION_DASHED}}` is the section number with dashes (`3.4` → `3-4`). The images are produced by `scripts/render_screens.py` from `screens/{{SECTION_DASHED}}.json`, written from block 9 as described in `screen-spec.md`. For a sub-process or a control (no screen) block 2 is one line: "Nessuna videata: {{WHAT_THE_STEP_IS}}".

## Which gaps become open points

A gap is anything the Knowledge, the PRD and the user's answers do not settle. Every gap gets an adopted value; only a few become open points. Rate each gap on two axes:

| Axis | Low | Medium | High |
|---|---|---|---|
| **Confidence** in the adopted value | Nothing supports it, and plausible alternatives lead to a different behaviour | The PRD or the Knowledge point that way, or it is the house convention of similar steps | Stated by the PRD, the Knowledge or the user |
| **Impact** if the value is different | Message texts, labels, field order, lengths, a default changeable by configuration | A control's existence or severity, a mandatory rule, the behaviour on one outcome, who may press a button | The flow or the steps, who performs the step, an integration or its outcomes, a regulatory or accounting effect, data written on other systems |

- **Open point** (row in block 8, `[DA CONFERMARE]` next to the value): confidence **low** and impact **medium or high**.
- **Everything else** is written in the section as the adopted value, with no marker and no row: a length stays `XX crt`, a message text is a proposal (chapter 1 says so once), an unknown low-impact code or table is "da definire". Nothing is invented to fill the gap.
- **Merge** gaps that one answer would close (the timeout of every service, the same profile question on several buttons): one point, in the first step where it arises, cited by number in the others.
- **Keep it short.** A step normally has 0–3 open points. More than five means the step is under-specified: re-rate the impact honestly and merge; what survives is listed — never dropped to make the count look better.

## How to fill it

1. **Retrieve first, write second.** Before filling a section, search the Knowledge for the as-is documentation of the step ("as-is {{STEP_NAME}} campi", "as-is {{STEP_NAME}} controlli", "as-is {{STEP_NAME}} messaggi"). Fill cells from what comes back and from the PRD and the user's answers. Nothing else.
2. **Prose once, then cells.** The opening description is the only prose of the section: a few readable sentences on what the step is. After it, a section is mostly tables. Keep prose to one line per bullet. A reader must find "what is the length of the fiscal code" by scanning a column, not by reading a paragraph.
3. **Unknown is a value.** `XX crt`, "da definire", "Nessun popup", "Nessuna integrazione": every cell is filled, and an honest gap is a legitimate content. A gap becomes an open point only through the triage above.
4. **Verbatim or proposed.** A message is either the text found in the Knowledge, or a proposed text. Never leave the message column empty.
5. **Same density everywhere.** The section for the last step of the map has the same tables, filled with the same care, as the first. If you are running out of budget, say so in the final response and name the sections to regenerate; do not thin them silently.
