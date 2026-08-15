---
type: decision
title: Confine dei provider di esecuzione
description: Separa l'esecuzione delle azioni dai connector e conserva ogni outcome in un record locale interrogabile
status: reviewed
decision_status: accepted
sources:
    - path: cli/internal/execution/provider.go
      role: implementation
    - path: cli/internal/execution/service.go
      role: implementation
    - path: cli/internal/execution/service_test.go
      role: verification
    - path: cli/internal/config/config.go
      role: implementation
    - path: cli/internal/config/config_test.go
      role: verification
    - path: cli/internal/cli/execution_cmd.go
      role: implementation
    - path: cli/internal/cli/execution_cmd_internal_test.go
      role: verification
review:
    content_hash: sha256:fce59959ca683d2f78064446b81b834e1a2e86c8970f8123e28da522852f68b1
    evidence_revision: cc9ba1eabbd6187a04b50d84b69f7f434f3a4935
    evidence_hash: sha256:f19e3155875a69e344bc67016a809035d177f8b4716ae6f2777154c1d039bd45
    reviewed_at: "2026-08-15T20:45:26Z"
---
# Confine dei provider di esecuzione

<!-- archetipo:wiki section=context -->
## Contesto

ARchetipo deve affidare azioni a esecutori intercambiabili senza aggiungere una responsabilità estranea ai connector che gestiscono backlog, piani e stato del workflow. Il risultato o l'errore di un dispatch deve inoltre restare interrogabile dopo la fine del processo. La prima azione supportata è `plan`, associata alla capability machine-readable `spec.plan`.

<!-- archetipo:wiki section=decision -->
## Decisione

I provider di esecuzione implementano un contratto separato da `connector.Connector` e vengono selezionati da un registry iniettabile. Il workspace può conservare in `execution.default_provider` un ID e una mappa di configurazione non segreta; `--provider` prevale sempre sul default e usa una configurazione vuota. Token e credenziali non vengono persistiti nel config.

Ogni provider possiede la validazione della propria configurazione. `set-default` risolve e valida il provider prima di aggiornare atomicamente il solo nodo `execution.default_provider`, preservando commenti e sezioni non correlate; un errore conserva byte-per-byte l'ultima selezione valida. Il servizio ripete la validazione dopo il controllo `spec.plan` ma prima di generare l'ID o creare dati, così anche modifiche manuali invalide falliscono chiuse. Ogni dispatch accettato crea esattamente un record `RUNNING` in `.archetipo/executions/`, poi aggiorna lo stesso record a `SUCCEEDED` con un payload JSON opaco oppure a `FAILED` con un errore strutturato.

Il provider riceve una richiesta serializzabile e non riceve il connector. Di conseguenza non può cambiare lo stato della spec. Un fallimento del provider già dispatchato è un outcome di dominio: la CLI restituisce exit `0` e un envelope `kind: "execution"` che conserva l'ID. Provider sconosciuti e capability mancanti falliscono prima della creazione del record.

<!-- archetipo:wiki section=alternatives -->
## Alternative

- Estendere `connector.Connector`: scartato perché costringerebbe file, GitHub, Jira e inmemory a implementare una capacità che non appartiene alla persistenza del workflow.
- Usare un adapter generico basato su comando shell: scartato perché perderebbe capability tipizzate e risultati strutturati.
- Restituire soltanto il valore della chiamata: scartato perché risultato, errore ed eventuale `external_id` non sarebbero rileggibili dopo il processo.
- Validare soltanto durante `run`: scartato perché consentirebbe di persistere configurazioni già note come invalide.
- Inserire gli schemi dei provider nel package `config`: scartato perché legherebbe il core ai provider futuri.
- Affidarsi soltanto all'editing manuale YAML: scartato perché non garantirebbe la conservazione atomica dell'ultima configurazione valida.

<!-- archetipo:wiki section=consequences -->
## Conseguenze

Il confine tra source of truth e sistema esecutore resta esplicito, i fake possono essere iniettati senza stato globale e le integrazioni future possono conservare un `external_id`. La doppia validazione protegge sia la scrittura sia il dispatch; la configurazione resta versionabile proprio perché esclude i segreti. Il costo è un piccolo lifecycle dedicato, un upsert YAML atomico e una directory runtime locale ignorata da Git. Retry, idempotenza cross-request, fallback, selezione automatica e provider remoto reale restano fuori da questa decisione.

<!-- archetipo:wiki section=verification -->
## Verifica

Il registry, `ConfigurationError` e il lifecycle sono implementati in [provider.go](../../../cli/internal/execution/provider.go) e [service.go](../../../cli/internal/execution/service.go); l'upsert atomico vive in [config.go](../../../cli/internal/config/config.go). I test del servizio verificano configurazione copiata, validazione prima degli effetti, una sola invocation, assenza di retry e outcome mutuamente esclusivi in [service_test.go](../../../cli/internal/execution/service_test.go). Il comando pubblico è in [execution_cmd.go](../../../cli/internal/cli/execution_cmd.go); i test CLI attraversano connector file, provider fake, parser YAML, store reale ed envelope in [execution_cmd_internal_test.go](../../../cli/internal/cli/execution_cmd_internal_test.go), inclusi rollback, default automatico e precedenza dell'override.
