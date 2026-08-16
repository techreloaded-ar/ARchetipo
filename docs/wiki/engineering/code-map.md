---
type: code-map
title: Mappa del codice di ARchetipo
description: Corrispondenza fisica tra responsabilita osservabili, entry point, integrazioni, test e boundary ispezionati
status: reviewed
sources:
    - path: cli/cmd/archetipo/main.go
      role: cli-entrypoint
    - path: cli/internal/cli/root.go
      role: command-composition
    - path: cli/internal/config/config.go
      role: shared-configuration
    - path: skills/archetipo-plan/references/assemble-plan-payload.mjs
      role: skill-helper
    - path: npm/archetipo/bin/archetipo.js
      role: package-entrypoint
    - path: test/e2e/run.mjs
      role: verification-harness
    - path: cli/internal/execution/arcipelago/provider.go
      role: implementation
    - path: cli/internal/cli/execution_arcipelago_test.go
      role: verification
coverage:
    - kind: boundary
      path: .
      status: mapped
      pages:
        - overview
    - kind: boundary
      path: cli
      status: mapped
      pages:
        - engineering/code-map
        - decisions/execution-provider-boundary
        - decisions/remote-plan-ownership
    - kind: boundary
      path: npm
      status: mapped
      pages:
        - operations/development
    - kind: boundary
      path: scripts
      status: mapped
      pages:
        - operations/development
    - kind: boundary
      path: skills
      status: mapped
      pages:
        - overview
    - kind: boundary
      path: test
      status: mapped
      pages:
        - operations/development
    - kind: capability
      path: trip
      status: excluded
      note: Fixture isolata in test/e2e/fixtures/wiki-codebase usata per verificare il bootstrap Wiki; non e una capability del prodotto ARchetipo.
review:
    content_hash: sha256:72ede1a76c91b07699721b857376f0a725c5294e70a4473844e4d65b9dd2f896
    evidence_revision: 9971aeffca5b0e72438e465a01df7e07dd7459e4
    evidence_hash: sha256:54914b603a20daa9130f2c042afb2406c0dfc882f76ebae18cc7bfb25045d1ed
    reviewed_at: "2026-08-16T00:05:04Z"
---
# Mappa del codice di ARchetipo

<!-- archetipo:wiki section=domain-code -->
## Responsabilita e codice

| Responsabilita osservata | Entry point e interfacce | Logica e dati | Integrazioni | Verifica | Concetto Wiki |
|---|---|---|---|---|---|
| CLI e workflow | `cli/cmd/archetipo/main.go`, `cli/internal/cli/root.go`, comandi in `cli/internal/cli` | tipi in `cli/internal/domain`, configurazione e servizi interni | connector file, GitHub, Jira e in-memory | test dei package `cli/internal/*` | [Panoramica](/overview.md) |
| Persistenza del workflow | `cli/internal/connector/connector.go` | implementazioni sotto `cli/internal/connector` | filesystem e servizi remoti configurati | conformance e test specifici dei connector | [Mappa dei contesti](/architecture/context-map.md) |
| Esecuzione provider | `cli/internal/cli/execution_cmd.go` (comando `run`, flag `--provider` e `--request-id`, mappatura degli errori di configurazione, verifica di stato dopo un `plan` riuscito e flag `reused` nell'envelope), `cli/internal/cli/root.go` (registrazione del provider nel registry della CLI reale) | registry, service e file store in `cli/internal/execution`; derivazione deterministica dell'ID in `cli/internal/execution/id.go`; riuso idempotente in `cli/internal/execution/service.go`; selezione workspace e upsert atomico in `cli/internal/config` | provider iniettati e registrati, `.archetipo/config.yaml` e `.archetipo/executions` | test config, `provider_test.go`, `service_test.go`, `id_test.go`, `file_store_test.go`, test CLI | [Decisione provider](/decisions/execution-provider-boundary.md) |
| Provider di esecuzione ARcipelago | `cli/internal/execution/arcipelago/provider.go` (identità, capability, `Execute`), `config.go` (validazione della configurazione non segreta e default) | contratto HTTP e classificazione degli status in `client.go`; costruzione deterministica di titolo, prompt, metadata e lettura della ricevuta in `prompt.go` | API `/api/external/*` dell'hub ARcipelago, credenziale letta dall'ambiente | `cli/internal/execution/arcipelago/config_test.go` e `provider_test.go` come test di package contro un hub simulato; `cli/internal/cli/execution_arcipelago_test.go` come test di integrazione end-to-end del percorso remoto | [Proprietà remota del piano](/decisions/remote-plan-ownership.md) |
| Viewer locale | comando `view`, `cli/internal/web/server.go` | handler, broker, watcher e asset web incorporati | connector e filesystem locale | test sotto `cli/internal/web` e smoke del viewer | [Mappa dei contesti](/architecture/context-map.md) |
| Skill e payload | `skills/*/SKILL.md` | runtime condiviso e helper `assemble-*-payload.mjs` | CLI tramite envelope JSON | scenari E2E basati su skill installate | [Panoramica](/overview.md) |
| Packaging e release | `npm/archetipo/bin/archetipo.js` | pacchetti npm e script in `scripts` | GoReleaser, npm registry, GitHub Actions | CI multipiattaforma e dry-run degli script quando applicabile | [Sviluppo e operazioni](/operations/development.md) |
| Harness di test | `test/e2e/run.mjs` e manifest `test/e2e/run.yaml` | fixture, sandbox, report HTML e summary JSON | binario Go costruito dal sorgente e agenti configurati | test Node unitari, smoke Wiki e scenari E2E | [Sviluppo e operazioni](/operations/development.md) |

<!-- archetipo:wiki section=shared -->
## Codice condiviso

`cli/internal/domain`, `cli/internal/config`, `cli/internal/iox`, il runtime `.archetipo/shared-runtime.md` e gli helper Git/worktree supportano piu comandi. `cli/internal/config` modella anche `execution.default_provider` e ne aggiorna atomicamente soltanto il nodo dedicato. La root CLI registra i comandi e fornisce le dipendenze di esecuzione; `package.json` raccoglie i comandi di build e test Node/Go. Questi elementi sono infrastruttura condivisa, non capability autonome.

<!-- archetipo:wiki section=unmapped -->
## Elementi non mappati come capability

Non emergono capability di prodotto non coperte dall'inventario corrente. `trip` compare soltanto sotto `test/e2e/fixtures/wiki-codebase/src`: e il dominio fittizio di una fixture che verifica l'ispezione e il bootstrap della Wiki, quindi e escluso esplicitamente. Metadata, dipendenze, directory di build e la root Wiki configurata sono esclusioni dichiarate dall'inspector.

<!-- archetipo:wiki section=coverage -->
## Copertura deterministica

Il frontmatter copre tutti i boundary restituiti dall'ultima ispezione: `.` e `skills` rimandano alla panoramica; `cli` a questa mappa, alla decisione sul confine dei provider e alla decisione sulla proprietà remota del piano; `npm`, `scripts` e `test` alle operazioni di sviluppo. La candidate `trip` e marcata `excluded` con la motivazione della fixture. La copertura va riallineata quando `wiki inspect` aggiunge o rimuove boundary o candidate.
