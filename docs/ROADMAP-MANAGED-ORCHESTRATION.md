# Roadmap ARchetipo — orchestrazione gestita

Data di riferimento: 19 agosto 2026.

Questo documento serve a riprendere lo sviluppo in una sessione che non conosce le conversazioni precedenti. Contiene la direzione di prodotto, ciò che è già consegnato, l'ordine di lavoro e le regole operative. **Non è la fonte di verità sullo stato**: quella è il backlog, interrogato con `archetipo spec list`. Quando i due divergono, vince il backlog e questo documento va corretto.

## Direzione di prodotto

ARchetipo e ARcipelago sono due prodotti distinti e utilizzabili separatamente.

- **ARchetipo** possiede il workspace percepito dall'utente, il processo, gli Archetipi, le skill, la board, la UI e la scelta del provider di esecuzione.
- **ARcipelago** possiede dispatch, runner, esecuzioni remote, eventi, messaggi, approvazioni e cancellazione delle run.
- Per ARchetipo, ARcipelago è un `ExecutionProvider` esattamente come lo sono Codex e Claude in locale.
- ARchetipo deve continuare a funzionare nella modalità diretta: l'utente apre Codex, Claude o un altro coding agent compatibile e invoca le skill ARchetipo.
- La UI di ARchetipo non va spostata dentro ARcipelago.
- Il processo resta dichiarativo e semplice — stati, azioni, skill. Nessun motore BPM/DAG generico finché una storia concreta non lo richiede.

### Terminologia

**Workspace** è l'unità che l'utente crea, apre e sceglie: ha un Archetipo, un backlog, una configurazione e un provider predefinito. È il termine di dominio, già usato in configurazione, Template e CLI.

**Project root** resta il termine tecnico per la directory che contiene `.archetipo/config.yaml`. **Project** senza qualificazione appartiene al connector GitHub (Projects v2) e non va usato per indicare un workspace. La UI ha ancora qualche etichetta `Project ...` ereditata: allinearla è cosmesi opportunistica, non una storia.

## Stato consegnato

Entrambi i repository lavorano sul branch `sleli`.

### ARchetipo — `/Users/stefanoleli/Project/ARchetipo`

`DONE`: US-023, US-024, US-025, US-026, US-027 (provider intercambiabile, provider predefinito del workspace, pianificazione via ARcipelago, Template incorporato `fabbrica-del-software`, azioni calcolate dallo stato della spec); US-028, US-029, US-030, US-031 (configurazione esecuzione e azioni nella UI, pianificazione dalla UI, interazione con una run remota, creazione manuale di una spec); US-032, US-033 (pianificazione con Codex e con Claude come provider locali).

Il branch `sleli` è 14 commit sopra `main` e non è mai stato integrato.

### ARcipelago — `/Users/stefanoleli/Project/ARcipelago`

`DONE`: US-001–US-005 (credenziali applicative limitate al workspace, creazione idempotente dei task esterni, lettura di stato e risultato per identità esterna, stream riprendibile degli eventi, messaggi/approvazioni/cancellazione da client esterno).

Queste API sono generiche e non devono acquisire semantica specifica di ARchetipo. **La roadmap corrente non richiede nuove storie ARcipelago.**

## Il problema che il prossimo blocco risolve

Oggi la UI non ha un ingresso nel processo. Il viewer serve un solo workspace, quello della directory da cui è stato lanciato, e un workspace appena inizializzato non offre alcun percorso: il pulsante *New spec* è disabilitato finché non esiste un backlog con almeno un'epica, e nulla dice come arrivarci. Chi non conosce le skill non può cominciare.

Il blocco che segue costruisce il percorso completo: creare il workspace, dialogare con un provider locale, fare l'inception, generare il backlog, ed essere accompagnati di passo in passo.

## Ordine di lavoro

Una storia per volta, ciascuna attraverso pianificazione, implementazione e review prima di iniziare la successiva. L'ordine sotto è quello del backlog: verificalo sempre con `archetipo spec list --status TODO`, che lo restituisce già ordinato.

| # | Spec | Perché in questa posizione |
|---|---|---|
| 1 | **US-044** — Creare e inizializzare un workspace dalla UI | Il punto di partenza che oggi manca. Senza, ogni passo successivo presuppone una CLI. |
| 2 | **US-038** — Dialogare con una run locale di Codex | Prerequisito reale dell'inception guidata: senza dialogo, il percorso funziona solo con un hub remoto. |
| 3 | **US-039** — Dialogare con una run locale di Claude | Stessa capacità sul secondo provider locale, senza riscrivere la semantica. |
| 4 | **US-040** — Avviare l'inception dalla UI e ottenere il PRD | Primo passo del processo eseguibile interamente dalla UI. |
| 5 | **US-041** — Generare il backlog iniziale dalla UI | Secondo passo: la board si popola e la creazione manuale di spec si sblocca da sé. |
| 6 | **US-043** — Conoscere lo stato del workspace e il passo successivo | Cuce i passi in un percorso guidato. Deliberatamente **dopo** US-040 e US-041: prima di quelle, guiderebbe verso passi inesistenti e i suoi criteri sarebbero previsioni. |
| 7 | **US-042** — Creare una spec con l'assistenza dell'agente | Raffinamento dell'ingresso, non più bloccante una volta che il backlog esiste. |
| 8 | **US-045** — Ritrovare i workspace conosciuti | Serve solo quando i workspace diventano più di uno. |
| 9 | **US-046** — Aprire un workspace dalla home senza riavviare il viewer | Completa la navigazione multi-workspace. |
| 10 | **US-034** — Implementare una spec tramite un provider | Ciclo di delivery gestito, dopo che l'ingresso è risolto. |
| 11 | **US-035** — Preparare e decidere la review con un gate umano | Chiude il ciclo mantenendo umano il verdetto. |
| 12 | **US-036** — Installare un Archetipo da un pacchetto | Generalizzazione del processo. |
| 13 | **US-037** — Inizializzare un workspace con un Archetipo installato | Aggiunge la scelta dell'Archetipo, anche all'inizializzazione dalla UI di US-044. |

### Relazione fra US-044 e US-037

Esiste un solo Archetipo, quello incorporato. **US-044 non chiede quindi alcuna scelta**: crea il workspace sul processo incorporato e ne persiste identità e versione. La scelta fra Archetipi arriva con US-037, che estende l'inizializzazione — CLI e UI — quando esiste davvero un catalogo. Non anticipare US-036/US-037 per "completare" US-044: una scelta con un elemento solo non è una funzionalità.

## Vincoli di prodotto e architettura

- I confini fra ARchetipo e ARcipelago descritti sopra non si spostano per comodità implementativa.
- Il processo resta stati + azioni + skill. Le regole del processo non vanno duplicate nel frontend: la UI consuma i contratti (`spec actions`, configurazione, capacità del provider) e non ne reimplementa la logica.
- Credenziali e materiale di sessione dei provider non entrano mai nella configurazione del workspace né nelle risposte del viewer. La configurazione provider è versionabile e non segreta.
- `RunCollaborator` (`cli/internal/execution/run.go`) è la capacità opzionale di osservare e comandare una run, scoperta a runtime con `RunCollaboratorFor` e oggi implementata dal solo ARcipelago. US-038 e US-039 la implementano per i provider locali: **estendere quell'interfaccia, non crearne una parallela.** Le capacità sono già esposte alla UI da `cli/internal/web/execution.go`.
- Con US-038, Codex non gira più one-shot: la pianificazione apre una sessione viva su `codex app-server --listen stdio://` (JSON-RPC su stdio), l'unica superficie di codex-cli 0.147.0 che accetta un messaggio mentre lavora. Claude gira ancora one-shot (`claude --print --no-session-persistence --permission-mode auto`) e US-039 gli darà la stessa sessione: il dialogo richiede una sessione viva, non un flag in più. Le regole comuni del dialogo vivono in `cli/internal/execution/localrun` e non vanno duplicate per provider.
- Il viewer è a uso locale singolo, ascolta su loopback e non autentica. US-046 introduce il cambio di workspace a runtime senza cambiare questa premessa.
- Non introdurre astrazioni per casi futuri che i criteri di accettazione della storia corrente non richiedono.

## Come si lavora

1. Opera solo sul branch `sleli` in entrambi i repository. Non integrare in `main` senza richiesta esplicita.
2. Prima di toccare codice: leggi `AGENTS.md`, esegui `archetipo config show` e `archetipo spec list`, e considera autorevole ciò che rispondono.
3. Usa le skill ARchetipo: `archetipo-plan`, `archetipo-implement`, `archetipo-review`. Usa `archetipo-autopilot` solo se richiesto esplicitamente.
4. La Wiki automatica è disattivata in questo repository (`wiki.enabled: false`): le skill di workflow non svolgono lavoro Wiki. `archetipo-wiki` invocata esplicitamente funziona comunque.
5. Prima di consegnare, da `cli/`: `gofmt -l .` (vuoto), `go vet ./...`, `go build ./...`, `go test ./...`, `golangci-lint run --timeout 5m ./...`.
6. Per il lavoro sulla UI esistono smoke senza credenziali, da lanciare dal root: `npm run test:view-execution-smoke`, `test:view-plan-smoke`, `test:view-run-smoke`, `test:view-create-smoke`, `test:view-delete-smoke`, `test:wiki-smoke`. Sono il modello da seguire per le nuove storie di UI: hub finto su `127.0.0.1`, tutto il resto reale, nessuna attesa arbitraria.
7. Preserva le modifiche non correlate presenti nel working tree.
8. Non consultare né ricreare il vecchio branch `codex/autopilot-backlog-20260723`.

## Prompt per una nuova sessione

```text
Continua lo sviluppo della roadmap di orchestrazione gestita di ARchetipo.

Repository:
- ARchetipo: /Users/stefanoleli/Project/ARchetipo
- ARcipelago: /Users/stefanoleli/Project/ARcipelago

Lavora esclusivamente sui branch `sleli` e non integrare in `main` senza una mia
richiesta esplicita.

Prima di agire:
1. leggi /Users/stefanoleli/Project/ARchetipo/docs/ROADMAP-MANAGED-ORCHESTRATION.md;
2. controlla branch e working tree di entrambi i repository;
3. esegui `archetipo config show` e `archetipo spec list` in entrambi;
4. leggi gli AGENTS.md applicabili;
5. considera autorevole lo stato restituito dal backlog, non quello riassunto qui.

Lavora la prima storia TODO nell'ordine del backlog, salvo mia indicazione
diversa. Una storia per volta: pianificazione, implementazione e review prima di
iniziare la successiva. Verifica i criteri di accettazione con test proporzionati
al rischio.

Alla fine riportami:
- stato della storia;
- funzionalità visibile consegnata;
- test eseguiti e risultato;
- decisioni o limiti rimasti aperti;
- prossimo elemento raccomandato.
```

## Manutenzione di questo documento

Aggiornalo quando cambia la direzione, l'ordine o un vincolo — non a ogni storia chiusa: lo stato per spec si legge dal backlog. Se lo trovi in disaccordo con `archetipo spec list`, correggilo prima di procedere.
