# Project Wiki

## Architecture

* [Mappa dei contesti di ARchetipo](architecture/context-map.md) - Confini runtime osservabili e dipendenze tra skill, CLI, connector, provider, viewer e distribuzione. _State: evidence-changed._

## Decisions

* [Confine dei provider di esecuzione](decisions/execution-provider-boundary.md) - Separa l'esecuzione delle azioni dai connector e conserva ogni outcome in un record locale interrogabile. _State: evidence-changed._
* [Le conversazioni di View sono sessioni native dell'harness](decisions/native-harness-sessions.md) - Una conversazione del viewer è una sessione durevole del vero harness di sviluppo, con contesto nativo, scelte per turno e azioni di processo nello stesso thread. _State: generated._
* [Proprietà remota della scrittura del piano](decisions/remote-plan-ownership.md) - L'agente remoto scrive il piano attraverso il connector condiviso e ARchetipo ne accetta il successo solo dietro ricevuta. _State: stale._
* [Template di processo del workspace](decisions/workspace-process-template.md) - Il processo di un workspace è un valore builtin nominato, risolto per ID all'inizializzazione e conservato nella configurazione. _State: evidence-changed._

## Engineering

* [Mappa del codice di ARchetipo](engineering/code-map.md) - Corrispondenza fisica tra responsabilita osservabili, entry point, integrazioni, test e boundary ispezionati. _State: evidence-changed._

## Operations

* [Sviluppo e operazioni di ARchetipo](operations/development.md) - Comandi locali, pipeline CI, packaging, release e vincoli operativi del repository. _State: evidence-changed._

## Project

* [Panoramica di ARchetipo](overview.md) - Scopo, attori, stack e perimetro della mappa codebase-first di ARchetipo. _State: evidence-changed._
