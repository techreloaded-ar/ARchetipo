---
type: context-map
title: Mappa dei contesti di ARchetipo
description: Confini runtime osservabili e dipendenze tra skill, CLI, connector, provider, viewer e distribuzione
status: reviewed
sources:
    - path: cli/internal/cli/root.go
      role: runtime-composition
    - path: cli/internal/connector/connector.go
      role: persistence-contract
    - path: cli/internal/execution/provider.go
      role: execution-contract
    - path: cli/internal/web/server.go
      role: viewer-runtime
    - path: npm/archetipo/bin/archetipo.js
      role: distribution-entrypoint
    - path: cli/internal/execution/arcipelago/client.go
      role: execution-contract
    - path: cli/internal/execution/arcipelago/provider.go
      role: implementation
    - path: cli/internal/template/template.go
      role: process-definition
    - path: cli/internal/cli/init_project_cmd.go
      role: workspace-bootstrap
    - path: cli/internal/cli/spec_cmd.go
      role: process-actions
review:
    content_hash: sha256:2200af0eccc16f77a7f5b47879c398fcbb41714e3dacd2746f7568d37dccfc45
    evidence_revision: aff2c2f75381020af04298015fb37c51fc7eddfc
    evidence_hash: sha256:fd80fe7241d20317eeb90ee20e530b9b7b9a876e36f1e083c41beeb7f15b5cfc
    reviewed_at: "2026-08-16T11:33:43Z"
---
# Mappa dei contesti di ARchetipo

<!-- archetipo:wiki section=contexts -->
## Contesti osservati

- **Workflow dell'agente.** Le skill orchestrano le fasi di prodotto e invocano operazioni esplicite della CLI; i loro helper assemblano payload deterministici senza diventare un runtime applicativo separato.
- **Template di processo.** Quali skill un workspace possiede, quali stati usa il suo workflow e quali azioni sono ammesse in ciascuno stato non sono piu impliciti in una costante della CLI: sono dichiarati da un Template risolto per identificativo all'inizializzazione. Ogni azione porta un identificativo stabile su cui un programma fa match, un'etichetta destinata a chi legge, la skill che la realizza e gli stati che la ammettono. Il package `template` e la sorgente unica di quella definizione; il workspace ne conserva identificativo e versione.
- **CLI e workflow persistente.** `cli/internal/cli/root.go` compone i comandi pubblici e instrada le operazioni verso configurazione, validazione, Wiki, worktree e connector.
- **Connector.** `connector.Connector` definisce lettura e scrittura di PRD, backlog, spec, piani, task e stato del workflow. Le implementazioni file, GitHub, Jira e in-memory condividono questo contratto.
- **Esecuzione tramite provider.** Il package `execution` seleziona un provider per ID e capability; per `plan` richiede `spec.plan` prima del dispatch e persiste l'outcome separatamente dagli artefatti del connector.
- **Esecuzione remota su ARcipelago.** La CLI ARchetipo agisce da client machine-to-machine dell'hub ARcipelago: crea un task esterno, ne attende l'esito e, quando serve, lo ritrova dal riferimento esterno che già possiede. È il primo confine runtime uscente verso un sistema esterno.
- **Viewer locale.** Il server web espone board, metriche, spec, piani, review, configurazione e mockup tramite HTTP e usa il connector come backend.
- **Distribuzione e verifica.** Lo shim npm seleziona il binario nativo; gli script sincronizzano skill, runtime e pacchetti di piattaforma; l'harness E2E costruisce la CLI e verifica scenari in sandbox.

La [panoramica](/overview.md) definisce il perimetro e la [mappa del codice](/engineering/code-map.md) collega questi contesti ai file.

<!-- archetipo:wiki section=relationships -->
## Relazioni

Le skill dipendono dalla superficie pubblica della CLI e dal suo envelope JSON. La CLI carica configurazione e connector per leggere o modificare il source of truth del workflow. Il viewer dipende dallo stesso connector, ma aggiunge un'interfaccia HTTP locale e asset incorporati.

All'inizializzazione la CLI risolve il Template prima di scrivere qualunque cosa, installa le skill che quel Template dichiara e ne conserva identificativo e versione nella configurazione del workspace; le skill rileggono quella selezione dall'envelope di `config show`, dove leggono gia ogni altro metadato di workspace. Un workspace privo del blocco risolve comunque il Template predefinito. Le alternative valutate e i limiti accettati sono nel [Template di processo del workspace](/decisions/workspace-process-template.md).

Oltre alla selezione, un client puo chiedere alla CLI quali azioni sono disponibili per una singola spec: `archetipo spec actions US-XXX` restituisce l'envelope `spec_actions` con il codice e lo stato della spec, l'identificativo e la versione del Template risolto, e le azioni ammesse in quello stato. È il confine che rende il processo consumabile da un programma: il client sceglie un'azione per identificativo stabile e mostra l'etichetta, mentre la skill che la realizza arriva insieme all'azione e non deve essere nota in anticipo. L'elenco non è persistito da nessuna parte: viene ricalcolato a ogni richiesta leggendo lo stato corrente della spec dal connector e filtrando le azioni dichiarate dal Template, e uno stato senza azioni ammesse produce una lista vuota, non un errore. La versione riportata accanto alle azioni è quella del Template risolto in-process, non la coppia persistita nella configurazione del workspace, che resta esposta da `config show`.

Il servizio di esecuzione riceve dalla CLI una spec gia letta e un registry di provider, ma non riceve `connector.Connector`: il provider non puo invocare transizioni di stato della spec. La verifica fail-closed della capability `spec.plan`, il singolo record durevole e gli outcome `SUCCEEDED` o `FAILED` sono fissati nella [decisione sul confine dei provider](/decisions/execution-provider-boundary.md).

### Dipendenza esterna: hub ARcipelago

| Direzione | Rotte | Autenticazione | Vincolo di confine |
|---|---|---|---|
| Uscente, unidirezionale (ARcipelago non chiama ARchetipo) | `POST /api/external/tasks` (creazione), `GET /api/external/tasks/{id}` (attesa dell'esito), `GET /api/external/tasks/by-reference?workspaceId&source&externalId` (recupero del task dal riferimento esterno, quando l'identificativo locale è andato perso o l'attesa è scaduta) | Bearer applicativo il cui **nome di variabile d'ambiente** è configurato; il valore non è mai persistito nella configurazione né in un artefatto del repository | Il workspace ARcipelago usato per la pianificazione remota deve ammettere soltanto runner la cui **prima** working directory è il checkout di ARchetipo, e quel checkout deve condividere lo stato del connector configurato |

La relazione fra i due contesti è una divisione di proprietà, non una duplicazione: ARchetipo possiede backlog, piani e stato del workflow; ARcipelago possiede l'esecuzione degli agenti. Il piano viene scritto nel connector configurato dal lato che esegue e non viene trasferito fra i due, come stabilito dalla [proprietà remota della scrittura del piano](/decisions/remote-plan-ownership.md).

Il vincolo sulla prima working directory non è un dettaglio di configurazione ma la condizione che rende vero il confine descritto. L'API esterna accetta soltanto `workspaceId`, `source`, `externalId`, `title`, `prompt` e `metadata`, e non accetta `cwdHint`, `skills`, `assigneeAgentId` né `targetRunnerId`; il dispatcher risolve la working directory come `task.cwdHint ?? runner.system.workdirs[0]`. Poiché un task esterno non può trasportare `cwdHint`, la directory effettiva è sempre la prima working directory del runner scelto: un task esterno non ha alcun modo di dirigere sé stesso.

Il packaging npm dipende dagli artefatti GoReleaser e dalle skill sorgente. Lo shim delega poi al binario specifico per sistema operativo e architettura. L'harness di test dipende dal binario costruito dal sorgente e crea sandbox isolate per esercitare CLI, skill e Wiki.

<!-- archetipo:wiki section=shared -->
## Infrastruttura condivisa

La configurazione `.archetipo/config.yaml` — che porta anche il blocco `template:` con identificativo e versione del processo installato — il formato di envelope JSON, i tipi di dominio, le regole del runtime condiviso e il filesystem Git sono infrastruttura attraversata da piu flussi. Gli script Node.js supportano assemblaggio, packaging e test; non possiedono dati di dominio autonomi. I comandi e i vincoli sono raccolti in [sviluppo e operazioni](/operations/development.md).

<!-- archetipo:wiki section=uncertainties -->
## Confini non ancora risolti

Questa mappa descrive dipendenze osservabili e non assegna nomi DDD specializzati alle relazioni, perche il codice non documenta una governance semantica sufficiente. Le skill, la CLI, i connector, il viewer e il packaging sono confini tecnici verificati, ma questa prima mappa non li promuove automaticamente a bounded context. L'ispezione di `cli` e `test` e campionata; cambiamenti futuri possono richiedere una mappa piu granulare.
