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
review:
    content_hash: sha256:f39f1ce5a13ada281c3df558973ffb18e0986de4e5001e4d45eabcc894e4a685
    evidence_revision: cc9ba1eabbd6187a04b50d84b69f7f434f3a4935
    evidence_hash: sha256:c4ff01c3e28f437b5b66c1307a538c2b5a4162b9be6abb3a2d12143b225cef98
    reviewed_at: "2026-08-15T20:45:27Z"
---
# Mappa dei contesti di ARchetipo

<!-- archetipo:wiki section=contexts -->
## Contesti osservati

- **Workflow dell'agente.** Le skill orchestrano le fasi di prodotto e invocano operazioni esplicite della CLI; i loro helper assemblano payload deterministici senza diventare un runtime applicativo separato.
- **CLI e workflow persistente.** `cli/internal/cli/root.go` compone i comandi pubblici e instrada le operazioni verso configurazione, validazione, Wiki, worktree e connector.
- **Connector.** `connector.Connector` definisce lettura e scrittura di PRD, backlog, spec, piani, task e stato del workflow. Le implementazioni file, GitHub, Jira e in-memory condividono questo contratto.
- **Esecuzione tramite provider.** Il package `execution` seleziona un provider per ID e capability; per `plan` richiede `spec.plan` prima del dispatch e persiste l'outcome separatamente dagli artefatti del connector.
- **Viewer locale.** Il server web espone board, metriche, spec, piani, review, configurazione e mockup tramite HTTP e usa il connector come backend.
- **Distribuzione e verifica.** Lo shim npm seleziona il binario nativo; gli script sincronizzano skill, runtime e pacchetti di piattaforma; l'harness E2E costruisce la CLI e verifica scenari in sandbox.

La [panoramica](/overview.md) definisce il perimetro e la [mappa del codice](/engineering/code-map.md) collega questi contesti ai file.

<!-- archetipo:wiki section=relationships -->
## Relazioni

Le skill dipendono dalla superficie pubblica della CLI e dal suo envelope JSON. La CLI carica configurazione e connector per leggere o modificare il source of truth del workflow. Il viewer dipende dallo stesso connector, ma aggiunge un'interfaccia HTTP locale e asset incorporati.

Il servizio di esecuzione riceve dalla CLI una spec gia letta e un registry di provider, ma non riceve `connector.Connector`: il provider non puo invocare transizioni di stato della spec. La verifica fail-closed della capability `spec.plan`, il singolo record durevole e gli outcome `SUCCEEDED` o `FAILED` sono fissati nella [decisione sul confine dei provider](/decisions/execution-provider-boundary.md).

Il packaging npm dipende dagli artefatti GoReleaser e dalle skill sorgente. Lo shim delega poi al binario specifico per sistema operativo e architettura. L'harness di test dipende dal binario costruito dal sorgente e crea sandbox isolate per esercitare CLI, skill e Wiki.

<!-- archetipo:wiki section=shared -->
## Infrastruttura condivisa

La configurazione `.archetipo/config.yaml`, il formato di envelope JSON, i tipi di dominio, le regole del runtime condiviso e il filesystem Git sono infrastruttura attraversata da piu flussi. Gli script Node.js supportano assemblaggio, packaging e test; non possiedono dati di dominio autonomi. I comandi e i vincoli sono raccolti in [sviluppo e operazioni](/operations/development.md).

<!-- archetipo:wiki section=uncertainties -->
## Confini non ancora risolti

Questa mappa descrive dipendenze osservabili e non assegna nomi DDD specializzati alle relazioni, perche il codice non documenta una governance semantica sufficiente. Le skill, la CLI, i connector, il viewer e il packaging sono confini tecnici verificati, ma questa prima mappa non li promuove automaticamente a bounded context. L'ispezione di `cli` e `test` e campionata; cambiamenti futuri possono richiedere una mappa piu granulare.
