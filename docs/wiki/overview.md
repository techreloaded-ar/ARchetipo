---
type: overview
title: Panoramica di ARchetipo
description: Scopo, attori, stack e perimetro della mappa codebase-first di ARchetipo
status: reviewed
sources:
    - path: README.md
      role: product-overview
    - path: AGENTS.md
      role: development-contract
    - path: cli/internal/cli/root.go
      role: runtime-composition
    - path: package.json
      role: project-manifest
review:
    content_hash: sha256:737dda677be3357b3c83c043f1ef682a0186353906bf04b1026e20b859a96f9e
    evidence_revision: d19036939e60214eff4bb0f89b76ce0685298ba1
    evidence_hash: sha256:9dc1b5360be71a4b49d60e8956ac487deb72448acb8bb24393d4fe7b4a6b9543
    reviewed_at: "2026-08-16T09:36:03Z"
---
# Panoramica di ARchetipo

ARchetipo e un workflow spec-driven per agenti di coding. Trasforma un'idea di prodotto in artefatti persistenti e verificabili — PRD, backlog, spec, piani, mockup, implementazioni e conoscenza Wiki — tramite skill che applicano un processo comune e una CLI deterministica che gestisce le operazioni sul progetto.

## Attori e interfacce

- L'utilizzatore guida il lavoro conversando con un agente compatibile con Claude Code, Codex, Cursor, Gemini CLI, OpenCode, GitHub Copilot o Pi.
- Le skill definiscono ruoli e contratti delle fasi di inception, specifica, pianificazione, implementazione, review e manutenzione Wiki.
- La CLI `archetipo` offre l'interfaccia stabile verso configurazione, connector, validazione, worktree, viewer locale ed esecuzioni tramite provider.
- I connector mantengono gli artefatti di workflow su file locali, GitHub o Jira; il provider di esecuzione e un confine distinto, descritto nella [decisione sul confine dei provider](/decisions/execution-provider-boundary.md).

## Stack e distribuzione

Il runtime principale e scritto in Go e usa Cobra per comporre i comandi. Script Node.js assemblano payload, costruiscono e pubblicano i pacchetti npm, ed eseguono l'harness E2E. Il pacchetto `@techreloaded/archetipo` installa skill e runtime condiviso e delega a uno dei sei pacchetti binari per piattaforma.

## Perimetro ispezionato

La mappa copre i boundary deterministici restituiti da `wiki inspect`: root del progetto, modulo Go `cli`, packaging `npm`, tooling `scripts`, helper sotto `skills` e harness `test`. La vista fisica completa e nella [mappa del codice](/engineering/code-map.md); le relazioni runtime sono nella [mappa dei contesti](/architecture/context-map.md) e i comandi operativi in [sviluppo e operazioni](/operations/development.md).

Sono esclusi metadata del repository, dipendenze e directory di build, oltre alla stessa Wiki configurata. L'ispezione usa campioni rappresentativi per `cli` e `test`: questa pagina non pretende quindi di catalogare ogni file. La candidate `trip` rilevata sotto `test/e2e/fixtures/wiki-codebase` e una fixture isolata usata per testare il bootstrap Wiki, non una capability di ARchetipo.
