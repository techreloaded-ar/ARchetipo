---
type: decision
title: Proprietà remota della scrittura del piano
description: L'agente remoto scrive il piano attraverso il connector condiviso e ARchetipo ne accetta il successo solo dietro ricevuta
status: reviewed
decision_status: accepted
sources:
    - path: cli/internal/execution/arcipelago/provider.go
      role: implementation
    - path: cli/internal/execution/arcipelago/prompt.go
      role: implementation
    - path: cli/internal/cli/execution_cmd.go
      role: implementation
    - path: cli/internal/execution/arcipelago/provider_test.go
      role: verification
    - path: cli/internal/cli/execution_arcipelago_test.go
      role: verification
review:
    content_hash: sha256:603376f2c3f01d1402f086651c1a780ce5fd304f05cf12b083fc3613009f2b93
    evidence_revision: 9971aeffca5b0e72438e465a01df7e07dd7459e4
    evidence_hash: sha256:7a1a3e190c227a52450c997c0f074c0c6489c5c62d18b42897231fec3b2e8341
    reviewed_at: "2026-08-16T00:05:04Z"
---
# Proprietà remota della scrittura del piano

<!-- archetipo:wiki section=context -->
## Contesto

ARchetipo può affidare l'azione `plan` a un esecutore remoto: il [confine dei provider di esecuzione](execution-provider-boundary.md) stabilisce come quel dispatch avviene, questa pagina stabilisce chi possiede l'artefatto che ne risulta. Il piano non è un valore di ritorno qualsiasi: è un artefatto voluminoso e strutturato — corpo del piano più lista di task con contratto operativo — e la sua persistenza deve avvenire nel connector configurato, che è la source of truth di backlog, piani e stato del workflow.

Occorre quindi stabilire chi scrive quell'artefatto quando l'esecuzione avviene altrove, e come ARchetipo può sapere che la scrittura è davvero avvenuta senza attribuire al confine di esecuzione un accesso al connector che il modello gli nega di proposito.

<!-- archetipo:wiki section=decision -->
## Decisione

La scrittura del piano appartiene all'**agente remoto**. L'agente invoca la skill di pianificazione di ARchetipo nella working directory del runner e persiste il piano attraverso il connector configurato, condiviso con ARchetipo. ARchetipo avvia l'azione, attende l'esito e registra nel proprio record soltanto l'identificativo remoto e una ricevuta compatta: nessun payload di piano attraversa il canale dei messaggi.

La verifica di quella scrittura è a **due livelli**, perché i due livelli possono osservare cose diverse.

Il primo livello è la **ricevuta**, e vive nel provider. Il prompt inviato al runner impone all'agente remoto di chiudere con una singola riga JSON `{"spec_code", "status", "tasks"}`, e il provider la pretende. Un task remoto che si chiude `completed` senza ricevuta, con uno stato diverso da `PLANNED`, con zero task o con uno spec code diverso da quello richiesto è un fallimento del provider. Il riconoscimento della ricevuta è per campi attesi, non per posizione: un dump d'errore o un frammento di output stampato dopo la ricevuta è anch'esso un oggetto JSON e non deve poterla oscurare.

Il secondo livello è la **verifica di stato**, e vive nel comando CLI. Una ricevuta è una dichiarazione dell'agente, non un'ispezione: una skill remota che muore a metà, o un agente che allucina la propria chiusura, produce una ricevuta ben formata su una spec che nessuno ha pianificato. Il provider non può distinguere i due casi perché non possiede il connector, ma il comando `execution run` sì: lo ha già in mano e lo ha già usato per leggere la spec prima del dispatch. Dopo un esito riuscito dell'azione `plan`, quindi, rilegge la spec e pretende che il connector la riporti `PLANNED` con almeno un task di piano leggibile. Se la rilettura smentisce la ricevuta l'esecuzione non è presentata come riuscita: il record viene riscritto `FAILED` con codice `UNCONFIRMED_EFFECT`, il motivo e l'identificativo del task remoto, e il comando esce in errore.

La divisione dei due livelli è deliberata e non è un dettaglio implementativo: il confine di esecuzione continua a non ricevere il connector, quindi resta vero per costruzione che un fallimento remoto non può toccare la spec. La verifica di stato non transita nulla, legge soltanto, e legge da uno strato che il connector lo possiede già.

Ne segue un prerequisito operativo esplicito: la working directory del runner è un checkout dello stesso progetto e condivide lo stato del connector configurato.

<!-- archetipo:wiki section=alternatives -->
## Alternative

- **Far tornare il payload completo del piano nel campo di riassunto del task remoto**: scartata perché quel campo è il messaggio finale dell'agente (`resultSummary` è `lastAssistantText()` del runner), mentre un piano reale supera facilmente le decine di migliaia di token. È inoltre esattamente l'anti-pattern che la stessa skill di pianificazione vieta.
- **Far tornare un riferimento ad artefatto e scaricarlo**: scartata perché l'API esterna di ARcipelago espone oggi soltanto `status` e `resultSummary` e non ha alcun canale artefatti.
- **Ricostruire il piano localmente a partire da un riassunto**: scartata perché produrrebbe un artefatto diverso da quello verificato in remoto, cioè un piano che nessuno ha davvero validato.
- **Rileggere la spec dal connector dentro il provider**: scartata perché darebbe al confine di esecuzione un accesso al connector che il modello gli nega di proposito, indebolendo la garanzia che un fallimento remoto non tocchi la spec. La rilettura in sé non è stata scartata: è stata spostata nello strato CLI, che il connector lo possiede già, ed è la soluzione adottata.
- **Fermarsi alla sola ricevuta**: scartata perché una ricevuta è una dichiarazione dell'agente. Esclude l'agente che si limita a terminare, non quello che dichiara il falso: una skill remota interrotta a metà o un modello che allucina la chiusura produce una ricevuta valida su una spec ancora `TODO`.
- **Rinunciare del tutto alla verifica, dichiarando l'esito non osservabile localmente**: scartata perché renderebbe indistinguibile un piano prodotto da un agente che si è limitato a terminare.

<!-- archetipo:wiki section=consequences -->
## Conseguenze

Nessun payload voluminoso attraversa il canale dei messaggi e il piano nasce già valido nel connector, senza un passaggio di serializzazione e rivalidazione. In cambio l'integrazione richiede un prerequisito operativo sul checkout del runner: è immediato con connector condivisi come `github` o `jira`, mentre con un connector locale richiede che il runner lavori sulla stessa checkout.

Il prerequisito ha una forma esatta e verificabile, non una preferenza. L'API esterna accetta soltanto `workspaceId`, `source`, `externalId`, `title`, `prompt` e `metadata`: non accetta `cwdHint`, né `skills`, né `assigneeAgentId`, né `targetRunnerId`. Il dispatcher risolve la working directory come `task.cwdHint ?? runner.system.workdirs[0]`; poiché `cwdHint` è assente per costruzione in un task esterno, la directory effettiva è sempre la **prima** working directory del runner scelto. Il workspace usato per la pianificazione remota deve quindi ammettere soltanto runner la cui prima working directory è il checkout di ARchetipo, con le skill ARchetipo già installate e il medesimo connector configurato.

L'esito remoto è dichiarato dalla ricevuta e confermato dallo stato del connector, quindi un successo osservato dall'utente è un successo verificato e non soltanto asserito. Il prezzo è una lettura in più per ogni `plan` riuscito e un esito in più da saper leggere: un'esecuzione può ora fallire *dopo* che il provider ha avuto successo, con codice `UNCONFIRMED_EFFECT` e uscita in errore anziché con l'exit `0` che segnala un fallimento remoto ordinario. La verifica non si applica a un record riusato per idempotenza: il suo effetto è già stato confermato quando è stato creato, e la spec nel frattempo può legittimamente essere andata avanti.

Un task remoto già avviato sopravvive a qualunque fallimento locale successivo alla sua creazione — timeout, errore HTTP durante il polling, ricevuta assente o smentita — e ogni messaggio di quei percorsi ne riporta l'identificativo insieme alla rotta di recupero `GET /api/external/tasks/by-reference?workspaceId&source&externalId`; lo stesso identificativo è conservato nel campo strutturato `error.external_id` del record. Il trasferimento automatico di skill o repository al runner resta fuori ambito.

<!-- archetipo:wiki section=verification -->
## Verifica

Il costruttore deterministico di titolo, prompt e ricevuta vive in [prompt.go](../../../cli/internal/execution/arcipelago/prompt.go), dove lo stato atteso è legato alla costante canonica del dominio invece di essere scritto due volte come letterale; il provider che pretende la ricevuta prima di dichiarare il successo è in [provider.go](../../../cli/internal/execution/arcipelago/provider.go). La verifica di stato successiva vive in [execution_cmd.go](../../../cli/internal/cli/execution_cmd.go).

Il contratto HTTP e ogni esito remoto — inclusi i casi `completed` senza ricevuta valida e il riconoscimento della ricevuta in presenza di altro JSON stampato dopo di essa — sono esercitati contro un hub simulato in [provider_test.go](../../../cli/internal/execution/arcipelago/provider_test.go), dove un golden test fissa anche il corpo serializzato che l'hub usa come impronta. Che dopo il completamento remoto la spec risulti `PLANNED` con un piano leggibile dal connector, che un `completed` senza piano la lasci `TODO`, e che **una ricevuta valida su una spec ancora `TODO` non produca un'esecuzione riuscita** ma un record `UNCONFIRMED_EFFECT` e un'uscita in errore, è provato end-to-end dalla sola CLI in [execution_arcipelago_test.go](../../../cli/internal/cli/execution_arcipelago_test.go).
