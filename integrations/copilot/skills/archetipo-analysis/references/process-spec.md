# Process spec — the JSON of the swimlane (chapter 5)

One JSON file per analysis, `processo.json`, written in phase 3 after chapters 3 and 4, from which `render_process.py` draws `screens/swimlane-processo.png`: the swimlane view of chapter 5. You never draw: you describe, the script lays out and draws.

The figure uses the customer's swimlane notation (BPMN with a swimlane approach). Its elements are the ones below and no others:

| Element | What it represents | In the JSON |
|---|---|---|
| **Blocco funzionale** | A phase of the process: the chevron bar at the top, one column per phase | `phases[]` |
| **Utente** | The user who owns the functional step on the application: a yellow tag at the start of the lane. Ringed in orange when the user is in the first FREE release | `lanes[]` with `kind: "utente"`, `primo_rilascio` |
| **Applicazione** | The application where the step happens. Green: web application. Grey: 3270 application. Orange: 3270 application whose webisation is already planned. Light blue: application that is part of the process but is not queried by the user (only information passes between applications). A dashed border marks a step that is not mandatory | `nodes[]` with `kind: "applicazione"`, `tecnologia`, `opzionale` |
| **Freccia continua** | A passage between applications that are already integrated | `flows[]`, default |
| **Freccia tratteggiata** | A passage between applications that are not integrated: the user makes it by hand | `flows[]` with `integrato: false` |
| **Notifica via mail** | A mail sent by the process. Green: to a user. Yellow: to a pool of resources | `nodes[]` with `kind: "notifica"`, `destinatario` |
| **ID evento** | The unique identifier of the event, as in column A of the events file | `flows[].evento` |
| **Trigger evento** | The functional trigger that sends the event, as in column D of the events file | `flows[].label` |
| **Decisione** | A branch of the process (type of request, outcome of a check) | `nodes[]` with `kind: "decisione"` |
| **Avvio / Chiusura processo** | Where the process starts, and its closure OK or KO | `nodes[]` with `kind: "avvio"` or `kind: "chiusura"` and `esito` |
| **Gestione con modale** | The step is handled in a modal: an M badge on the application | `nodes[].modale` |
| **Intervento evolutivo** | An evolution planned or in progress on that application: a gear badge | `nodes[].evolutivo` |

The colour of an arrow adds one reading to the customer's notation: teal when the process moves forward, green when it goes back to a previous phase, orange when it is cancelled or closes early. The legend of the figure lists only the elements it uses.

## Rules

- **Derived, not invented.** Everything in the figure is already in the document. Phases are the macro-processes of chapter 2 (or the practice states, when the process is governed by them). Lanes are the users of block 1 («Chi può eseguirlo»); a lane `kind: "sistemi"` holds the applications no user works in. Applications are those of the sections and of chapter 4, with the activity of the step as a short `attivita`. Flows are the transitions the sections describe (button effects, integration outcomes, cancellations, expiry).
- **Facts from the Knowledge only.** `tecnologia`, `integrato`, `evento`, `primo_rilascio`, `modale` and `evolutivo` say something about the customer's systems: set them only when the Knowledge, the PRD or the user say so. A technology nobody stated is `da-definire`; an event ID nobody stated is left out (the trigger stays). `integrato` defaults to true: write `false` only for a passage the sources describe as manual.
- **The main path, not every path.** The figure is for reading the process, the sections are for the detail. Leave out field-level errors that keep the user on the same screen; keep the branches that change phase, change application, go back or end the process.
- **Readable on one landscape page.** At most 6 phases, 5 lanes, 12 nodes and 18 flows; a single lane fits about 7 nodes. An application label is its name (one line), its `attivita` at most about 4 words; a trigger at most 4 words; a phase label at most 5 words. If the process is bigger, draw the main path and say in the text of chapter 5 which branches are described only in the sections.
- **Same words as the document.** Phase labels are the macro-process names of chapter 2; application names and user names are those of the PRD and the Knowledge.

## Format

```json
{
  "id": "processo",
  "title": "Estinzione anticipata finanziamento",
  "subtitle": "Vista di processo: fasi, utenti e applicativi",
  "phases": [
    {"id": "richiesta", "label": "Richiesta", "note": "L'operatore avvia l'estinzione dalla scheda del rapporto; FREE apre la pratica."},
    {"id": "conteggio", "label": "Conteggio estintivo", "note": "Il conteggio viene calcolato e confermato dall'operatore."}
  ],
  "lanes": [
    {"id": "rete", "label": "Utente Rete", "kind": "utente", "primo_rilascio": true},
    {"id": "sistemi", "label": "Sistemi", "kind": "sistemi"}
  ],
  "nodes": [
    {"id": "avvio", "lane": "rete", "phase": "richiesta", "kind": "avvio", "label": "Avvio processo"},
    {"id": "free", "lane": "rete", "phase": "richiesta", "label": "FREE", "tecnologia": "web", "attivita": "Apre la pratica"},
    {"id": "mdweb", "lane": "rete", "phase": "conteggio", "label": "MDWEB", "tecnologia": "web", "attivita": "Verifica il conteggio", "modale": true},
    {"id": "calcolo", "lane": "sistemi", "phase": "conteggio", "label": "MD00", "tecnologia": "sistema", "attivita": "Calcola il debito"},
    {"id": "ko", "lane": "rete", "phase": "conteggio", "kind": "chiusura", "esito": "ko"}
  ],
  "flows": [
    {"from": "avvio", "to": "free", "evento": "1"},
    {"from": "free", "to": "mdweb", "label": "Avvio conteggio", "evento": "2"},
    {"from": "mdweb", "to": "calcolo", "label": "Richiesta conteggio"},
    {"from": "mdweb", "to": "ko", "label": "Annullo", "kind": "annullo", "route": "sotto"}
  ]
}
```

`python <skill>/scripts/render_process.py --example` prints a complete example that uses every element: three lanes, the four application colours, a decision, a mail to a pool, a non-integrated passage, a return to a previous phase, both closures and the badges.

| Key | Meaning | Values and notes |
|---|---|---|
| `id` | Names the output file `swimlane-<id>.png` | Always `processo`, so the image is `screens/swimlane-processo.png` |
| `title`, `subtitle` | Header of the figure | The process name; the subtitle says what the view shows |
| `phases[]` | Blocchi funzionali, left to right | `id`, `label`, optional `note`: one or two sentences drawn in a box under the column, saying what happens in the phase |
| `lanes[]` | Rows, top to bottom | `id`, `label`, `kind`: `utente` (default) or `sistemi`; `primo_rilascio: true` rings the tag in orange |
| `nodes[]` | Elements in a lane and a phase | `id`, `lane`, `phase`, `label`, `kind`: `applicazione` (default), `decisione`, `notifica`, `avvio`, `chiusura`. Optional `slot` (0, 1, …) orders several nodes of the same lane and phase left to right; without it they follow the list order |
| `applicazione` | | `label` = application name; `tecnologia`: `web`, `3270`, `3270-webizzazione`, `sistema`, `da-definire`; optional `attivita`, `opzionale`, `modale`, `evolutivo` |
| `notifica` | | `destinatario`: `utente` or `pool`; `label` = what the mail says, in a few words |
| `chiusura` | | `esito`: `ok` or `ko`; `label` defaults to «Chiusura processo OK/KO» |
| `flows[]` | Arrows | `from`, `to`, optional `label` (trigger evento), `evento` (ID evento), `kind`: `avanzamento` (default), `ricircolo`, `annullo`; `integrato` (default true); `route: "sotto"` sends the arrow through the corridor below the lanes involved — use it for cancellations, so they stay out of the main path |
| `legend` | Optional | Overrides a legend text, by key: `avanzamento`, `ricircolo`, `annullo`, `integrato`, `non-integrato`, `web`, `3270`, `3270-webizzazione`, `sistema`, `da-definire`, `opzionale`, `mail-utente`, `mail-pool`, `evento`, `trigger`, `modale`, `evolutivo`, `primo-rilascio` |

## How the script lays it out

Each node takes a slot in its phase column; a phase is as wide as its busiest lane. Arrows run only in the gaps between slots and in the corridors between lanes, so they never cross a node; parallel arrows get separate tracks. An arrow going back loops through the corridor above its source. Trigger and event ID are placed along their own arrow where they hide the fewest other lines. You control the result with the order of lanes, the phase of each node, `slot` and `route` — never with coordinates.

## The report

`render_process.py processo.json --out screens` prints one line per figure (size, counts, the estimated size of the smallest text on a landscape A4 page) and ends with `RESULT: OK` or `RESULT: CHECK FAILED`.

- `FAIL … does not fit`: an application name, activity, lane or phase label is too long. Shorten it and run again.
- `ERROR SpecError`: a lane, phase or node id that does not exist, two nodes in the same slot, an unknown kind or technology. Fix the JSON.
- `WARN … overlaps`: a flow label could not find a free place. Shorten the trigger, move a node to another slot, or use `route: "sotto"` for a cancellation.
- `WARN smallest text ≈ N pt`: the figure is too wide to read in Word. Merge nodes, reduce the phases, or draw the main path only.

A warning does not block the delivery, but a figure with overlapping labels is not delivered: fix it or say in the final response why it stays.
