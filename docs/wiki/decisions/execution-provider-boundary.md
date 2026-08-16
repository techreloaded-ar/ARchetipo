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
    - path: cli/internal/execution/id.go
      role: implementation
    - path: cli/internal/cli/root.go
      role: implementation
    - path: cli/internal/cli/execution_arcipelago_test.go
      role: verification
review:
    content_hash: sha256:b0da9bfdbdf997f61184bdd4159b9f1ac62e1b52d201d5c2fb42916ce96f7e5e
    evidence_revision: 9971aeffca5b0e72438e465a01df7e07dd7459e4
    evidence_hash: sha256:69a782b4855b54142e797c6c04b94fdd533e5ff4c63da3df14156afa335ed445
    reviewed_at: "2026-08-16T00:05:04Z"
---
# Confine dei provider di esecuzione

<!-- archetipo:wiki section=context -->
## Contesto

ARchetipo deve affidare azioni a esecutori intercambiabili senza aggiungere una responsabilità estranea ai connector che gestiscono backlog, piani e stato del workflow. Il risultato o l'errore di un dispatch deve inoltre restare interrogabile dopo la fine del processo. La prima azione supportata è `plan`, associata alla capability machine-readable `spec.plan`.

La prima azione `plan` ha ora un esecutore remoto reale, non più soltanto provider fake nei test. Un dispatch che attraversa la rete può fallire o restare senza risposta dopo aver già avviato lavoro dall'altro lato, quindi una richiesta ripetibile deve poter essere ripetuta senza duplicare quel lavoro.

<!-- archetipo:wiki section=decision -->
## Decisione

I provider di esecuzione implementano un contratto separato da `connector.Connector` e vengono selezionati da un registry iniettabile. Il workspace può conservare in `execution.default_provider` un ID e una mappa di configurazione non segreta; `--provider` prevale sempre sul default e usa una configurazione vuota. Token e credenziali non vengono persistiti nel config.

Ogni provider possiede la validazione della propria configurazione. `set-default` risolve e valida il provider prima di aggiornare atomicamente il solo nodo `execution.default_provider`, preservando commenti e sezioni non correlate; un errore conserva byte-per-byte l'ultima selezione valida. Il servizio ripete la validazione dopo il controllo `spec.plan` ma prima di generare l'ID o creare dati, così anche modifiche manuali invalide falliscono chiuse. Ogni dispatch accettato crea esattamente un record `RUNNING` in `.archetipo/executions/`, poi aggiorna lo stesso record a `SUCCEEDED` con un payload JSON opaco oppure a `FAILED` con un errore strutturato.

Il provider riceve una richiesta serializzabile e non riceve il connector. Di conseguenza non può cambiare lo stato della spec. Un fallimento del provider già dispatchato è un outcome di dominio: la CLI restituisce exit `0` e un envelope `kind: "execution"` che conserva l'ID. Provider sconosciuti e capability mancanti falliscono prima della creazione del record.

Proprio perché il provider non vede il connector, il successo che dichiara è verificato dallo strato CLI, che invece lo possiede: dopo un `plan` riuscito il comando rilegge la spec e, se il connector non conferma l'effetto, riscrive il record `FAILED` e esce in errore. Il modo in cui quella verifica è disegnata, e perché non appartiene al provider, sono materia della [proprietà remota della scrittura del piano](remote-plan-ownership.md).

L'idempotenza cross-request è esplicita e opt-in: si attiva soltanto passando una chiave di richiesta. Con essa l'ID dell'esecuzione non è più casuale ma derivato deterministicamente da spec code, azione, provider e chiave, così la stessa richiesta produce sempre lo stesso identificativo. Se il record esiste già viene restituito senza invocare il provider, e una creazione che incontra un record già presente — la corsa fra due richieste concorrenti — è trattata anch'essa come riuso. Il riuso è incondizionato: vale anche per un record fallito.

Il medesimo identificativo viaggia come identità esterna verso il sistema remoto, così l'idempotenza locale e quella remota coincidono e nessuna coppia richiesta/spec può collidere con un'altra. L'equivalenza remota, però, non si esaurisce nella terna di identità: comprende anche un'impronta della richiesta calcolata su titolo, prompt e metadata. Ne segue che la stessa chiave usata per un incarico diverso è un conflitto e non un riuso, e che il payload inviato al sistema remoto deve essere una funzione pura e deterministica dei soli campi della richiesta.

I segreti dei provider provengono dall'ambiente attraverso un nome di variabile dichiarato nella configurazione non segreta, mai dal file di configurazione. La validazione della forma non richiede che la variabile sia popolata, così la selezione del default resta eseguibile su una macchina priva della credenziale; l'assenza del segreto viene rilevata al dispatch. Infine, il registry costruito dalla CLI reale non è più vuoto: contiene un provider di rete registrato, quindi il confine è raggiungibile dalla sola riga di comando.

<!-- archetipo:wiki section=alternatives -->
## Alternative

- Estendere `connector.Connector`: scartato perché costringerebbe file, GitHub, Jira e inmemory a implementare una capacità che non appartiene alla persistenza del workflow.
- Usare un adapter generico basato su comando shell: scartato perché perderebbe capability tipizzate e risultati strutturati.
- Restituire soltanto il valore della chiamata: scartato perché risultato, errore ed eventuale `external_id` non sarebbero rileggibili dopo il processo.
- Validare soltanto durante `run`: scartato perché consentirebbe di persistere configurazioni già note come invalide.
- Inserire gli schemi dei provider nel package `config`: scartato perché legherebbe il core ai provider futuri.
- Affidarsi soltanto all'editing manuale YAML: scartato perché non garantirebbe la conservazione atomica dell'ultima configurazione valida.
- Lifecycle asincrono con avvio non bloccante e comando di ripresa: scartato perché richiederebbe di riaprire il lifecycle che finalizza il record nella stessa invocazione, mentre l'attesa sincrona limitata soddisfa già i criteri di accettazione. Da scartare è il *lifecycle*, non lo stato `RUNNING`, che invece viene persistito: è la fase transitoria del record fra creazione e finalizzazione, ed è ciò che rende osservabile un dispatch in corso. Che quella fase non abbia un percorso di ripresa è un limite noto, descritto sotto.

<!-- archetipo:wiki section=consequences -->
## Conseguenze

Il confine tra source of truth e sistema esecutore resta esplicito, i fake possono essere iniettati senza stato globale e le integrazioni future possono conservare un `external_id`. La doppia validazione protegge sia la scrittura sia il dispatch; la configurazione resta versionabile proprio perché esclude i segreti. Il costo è un piccolo lifecycle dedicato, un upsert YAML atomico e una directory runtime locale ignorata da Git. Retry automatico, fallback e selezione automatica del provider restano fuori da questa decisione.

L'idempotenza cross-request porta con sé quattro limiti noti. Il riuso è incondizionato, quindi ritentare dopo un fallimento significa usare una nuova chiave di richiesta: ripetere la stessa chiave restituisce il record fallito, non un nuovo tentativo. Perché il riuso non sia invisibile, l'envelope di `execution run` porta un campo `reused`, che distingue un dispatch nuovo da un record restituito così com'era. La stessa chiave usata per un incarico diverso viene respinta come conflitto, perché l'equivalenza remota comprende un'impronta della richiesta oltre alla terna di identità. E nessun fallimento locale successivo alla creazione annulla il lavoro remoto già avviato: l'esecuzione risulta `FAILED` da questo lato mentre il task remoto resta vivo, ritrovabile per riferimento esterno con `GET /api/external/tasks/by-reference?workspaceId&source&externalId`, la rotta di recupero che l'hub espone e che ogni messaggio di fallimento successivo alla creazione cita insieme ai tre valori necessari a interrogarla; il medesimo identificativo resta leggibile da un programma nel campo `error.external_id` del record.

Il quarto limite riguarda proprio la fase `RUNNING`. Il record viene creato `RUNNING` prima del dispatch e finalizzato al ritorno del provider, quindi se il processo muore durante l'attesa il record resta `RUNNING` per sempre e **non esiste alcun percorso di ripresa**. Ripetere la stessa `--request-id` restituisce indefinitamente quel record — con `reused: true`, così almeno l'utente vede che non è stato avviato nulla di nuovo — mentre una chiave nuova crea un secondo task remoto invece di riagganciare il primo. La via d'uscita è manuale: ritrovare il task per riferimento esterno con la rotta `by-reference` e verificarne l'esito sull'hub. Un comando di ripresa che riagganci un record `RUNNING` al task remoto corrispondente resta lavoro futuro, fuori dall'ambito di questa decisione.

Un limite ulteriore, indipendente dall'idempotenza, riguarda `--provider <id>`: l'override esplicito passa sempre una configurazione **vuota**, perché non esiste un percorso da cui leggerla. È quindi utilizzabile soltanto con provider che non richiedono configurazione; con gli altri fallisce con `E_INVALID_INPUT` nominando il campo mancante, e il rimedio è configurare il default di workspace con `execution provider set-default`. Anche l'help del flag lo dichiara. Un override di configurazione da riga di comando o da file resta fuori ambito.

<!-- archetipo:wiki section=verification -->
## Verifica

Il registry, `ConfigurationError` e il lifecycle sono implementati in [provider.go](../../../cli/internal/execution/provider.go) e [service.go](../../../cli/internal/execution/service.go); l'upsert atomico vive in [config.go](../../../cli/internal/config/config.go). I test del servizio verificano configurazione copiata, validazione prima degli effetti, una sola invocation, assenza di retry e outcome mutuamente esclusivi in [service_test.go](../../../cli/internal/execution/service_test.go). Il comando pubblico è in [execution_cmd.go](../../../cli/internal/cli/execution_cmd.go); i test CLI attraversano connector file, provider fake, parser YAML, store reale ed envelope in [execution_cmd_internal_test.go](../../../cli/internal/cli/execution_cmd_internal_test.go), inclusi rollback, default automatico e precedenza dell'override.

La derivazione deterministica dell'ID vive in [id.go](../../../cli/internal/execution/id.go) e il riuso senza dispatch in [service.go](../../../cli/internal/execution/service.go); il flag di idempotenza e la mappatura degli errori di configurazione dei provider sono in [execution_cmd.go](../../../cli/internal/cli/execution_cmd.go), mentre la registrazione del provider di rete nel registry della CLI reale è in [root.go](../../../cli/internal/cli/root.go). La prova end-to-end che una seconda richiesta con la stessa chiave restituisce la stessa esecuzione senza creare un secondo task remoto è in [execution_arcipelago_test.go](../../../cli/internal/cli/execution_arcipelago_test.go).
