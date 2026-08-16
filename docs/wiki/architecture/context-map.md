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
review:
    content_hash: sha256:c0d776f763a4d761686f9d6e20263f47533cc9791423320b580cdd44222351e6
    evidence_revision: 9971aeffca5b0e72438e465a01df7e07dd7459e4
    evidence_hash: sha256:eea75b41d1cd929c9ebf75ee461a85670f2f9cd40af75410756fbd3807bdfddf
    reviewed_at: "2026-08-16T00:05:04Z"
---
# Mappa dei contesti di ARchetipo

<!-- archetipo:wiki section=contexts -->
## Contesti osservati

- **Workflow dell'agente.** Le skill orchestrano le fasi di prodotto e invocano operazioni esplicite della CLI; i loro helper assemblano payload deterministici senza diventare un runtime applicativo separato.
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

La configurazione `.archetipo/config.yaml`, il formato di envelope JSON, i tipi di dominio, le regole del runtime condiviso e il filesystem Git sono infrastruttura attraversata da piu flussi. Gli script Node.js supportano assemblaggio, packaging e test; non possiedono dati di dominio autonomi. I comandi e i vincoli sono raccolti in [sviluppo e operazioni](/operations/development.md).

<!-- archetipo:wiki section=uncertainties -->
## Confini non ancora risolti

Questa mappa descrive dipendenze osservabili e non assegna nomi DDD specializzati alle relazioni, perche il codice non documenta una governance semantica sufficiente. Le skill, la CLI, i connector, il viewer e il packaging sono confini tecnici verificati, ma questa prima mappa non li promuove automaticamente a bounded context. L'ispezione di `cli` e `test` e campionata; cambiamenti futuri possono richiedere una mappa piu granulare.
