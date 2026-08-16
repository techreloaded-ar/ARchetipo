---
type: operations
title: Sviluppo e operazioni di ARchetipo
description: Comandi locali, pipeline CI, packaging, release e vincoli operativi del repository
status: reviewed
sources:
    - path: AGENTS.md
      role: development-contract
    - path: package.json
      role: command-manifest
    - path: .archetipo/config.yaml
      role: workspace-config-template
    - path: .github/workflows/ci.yml
      role: continuous-integration
    - path: .github/workflows/release.yml
      role: release-pipeline
    - path: scripts/build-npm.mjs
      role: packaging-implementation
    - path: test/e2e/run.mjs
      role: e2e-harness
review:
    content_hash: sha256:b4c7956bea6fa91a74a24666ca9d67ee0efed9aa88ddaee09a204c58b3a263a8
    evidence_revision: d19036939e60214eff4bb0f89b76ce0685298ba1
    evidence_hash: sha256:bc0c1e5745f5124550f530f9bfd5d1e15867866345150f3d0f40cacf953ec095
    reviewed_at: "2026-08-16T09:36:01Z"
---
# Sviluppo e operazioni di ARchetipo

## Prerequisiti e runtime locale

Lo sviluppo richiede Go secondo `cli/go.mod` e Node.js con le dipendenze del `package-lock.json`. Il binario si avvia dal modulo Go; `npm run build:cli:dev`, `npm run install:dev` e `npm run uninstall:dev` gestiscono il ciclo locale. `archetipo view` avvia il viewer HTTP locale, che per impostazione predefinita ascolta su loopback e non offre autenticazione.

La CLI legge `.archetipo/config.yaml`, emette envelope JSON su stdout o stderr e puo operare con connector file, GitHub o Jira. Il blocco `template:` conserva identificativo e versione del Template di processo installato nel workspace, cioe quali skill sono state installate e quali stati usa il suo workflow. `archetipo init --template <id>` seleziona il Template; omettere il flag seleziona `fabbrica-del-software`, e un identificativo sconosciuto viene rifiutato prima di scrivere qualunque file, senza lasciare un'inizializzazione parziale. `archetipo config show` riporta la selezione insieme agli altri metadati di workspace. `execution.default_provider` contiene soltanto ID e configurazione non segreta del provider; token e credenziali restano nell'ambiente o in un meccanismo esterno. `archetipo execution provider set-default <id> --file <path>` valida e salva atomicamente la selezione, `show-default` la verifica, mentre `execution run` la usa in assenza di `--provider`. Le integrazioni remote richiedono le rispettive credenziali; i test unitari e gli smoke Wiki sono progettati per non richiederle.

## Build e controlli

Prima della consegna, da `cli`:

```text
gofmt -l .
go vet ./...
go build ./...
go test ./...
golangci-lint run --timeout 5m ./...
```

`gofmt -l .` deve essere vuoto e gli altri comandi devono terminare senza errori. Dal root, `npm run test:e2e:unit` verifica schema e baseline dell'harness, mentre `npm run test:wiki-smoke` costruisce la CLI ed esercita il lifecycle Wiki. `npm run test:e2e` esegue gli scenari configurati: costruisce `test/e2e/.bin/archetipo`, crea una sandbox per run e produce `report.html` e `summary.json` sotto `test/workspaces`; questi artefatti sono ignorati da Git.

La pipeline CI ripete formattazione, vet e lint su Linux, poi esegue test Go senza cache, test Node e Wiki smoke su Linux, macOS e Windows. Sono presenti gate nativi aggiuntivi per la portabilita dei path e una compilazione incrociata supplementare dei test Wiki.

## Packaging e release

I tag `v*` avviano `.github/workflows/release.yml`: la pipeline esegue i test, produce i binari con GoReleaser, chiama `scripts/build-npm.mjs` e pubblica con `scripts/publish-npm.mjs`. Il build copia i binari nei sei pacchetti `@techreloaded/archetipo-{os}-{arch}` e sincronizza skill e runtime nel pacchetto principale. Lo shim [npm](/engineering/code-map.md) sceglie il pacchetto nativo in base a piattaforma e architettura e inoltra gli argomenti al binario.

Le release richiedono `GITHUB_TOKEN` per GoReleaser e `NPM_TOKEN` per la pubblicazione. Le versioni prerelease usano un dist-tag dedicato; lo script rifiuta di pubblicarle come `latest`. La [mappa dei contesti](/architecture/context-map.md) descrive la relazione tra distribuzione, CLI e skill.

## Vincoli operativi

- `.archetipo/executions/` contiene stato runtime locale ed e ignorata da Git.
- La configurazione provider versionabile deve essere non segreta; un override `--provider` prevale sul default e non eredita la sua mappa.
- I Template di processo sono risolti in-process dal binario: introdurne uno nuovo richiede una release della CLI. Un workspace preesistente, privo del blocco `template:`, risolve comunque la Fabbrica del software.
- Le directory generate `cli/dist`, `test/e2e/.bin` e `test/workspaces` non sono sorgenti da versionare.
- I connector remoti possono dipendere da rete e autenticazione; il connector file consente il flusso offline.
- Il viewer e destinato all'uso locale singolo e chiude il server con un periodo di grazia quando il contesto viene cancellato.
- Le operazioni Wiki rigenerano `docs/wiki/index.md` e `docs/wiki/log.md` esclusivamente tramite la CLI; le pagine create durante implementazione restano `generated` fino alla review esplicita.
