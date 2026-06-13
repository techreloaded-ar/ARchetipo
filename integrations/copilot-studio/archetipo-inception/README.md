# ARchetipo Inception per Microsoft Copilot Studio

Questo kit porta la skill `archetipo-inception` dentro Microsoft Copilot Studio come agente conversazionale per discovery prodotto e generazione PRD. La versione Copilot Studio non usa la CLI `archetipo`: legge e scrive i PRD in una cartella OneDrive personale tramite Power Automate.

La cartella OneDrive diventa il patrimonio storico delle inception: all'inizio l'agente legge i PRD gia presenti per trovare progetti simili, pattern ricorrenti, rischi gia emersi e domande utili; alla fine salva il nuovo PRD nella stessa cartella con un nome sempre diverso.

## Contenuto del kit

- `agent-instructions.md`: istruzioni complete da incollare nelle istruzioni dell'agente.
- `topic-inception.md`: struttura del topic principale e comportamento conversazionale atteso.
- `prd-template.md`: template Markdown del PRD generato.
- `actions/onedrive-prd-library.md`: specifica dei flow Power Automate per leggere e scrivere nella cartella OneDrive.
- `config.example.json`: esempio delle variabili da configurare.
- `import-checklist.md`: checklist di creazione, collegamento, test e pubblicazione.

## Prerequisiti

- Accesso a Microsoft Copilot Studio.
- Permessi per creare o modificare agenti nell'ambiente Power Platform scelto.
- Accesso a OneDrive for Business con una cartella personale dedicata ai PRD ARchetipo.
- Permessi per creare flow Power Automate richiamabili da Copilot Studio.

## Configurazione consigliata

Creare nell'agente, o come environment variables della solution, questi valori:

| Nome | Obbligatorio | Esempio | Note |
|---|---:|---|---|
| `ARCHETIPO_ONEDRIVE_FOLDER_PATH` | si | `/ARchetipo/PRD` | Cartella OneDrive personale da usare come libreria PRD. |
| `ARCHETIPO_PRD_FILE_PREFIX` | no | `PRD` | Prefisso dei nuovi file. Default: `PRD`. |
| `ARCHETIPO_MAX_REFERENCE_PRDS` | no | `5` | Numero massimo di PRD storici da usare come riferimento. Default: `5`. |

Il nome file del nuovo PRD deve essere generato a ogni salvataggio e non deve mai essere riusato. Formato consigliato:

```text
{ARCHETIPO_PRD_FILE_PREFIX}-{YYYYMMDD-HHmm}-{product-slug}.md
```

Esempio:

```text
PRD-20260611-1435-portale-clienti.md
```

Se il nome esiste gia, il flow deve aggiungere un suffisso incrementale:

```text
PRD-20260611-1435-portale-clienti-2.md
```

## Setup in Copilot Studio

1. Creare un nuovo agente chiamato `ARchetipo Inception`.
2. Incollare il contenuto di `agent-instructions.md` nelle istruzioni dell'agente.
3. Creare un topic principale usando `topic-inception.md` come guida.
4. Creare i flow Power Automate descritti in `actions/onedrive-prd-library.md`.
5. Aggiungere i flow all'agente come tool/action con nomi `ListReferencePrdsFromOneDrive` e `SavePrdToOneDrive`.
6. Configurare le variabili OneDrive indicate sopra.
7. Testare una conversazione completa e verificare che l'agente legga PRD precedenti e salvi il nuovo PRD in OneDrive.
8. Pubblicare l'agente o aggiungerlo a una solution Power Platform per import/export tra ambienti.

## Lettura del patrimonio PRD

All'inizio di ogni inception l'agente chiama `ListReferencePrdsFromOneDrive` e usa i PRD restituiti per:

- riconoscere progetti simili o gia affrontati;
- evitare domande gia risolte in PRD precedenti;
- fare domande piu mirate;
- confrontare scope, personas, architettura, rischi e requisiti;
- segnalare analogie utili all'utente senza copiare ciecamente decisioni passate.

Se la cartella e vuota o il flow fallisce, l'agente procede comunque e lo dichiara in modo leggero.

## Contratto di lettura

Input:

```json
{
  "folderPath": "/ARchetipo/PRD",
  "maxFiles": 5,
  "query": "portale clienti B2B"
}
```

Output atteso:

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

## Contratto di salvataggio

L'agente chiama `SavePrdToOneDrive` solo dopo avere generato il PRD completo.

Input:

```json
{
  "folderPath": "/ARchetipo/PRD",
  "filePrefix": "PRD",
  "productName": "Portale Clienti",
  "contentMarkdown": "# Portale Clienti - Product Requirements Document\n..."
}
```

Output atteso:

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

Se `ok` e `false`, l'agente deve spiegare l'errore in modo operativo e offrire all'utente il contenuto PRD nella chat.
