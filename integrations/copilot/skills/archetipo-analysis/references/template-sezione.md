# Section template — one screen or one sub-process

Use this template for **every** step of the process map, in the order of the map. One step, one section, always with all ten blocks: a block that does not apply says so explicitly ("Nessuna integrazione chiamata in questo step"), it is never omitted. The last section of the document must be as dense as the first.

> **Language:** English scaffolding. Translate every static element (headings, table headers, bold labels) into the detected language, per the **Language policy** in the agent instructions. Keep `{{PLACEHOLDER}}` tokens unchanged. Do not translate the conventions `XX crt` and `[DA CONFERMARE]`: they are markers the reader searches for.

## Conventions used inside a section

| Convention | Meaning | When to use it |
|---|---|---|
| `XX crt` | Length not known | The field length is not in the Knowledge, not in the PRD and not given by the user. Never guess a number. |
| `[DA CONFERMARE]` | Assumption to be validated | Any value, rule, message or outcome you had to assume. Put the marker right after the assumed value and repeat the item in block 9. |
| `«…»` | Verbatim text | Messages, popup texts, labels and button captions exactly as they appear or will appear. If the text is not known, write a proposed text followed by `[DA CONFERMARE]`. |
| **Bloccante** / **Forzabile** | Control severity | *Bloccante*: the user cannot continue. *Forzabile*: the user may continue after an explicit action (reason, flag, authorisation); say which one and what it produces. |
| **Fonte** | Where a value comes from | Name the system or table (Anagrafe, NDG, Rapporti, Tabella `{{TABLE}}`), or "inserimento manuale". |

## Template

```markdown
### {{SECTION_NUMBER}} {{STEP_NAME}}

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

#### {{SECTION_NUMBER}}.8 Copertura RF

| RF | Come lo copre questa sezione |
|---|---|
| {{RF_ID}} | {{ONE_LINE_EXPLANATION}} |

#### {{SECTION_NUMBER}}.9 Punti aperti e assunzioni

| N | Punto | Assunzione adottata | Impatto se diversa |
|---|---|---|---|
| PA-{{SECTION_NUMBER}}-{{N}} | {{OPEN_POINT}} `[DA CONFERMARE]` | {{ASSUMPTION}} | {{IMPACT}} |

Every `[DA CONFERMARE]` in blocks 1–7 appears here. Write "Nessun punto aperto" if none.

#### {{SECTION_NUMBER}}.10 Screen spec

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

For a screen (Tipo = SCREEN) block 2 is a list of image lines, one per state listed in block 10, in that order: base first. `{{SECTION_DASHED}}` is the section number with dashes (`3.4` → `3-4`). The images are produced by `scripts/render_screens.py` from `screens/{{SECTION_DASHED}}.json`, written from block 10 as described in `screen-spec.md`. For a sub-process or a control (no screen) block 2 is one line: "Nessuna videata: {{WHAT_THE_STEP_IS}}".

## How to fill it

1. **Retrieve first, write second.** Before filling a section, search the Knowledge for the as-is documentation of the step ("as-is {{STEP_NAME}} campi", "as-is {{STEP_NAME}} controlli", "as-is {{STEP_NAME}} messaggi"). Fill cells from what comes back and from the PRD and the user's answers. Nothing else.
2. **Cells, not prose.** A section is mostly tables. Keep prose to one line per bullet. A reader must find "what is the length of the fiscal code" by scanning a column, not by reading a paragraph.
3. **Unknown is a value.** `XX crt`, `[DA CONFERMARE]`, "Nessun popup", "Nessuna integrazione": every cell is filled, and an honest gap is a legitimate content.
4. **Verbatim or proposed.** A message is either the text found in the Knowledge, or a proposed text marked `[DA CONFERMARE]`. Never leave the message column empty.
5. **Same density everywhere.** The section for the last step of the map has the same tables, filled with the same care, as the first. If you are running out of budget, say so in the final response and name the sections to regenerate; do not thin them silently.
