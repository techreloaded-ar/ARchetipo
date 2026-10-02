# Screen spec — the JSON that `scripts/render_screens.py` turns into FREE mockup images

One JSON file per screen of the process map, written in phase 3 from the section's Screen spec block, saved as `screens/<section>.json` (for example `screens/3-4.json`). The script draws the base state and every entry of `states` as a PNG, 1600 px wide, with the FREE shell (teal header, 72 px menu rail, 12-column grid, pill buttons, floating labels). You never draw: you describe, the script draws.

Keywords: screen spec, mockup, videata, stati, JSON, render, immagine, figura.

## Rules

- **Same content as the section.** Field labels are the field names of block 3, in the same order; buttons are those of block 4; the error state shows the «…» messages of blocks 3 and 5 verbatim; banners use the texts of the Screen spec. Nothing appears in the image that is not in the section.
- **Base state always.** Add a state only when block 10 lists it: `errore`, `caricamento`, `vuoto`, `modale-<nome>`. One state, one image.
- **Realistic Italian banking data**, never lorem ipsum: names, NDG, IBAN masked, amounts with thousands separator and comma decimals. Mark them as examples in the subtitle («Dati esemplificativi»).
- **File names**: the section number with dashes, then the state: `3-4-base.png`, `3-4-errore.png`. These are the names the Markdown references in block 2.
- **Width**: a widget is `"cols": 6` (half row) or `12` (full row). Fields flow three per row in a 12-column widget, two per row in a 6-column one; `"span"` makes a field wider.

## Format

```json
{
  "section": "3.4",
  "title": "Inserimento dati",
  "subtitle": "Mario Rossi · NDG 00451287 · Dati esemplificativi",
  "app": "Scrivania Digitale",
  "user": "Stefano Marello",
  "menu": [
    {"label": "Home", "icon": "home"},
    {"label": "Clienti", "icon": "user"},
    {"label": "Pratiche", "icon": "doc", "active": true},
    {"label": "Cassette", "icon": "box"}
  ],
  "context_tab": {"title": "Apertura contratto", "subtitle": "Cassetta di sicurezza"},
  "breadcrumb": ["Home", "Processi", "Apertura contratto"],
  "back": null,
  "header_actions": [],
  "tabs": [],
  "stepper": {"steps": ["Controlli", "Dati contratto", "Condizioni", "Riepilogo", "Esito"], "current": 2},
  "banner": {"type": "info", "text": "La cassetta selezionata resta prenotata fino al 31/10/2026."},
  "widgets": [
    {
      "type": "form", "title": "Intestatari e operatività", "cols": 6,
      "fields": [
        {"label": "Primo intestatario", "value": "Mario Rossi · NDG 00451287", "readonly": true, "span": 2},
        {"label": "Operatività", "value": "Disgiunta", "control": "select"},
        {"label": "Conto di addebito", "value": "IT60 X054 2811 1010 •••• 4567", "helper": "Conto intestato a Mario Rossi", "span": 3}
      ],
      "actions": ["Aggiungi cointestatario", "Aggiungi delegati"]
    },
    {
      "type": "table", "title": "Delegati", "cols": 12,
      "columns": ["Nome", "Codice fiscale", "Ruolo", "Stato"],
      "rows": [
        ["Anna Bianchi", "BNCNNA80A41G337K", "Delegato", {"status": "success", "label": "Attivo"}],
        ["Luca Verdi", "VRDLCU75M12G337Z", "Delegato", {"status": "warning", "label": "Da verificare"}]
      ],
      "row_actions": 2, "toolbar": {"search": "Cerca delegato", "action": "Aggiungi"}, "paginator": 3
    }
  ],
  "actions": {"back": "Indietro", "secondary": ["Sospendi"], "primary": "Avanti"},
  "states": {
    "errore": {
      "banner": {"type": "error", "text": "Sono presenti errori nei dati inseriti. Correggi i campi evidenziati."},
      "field_errors": {"Conto di addebito": "Codice IBAN formalmente errato. Verificare il valore inserito."}
    },
    "caricamento": {"loading": {"text": "Verifica della cassetta in corso…", "widget": "Cassetta"}},
    "vuoto": {"empty": {"widget": "Delegati", "title": "Nessun delegato", "text": "Aggiungi un delegato per consentire l'accesso alla cassetta.", "action": "Aggiungi delegato"}},
    "modale-sospensione": {"modal": {"title": "Confermi la sospensione?", "text": "La pratica resterà sospesa per 30 giorni.", "primary": "Conferma", "secondary": "Annulla"}}
  }
}
```

## Keys

| Key | What it draws | Notes |
|---|---|---|
| `section` | nothing; names the files | `"3.4"` → `3-4-*.png` |
| `title`, `subtitle` | page header | title from block 10 «Titolo pagina» |
| `app`, `user` | header: application name and user area | defaults: «Scrivania Digitale», «Operatore» |
| `menu` | the 72 px rail; one item is `"active": true` | icons: `home`, `user`, `doc`, `box`, `list`, `search`, `calendar`, `euro`, `settings`; unknown names draw a dot |
| `context_tab` | the white process tab on the teal band | omit for pages outside a process |
| `breadcrumb` | path from the section root | last element is the current page |
| `back` | «← Indietro» link above the title for second-level and detail pages | string |
| `header_actions` | page-level buttons at the right of the title | `["Esporta", {"label": "Nuova pratica", "style": "primary"}]` |
| `tabs` | in-page tabs under the header | `["Dati", {"label": "Documenti", "active": true}]` |
| `stepper` | wizard progress, max 5 steps | `current` is 1-based |
| `banner` | one banner, full width | `type`: `info`, `warning`, `error`, `success` |
| `widgets[]` | the content grid | types below |
| `actions` | the CTA row under the content: `back` left, `secondary[]` and `primary` right | omit on pages without a flow |
| `states{}` | variants: each key becomes `<section>-<key>.png` | keys below |

### Widget types

| `type` | Keys | Draws |
|---|---|---|
| `form` | `title`, `cols`, `fields[]`, `actions[]`, `lines[]`, `per_row` | fields in rows; `actions` are secondary buttons under the fields; `lines` are short teal notes |
| `table` | `title`, `cols`, `columns[]`, `rows[][]`, `row_actions`, `toolbar`, `paginator`, `selected`, `total` | FREE table: teal uppercase header, 56 px rows, status cells as `{"status": "success|warning|error|info|neutral", "label": "…"}`, up to 3 inline icon actions |
| `kv` | `title`, `cols`, `items[[k, v]]` | key-value list (detail and summary pages) |
| `text` | `title`, `cols`, `lines[]` | plain paragraphs |
| `kpi` | `title`, `cols`, `value`, `label` | one number with its label |
| `chart` | `title`, `cols`, `series[]`, `labels[]`, `height` | simple bar chart |

### Field keys

`label` (required), `value`, `placeholder`, `helper`, `readonly`, `optional`, `span` (1–3), `control`: `text` (default), `select`, `date`, `toggle`, `radio`, `checkbox`. Radio and checkbox take `options[]` and `value` (the selected one). An `error` key on a field draws it in error state with the message under it; in practice set it through the `errore` state.

### State keys

| Key | Effect |
|---|---|
| `banner` | replaces the base banner |
| `field_errors` | `{ "<label>": "<messaggio>" }` — border and message in error color on those fields |
| `field_values` | `{ "<label>": "<valore>" }` — changes values |
| `loading` | `{"text": "…", "widget": "<title>"}` draws skeleton bars in that widget (all widgets if no `widget`) and a loader overlay with the text; `true` draws skeletons only |
| `empty` | `{"widget": "<title>", "title", "text", "action", "icon"}` replaces that widget's content with the FREE empty state |
| `modal` | `{"title", "text", "primary", "secondary", "size": "sm|lg|xl"}` draws the page under a 75 % backdrop with the modal on top |
| any other key | overrides the base key (`title`, `widgets`, `actions`, …) |

## Escape hatch

If a screen does not fit these components, keep the spec minimal (`section`, `title`, `breadcrumb`) and put a `"text"` widget with the lines that describe what the screen shows. Never leave a screen without its base image; never draw outside the script.
