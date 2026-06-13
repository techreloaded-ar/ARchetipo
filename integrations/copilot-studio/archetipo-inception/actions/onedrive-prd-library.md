# Actions Power Automate - OneDrive PRD Library

Queste action collegano l'agente Copilot Studio a una cartella OneDrive personale. La cartella funziona come libreria viva dei PRD ARchetipo: viene letta all'inizio di ogni inception e aggiornata alla fine con un nuovo file dal nome univoco.

Creare due flow Power Automate e aggiungerli all'agente come tool/action:

- `ListReferencePrdsFromOneDrive`
- `SavePrdToOneDrive`

## Action 1 - ListReferencePrdsFromOneDrive

### Obiettivo

Leggere i PRD esistenti in una cartella OneDrive personale e restituire all'agente un set limitato di documenti Markdown da usare come riferimento.

### Trigger

Usare il trigger Power Automate per chiamata da Copilot Studio.

Nome azione:

```text
ListReferencePrdsFromOneDrive
```

### Input

| Nome | Tipo | Obbligatorio | Descrizione |
|---|---|---:|---|
| `folderPath` | string | si | Cartella OneDrive personale, per esempio `/ARchetipo/PRD`. |
| `maxFiles` | integer | no | Numero massimo di file da restituire. Default consigliato: `5`. |
| `query` | string | no | Descrizione sintetica dell'idea prodotto, utile per filtrare o ordinare i riferimenti. |

Esempio:

```json
{
  "folderPath": "/ARchetipo/PRD",
  "maxFiles": 5,
  "query": "portale clienti B2B con workflow approvativi"
}
```

### Output

Restituire sempre questa forma:

```json
{
  "ok": true,
  "items": [
    {
      "fileName": "PRD-20260601-1015-crm-b2b.md",
      "filePath": "/ARchetipo/PRD/PRD-20260601-1015-crm-b2b.md",
      "webUrl": "https://...",
      "lastModified": "2026-06-01T10:15:00Z",
      "contentMarkdown": "# CRM B2B - Product Requirements Document\n..."
    }
  ],
  "errorCode": "",
  "message": "Reference PRDs loaded"
}
```

In errore:

```json
{
  "ok": false,
  "items": [],
  "errorCode": "ONEDRIVE_READ_FAILED",
  "message": "Unable to read reference PRDs from OneDrive."
}
```

### Logica del flow

1. Validare `folderPath`.
2. Leggere i file nella cartella OneDrive.
3. Filtrare file Markdown:
   - estensione `.md`;
   - preferire nomi che iniziano con il prefisso configurato, se disponibile;
   - escludere file temporanei o vuoti.
4. Ordinare per `lastModified` decrescente.
5. Se `query` e disponibile, dare priorita ai file il cui nome o contenuto contiene parole chiave compatibili con la query.
6. Limitare a `maxFiles`, default `5`.
7. Per ogni file selezionato, leggere il contenuto.
8. Restituire gli item all'agente.

### Azioni Power Automate suggerite

Una possibile implementazione usa il connettore OneDrive for Business:

- `List files in folder` o azione equivalente per enumerare la cartella.
- `Get file content using path` per leggere il Markdown.
- `Respond to Copilot` per restituire `ok`, `items`, `errorCode`, `message`.

Se la cartella non esiste, restituire `ok: false` con `errorCode: ONEDRIVE_FOLDER_NOT_FOUND`.

## Action 2 - SavePrdToOneDrive

### Obiettivo

Ricevere Markdown dall'agente, creare un nuovo file in OneDrive con nome univoco e restituire all'agente il link al documento. Questa action non deve sovrascrivere file esistenti.

### Trigger

Usare il trigger Power Automate per chiamata da Copilot Studio.

Nome azione:

```text
SavePrdToOneDrive
```

### Input

| Nome | Tipo | Obbligatorio | Descrizione |
|---|---|---:|---|
| `folderPath` | string | si | Cartella OneDrive personale, per esempio `/ARchetipo/PRD`. |
| `filePrefix` | string | no | Prefisso file. Default: `PRD`. |
| `productName` | string | si | Nome prodotto, usato per generare lo slug. |
| `contentMarkdown` | string | si | Contenuto Markdown completo del PRD. |

Esempio:

```json
{
  "folderPath": "/ARchetipo/PRD",
  "filePrefix": "PRD",
  "productName": "Portale Clienti",
  "contentMarkdown": "# Portale Clienti - Product Requirements Document\n..."
}
```

### Naming univoco

Il flow deve generare il nome file. Formato consigliato:

```text
{filePrefix}-{YYYYMMDD-HHmm}-{product-slug}.md
```

Regole:

- `filePrefix`: usare `PRD` se vuoto.
- timestamp: usare l'orario corrente del flow.
- `product-slug`: minuscolo, senza accenti, spazi convertiti in trattini, caratteri non alfanumerici rimossi.
- se il file esiste gia, aggiungere `-2`, poi `-3`, e cosi via fino a trovare un nome libero.
- non usare mai `overwrite`.

Esempi:

```text
PRD-20260611-1435-portale-clienti.md
PRD-20260611-1435-portale-clienti-2.md
```

### Output

Restituire sempre questa forma:

```json
{
  "ok": true,
  "webUrl": "https://...",
  "filePath": "/ARchetipo/PRD/PRD-20260611-1435-portale-clienti.md",
  "fileName": "PRD-20260611-1435-portale-clienti.md",
  "errorCode": "",
  "message": "File saved"
}
```

In errore:

```json
{
  "ok": false,
  "webUrl": "",
  "filePath": "",
  "fileName": "",
  "errorCode": "ONEDRIVE_WRITE_FAILED",
  "message": "Unable to create the PRD file in OneDrive."
}
```

### Logica del flow

1. Validare input:
   - `folderPath` non vuoto;
   - `productName` non vuoto;
   - `contentMarkdown` non vuoto.
2. Normalizzare `folderPath` rimuovendo slash finale.
3. Calcolare `filePrefix`, default `PRD`.
4. Calcolare timestamp nel formato `YYYYMMDD-HHmm`.
5. Generare `productSlug` da `productName`.
6. Comporre `fileName`.
7. Verificare se `{folderPath}/{fileName}` esiste gia.
8. Se esiste, provare suffissi incrementali fino a un nome libero.
9. Creare il file in OneDrive.
10. Recuperare o costruire `webUrl`.
11. Rispondere all'agente con `ok: true`.

### Azioni Power Automate suggerite

Una possibile implementazione usa il connettore OneDrive for Business:

- `Get file metadata using path` per verificare collisioni.
- `Create file` per creare il nuovo Markdown.
- `Get file metadata` o azione equivalente per ottenere il link web.
- `Respond to Copilot` per restituire l'oggetto di output.

## Codici errore

Usare codici stabili per permettere all'agente di reagire senza interpretare il messaggio:

| Codice | Quando usarlo |
|---|---|
| `INVALID_INPUT` | Uno o piu input obbligatori sono mancanti. |
| `ONEDRIVE_FOLDER_NOT_FOUND` | La cartella OneDrive non esiste. |
| `ONEDRIVE_PERMISSION_DENIED` | Il flow non ha permessi di lettura o scrittura. |
| `ONEDRIVE_READ_FAILED` | Lettura dei PRD storici fallita. |
| `ONEDRIVE_WRITE_FAILED` | Creazione del nuovo PRD fallita. |
| `UNIQUE_NAME_FAILED` | Impossibile trovare un nome file libero. |
| `UNKNOWN_ERROR` | Errore inatteso. |

## Comportamento agente in caso di errore

### Lettura fallita

Se `ListReferencePrdsFromOneDrive.ok` e `false`, l'agente deve:

1. procedere comunque con l'inception;
2. spiegare brevemente che non ha potuto usare i PRD storici;
3. non bloccare l'utente.

### Scrittura fallita

Se `SavePrdToOneDrive.ok` e `false`, l'agente deve:

1. spiegare il problema usando `message`;
2. mostrare `errorCode`;
3. indicare il path tentato se presente;
4. offrire il PRD completo nella chat cosi che il lavoro non venga perso.

## Casi di test

### Cartella con PRD esistenti

Input:

```json
{
  "folderPath": "/ARchetipo/PRD",
  "maxFiles": 5,
  "query": "CRM B2B"
}
```

Atteso:

- `ok: true`;
- `items` contiene al massimo 5 PRD;
- ogni item contiene `fileName`, `filePath`, `lastModified`, `contentMarkdown`.

### Cartella vuota

Atteso:

- `ok: true`;
- `items: []`;
- l'agente procede senza riferimenti storici.

### Creazione nuovo PRD

Input:

```json
{
  "folderPath": "/ARchetipo/PRD",
  "filePrefix": "PRD",
  "productName": "Portale Clienti",
  "contentMarkdown": "# Portale Clienti - Product Requirements Document\n..."
}
```

Atteso:

- file creato con nome simile a `PRD-20260611-1435-portale-clienti.md`;
- `ok: true`;
- `webUrl`, `filePath` e `fileName` valorizzati.

### Collisione nome

Precondizione: il nome generato esiste gia.

Atteso:

- il flow crea un file con suffisso incrementale;
- nessun file esistente viene sovrascritto.

### Cartella OneDrive non valida

Input con `folderPath` inesistente.

Atteso:

- `ok: false`;
- `errorCode: ONEDRIVE_FOLDER_NOT_FOUND` oppure `ONEDRIVE_WRITE_FAILED`;
- `message` operativo.
