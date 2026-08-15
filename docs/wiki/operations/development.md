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
    content_hash: sha256:225fd7a3df2ec993bedf68b23bfabcc481a12a8b1fd1b5818a3b5d5af2627541
    evidence_revision: cc9ba1eabbd6187a04b50d84b69f7f434f3a4935
    evidence_hash: sha256:257c258a2e1d7401c278800a35c037ecf9e398dbc0aecbbff2132d480fccc5b1
    reviewed_at: "2026-08-15T20:45:26Z"
---
# Sviluppo e operazioni di ARchetipo

## Prerequisiti e runtime locale

Lo sviluppo richiede Go secondo `cli/go.mod` e Node.js con le dipendenze del `package-lock.json`. Il binario si avvia dal modulo Go; `npm run build:cli:dev`, `npm run install:dev` e `npm run uninstall:dev` gestiscono il ciclo locale. `archetipo view` avvia il viewer HTTP locale, che per impostazione predefinita ascolta su loopback e non offre autenticazione.

La CLI legge `.archetipo/config.yaml`, emette envelope JSON su stdout o stderr e puo operare con connector file, GitHub o Jira. `execution.default_provider` contiene soltanto ID e configurazione non segreta del provider; token e credenziali restano nell'ambiente o in un meccanismo esterno. `archetipo execution provider set-default <id> --file <path>` valida e salva atomicamente la selezione, `show-default` la verifica, mentre `execution run` la usa in assenza di `--provider`. Le integrazioni remote richiedono le rispettive credenziali; i test unitari e gli smoke Wiki sono progettati per non richiederle.

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
- Le directory generate `cli/dist`, `test/e2e/.bin` e `test/workspaces` non sono sorgenti da versionare.
- I connector remoti possono dipendere da rete e autenticazione; il connector file consente il flusso offline.
- Il viewer e destinato all'uso locale singolo e chiude il server con un periodo di grazia quando il contesto viene cancellato.
- Le operazioni Wiki rigenerano `docs/wiki/index.md` e `docs/wiki/log.md` esclusivamente tramite la CLI; le pagine create durante implementazione restano `generated` fino alla review esplicita.
