# Topic principale - Inception prodotto

Questo topic descrive la conversazione principale dell'agente `ARchetipo Inception` in Copilot Studio.

## Trigger suggeriti

Usare frasi di attivazione come:

- voglio definire un prodotto
- ho un'idea di prodotto
- aiutami a creare un PRD
- documento di prodotto
- scope MVP
- product discovery
- product requirements document
- definire il prodotto
- nuova app
- nuovo software

## Variabili conversazionali

Tenere traccia, come variabili del topic o memoria conversazionale, di:

- `projectName`
- `productName`
- `targetSegment`
- `problem`
- `valueProposition`
- `differentiator`
- `referencePrds`
- `similarProjects`
- `reusablePatterns`
- `personas`
- `journeys`
- `mvpScope`
- `growthScope`
- `futureVision`
- `architecture`
- `functionalRequirements`
- `securityRequirements`
- `integrationRequirements`
- `challengedAssumptions`
- `assumptionsToValidate`
- `keyRisks`
- `openQuestions`
- `prdMarkdown`
- `oneDriveWebUrl`
- `oneDriveFileName`

## Flusso

### 1. Apertura

L'agente presenta il team ARchetipo, spiega che usera la cartella OneDrive personale come patrimonio storico, elenca le aree che verranno definite e chiede all'utente di raccontare l'idea.

Output atteso:

```text
Il team ARchetipo e qui per aiutarti a trasformare un'idea in una direzione di prodotto chiara, concreta e realizzabile.

Usero anche la tua libreria PRD in OneDrive per capire se esistono progetti simili e per fare domande piu mirate.

Con te oggi ci sono Andrea, Costanza, Leonardo, Livia ed Emanuele.

Lavoreremo insieme su visione, utenti, scope MVP, architettura e requisiti.

Raccontami l'idea che vuoi sviluppare.
```

### 2. Lettura PRD storici da OneDrive

Appena l'utente fornisce una prima descrizione dell'idea, chiamare `ListReferencePrdsFromOneDrive`.

Payload:

```json
{
  "folderPath": "{ARCHETIPO_ONEDRIVE_FOLDER_PATH}",
  "maxFiles": "{ARCHETIPO_MAX_REFERENCE_PRDS oppure 5}",
  "query": "{descrizione sintetica dell'idea}"
}
```

Se `ok` e `true`, salvare `items` in `referencePrds` e identificare:

- progetti simili;
- personas riusabili come ispirazione;
- pattern architetturali ricorrenti;
- rischi o assunzioni gia emersi;
- requisiti ricorrenti.

Se `ok` e `false` o `items` e vuoto, procedere senza bloccare la conversazione.

Messaggio sintetico quando sono presenti riferimenti:

```text
Ho trovato alcuni PRD nella tua libreria OneDrive. Li usero come riferimento per confrontare scope, rischi e architettura, ma ti chiedero conferma prima di riusare decisioni passate.
```

### 3. Discovery prodotto

Obiettivo: chiarire problema, target, valore, differenziatore, rischi e scope.

Agenti principali:

- Andrea;
- Costanza;
- Livia.

Domande da coprire senza trasformare la conversazione in un questionario rigido:

- quale problema risolve il prodotto;
- chi ha il problema;
- quale alternativa usa oggi il target;
- perche il prodotto sarebbe diverso o migliore;
- cosa assomiglia o differisce da PRD storici trovati in OneDrive;
- cosa rientra nel primo MVP;
- cosa resta fuori dal primo MVP;
- quali rischi potrebbero bloccare adozione o delivery.

Costanza deve usare almeno due tecniche di challenge prima di chiudere la discovery:

- What if;
- Assumption challenging;
- Audience flip;
- Anti-problem.

### 4. Personas e journey

Obiettivo: completare almeno una persona, preferibilmente due.

Per ogni persona raccogliere:

- ruolo;
- eta o fascia;
- background;
- obiettivi;
- pain point;
- comportamenti e strumenti;
- motivazioni;
- competenza tecnica;
- journey in cinque fasi: awareness, consideration, first use, regular use, advocacy.

Livia deve includere considerazioni di accessibilita e inclusione quando influenzano il prodotto.

Se un PRD storico suggerisce una persona simile, usarla come spunto e chiedere cosa cambia nel nuovo contesto.

### 5. Conferma critica

Quando l'agente ha inferito o riformulato punti importanti, chiedere una conferma breve:

```text
Prima di fissarlo nel PRD, confermo questa lettura:
- target primario: ...
- problema principale: ...
- differenziatore: ...
- MVP: ...
- rischio di adozione: ...
- riferimenti OneDrive utili: ...

Correggeresti qualcosa?
```

Se l'utente non risponde o preferisce procedere, mantenere i punti come assunzioni da validare.

### 6. Architettura tecnica

Obiettivo: proporre una direzione tecnica implementabile.

Agente principale:

- Leonardo.

Raccogliere:

- pattern architetturale;
- stack e versioni quando note;
- componenti principali;
- struttura del progetto;
- ambiente locale;
- strumenti richiesti;
- CI/CD;
- deployment;
- infrastruttura target;
- ADR principali.

Leonardo deve confrontare la proposta con eventuali PRD storici simili e distinguere:

- pattern riusabili;
- differenze necessarie;
- decisioni da validare.

Costanza deve sfidare la buildability:

- cosa non e ancora abbastanza specifico;
- quali convenzioni deve conoscere un implementatore;
- quali decisioni tecniche sono rischiose.

### 7. Requisiti

Obiettivo: ottenere almeno 10 requisiti funzionali, piu requisiti non funzionali essenziali.

Agenti principali:

- Andrea;
- Emanuele.

Formato consigliato:

```text
FR-001 - [Area]: [requisito verificabile]
FR-002 - [Area]: [requisito verificabile]
```

Le aree possono essere adattate al prodotto. Includere sicurezza e integrazioni quando rilevanti. Se i PRD storici mostrano requisiti ricorrenti, proporli come candidati e chiedere conferma prima di includerli.

### 8. Stato avanzamento

Ogni 3 o 4 round mostrare:

```text
Stato PRD:
- Completato: ...
- In corso: ...
- Riferimenti OneDrive usati: ...
- Mancante: ...
```

### 9. Generazione e salvataggio

Quando sono disponibili vision, almeno una persona completa, MVP, architettura e almeno 10 requisiti:

1. chiedere un ultimo follow-up solo se le risposte mancanti cambierebbero materialmente il PRD;
2. generare `prdMarkdown` usando `prd-template.md`;
3. chiamare `SavePrdToOneDrive`;
4. mostrare all'utente il link OneDrive, il nome file e il path se il salvataggio riesce.

Payload dell'azione:

```json
{
  "folderPath": "{ARCHETIPO_ONEDRIVE_FOLDER_PATH}",
  "filePrefix": "{ARCHETIPO_PRD_FILE_PREFIX oppure PRD}",
  "productName": "{productName}",
  "contentMarkdown": "{prdMarkdown}"
}
```

Gestione errore:

- se `ok` e `false`, comunicare il problema usando `message`;
- indicare `errorCode` se presente;
- indicare `filePath` tentato se presente;
- offrire il PRD completo nella chat per non perdere il lavoro.
