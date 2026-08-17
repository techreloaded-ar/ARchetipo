# Roadmap ARchetipo — orchestrazione gestita

Data di riferimento: 17 agosto 2026.

Questo documento consente di riprendere il lavoro senza recuperare la conversazione che ha portato alle decisioni di prodotto.

## Direzione di prodotto

ARchetipo e ARcipelago sono due prodotti distinti e utilizzabili separatamente.

- **ARchetipo** possiede il workspace percepito dall'utente, il processo, gli Archetipi, le skill, la board, la UI e la scelta del provider di esecuzione.
- **ARcipelago** possiede dispatch, runner, esecuzioni remote, eventi, messaggi, approvazioni e cancellazione delle run.
- Per ARchetipo, ARcipelago è un `ExecutionProvider` come potranno esserlo Codex e Claude in locale.
- ARchetipo deve continuare a funzionare nella modalità diretta attuale: l'utente apre Codex, Claude o un altro coding agent compatibile e invoca le skill ARchetipo.
- La UI di ARchetipo non deve essere spostata dentro ARcipelago.
- Il processo resta dichiarativo e semplice: stati, azioni e skill. Non va introdotto un motore BPM/DAG generico finché una storia concreta non lo richiede.

## Stato consegnato

Entrambi i repository lavorano sul branch `sleli`.

### ARchetipo

Repository: `/Users/stefanoleli/Project/ARchetipo`

Completate:

- US-023 — provider di esecuzione intercambiabile;
- US-024 — provider predefinito del workspace;
- US-025 — pianificazione tramite ARcipelago;
- US-026 — Template incorporato `fabbrica-del-software`;
- US-027 — azioni del Template calcolate dallo stato della spec.

Il branch `sleli` contiene cinque commit sopra `main`. La suite Go era verde al termine della verifica.

### ARcipelago

Repository: `/Users/stefanoleli/Project/ARcipelago`

Completate:

- US-001 — credenziali applicative limitate al workspace;
- US-002 — creazione idempotente dei task esterni;
- US-003 — recupero di stato e risultato tramite identità esterna;
- US-004 — stream riprendibile degli eventi di una run;
- US-005 — messaggi, approvazioni e cancellazione da client esterno.

Il branch `sleli` contiene cinque commit sopra `main`. Queste API sono generiche e non devono acquisire semantica specifica di ARchetipo.

## Backlog da realizzare

Le nuove storie sono nel backlog ARchetipo. Il backlog ARcipelago non richiede nuove storie per questa roadmap: le capacità remote necessarie sono già presenti nelle US-001–US-005.

### EP-005 — Esperienza operativa ARchetipo

- US-028 — Configurare l'esecuzione e vedere le azioni nella UI.
- US-029 — Pianificare una spec dalla UI.
- US-030 — Interagire con una run remota dalla UI.
- US-031 — Creare una spec dalla UI.

### EP-006 — Esecuzione locale e ciclo di delivery

- US-032 — Pianificare con Codex come provider locale.
- US-033 — Pianificare con Claude come provider locale.
- US-034 — Implementare una spec tramite un provider.
- US-035 — Preparare e decidere la review con un gate umano.

### EP-007 — Archetipi installabili

- US-036 — Installare un Archetipo da un pacchetto.
- US-037 — Inizializzare un workspace con un Archetipo installato.

## Sequenza delle slice

### Slice 1 — UI consapevole del processo

Storia: US-028.

Risultato: dalla UI si configura il provider predefinito e, aprendo una spec, si vedono le azioni ammesse dal suo Archetipo e dal suo stato.

Vincolo: la UI deve consumare i contratti esistenti di configurazione e `spec actions`; non deve duplicare le regole del processo nel frontend.

### Slice 2 — Prima vertical slice di valore

Storia: US-029.

Risultato: da una spec `TODO` l'utente preme `Pianifica`; ARchetipo usa ARcipelago, mostra avanzamento ed esito e al successo presenta piano e stato `PLANNED`.

Questa è la prima release utilizzabile della nuova direzione di prodotto.

### Slice 3 — Collaborazione con la run remota

Storia: US-030.

Risultato: la UI segue eventi senza duplicazioni e permette messaggi, risposte alle approvazioni e cancellazione usando le API esterne già consegnate da ARcipelago.

Vincolo: ARchetipo proietta la run, ARcipelago ne resta il proprietario.

### Slice 4 — Ingresso del lavoro dalla UI

Storia: US-031.

Risultato: l'utente crea una spec `TODO` senza preparare payload CLI e può poi avviarne il processo dalla stessa UI.

Con questa slice il primo MVP UI è completo: creazione, scelta dell'azione, esecuzione remota e interazione avvengono da ARchetipo.

### Slice 5 — Provider locali

Storie: US-032, poi US-033.

Risultato: la stessa pianificazione gestita funziona senza ARcipelago usando prima Codex e poi Claude disponibili localmente.

Vincoli:

- la modalità diretta basata sulle skill deve restare invariata;
- Codex e Claude sono provider distinti dietro lo stesso contratto;
- credenziali e sessioni di autenticazione non vengono copiate nella configurazione ARchetipo.

### Slice 6 — Ciclo di delivery completo

Storie: US-034, poi US-035.

Risultato: un provider può implementare una spec pianificata e preparare il dossier di review. Il verdetto finale resta umano e nessun successo dichiarato dal provider chiude implicitamente la spec.

### Slice 7 — Generalizzazione tramite Archetipi

Storie: US-036, poi US-037.

Risultato: processi diversi da `fabbrica-del-software` possono essere installati come pacchetti e selezionati durante l'inizializzazione di un workspace.

Vincoli:

- nessun marketplace nella prima versione;
- nessuna ereditarietà o composizione di Archetipi;
- un pacchetto invalido non deve lasciare installazioni parziali;
- `fabbrica-del-software` resta il default compatibile.

## Ordine raccomandato

1. US-028
2. US-029
3. US-030
4. US-031
5. US-032
6. US-033
7. US-034
8. US-035
9. US-036
10. US-037

Non lavorare su più storie contemporaneamente salvo test o preparazione strettamente necessari alla storia corrente. Ogni storia deve attraversare pianificazione, implementazione e review prima di iniziare la successiva.

## Regole per la ripresa

1. Operare soltanto sui branch `sleli`, salvo istruzione esplicita differente.
2. Prima di modificare codice, leggere `AGENTS.md`, eseguire `archetipo config show` e leggere il backlog del repository interessato.
3. Usare le skill ARchetipo appropriate: `archetipo-plan`, `archetipo-implement` e `archetipo-review`; usare `archetipo-autopilot` soltanto se richiesto esplicitamente.
4. Verificare sempre lo stato reale con `archetipo spec list` o `archetipo spec show`; non dedurlo da questo documento.
5. Non ricreare o consultare il vecchio branch `codex/autopilot-backlog-20260723`.
6. Non spostare responsabilità di UI o processo in ARcipelago.
7. Non introdurre astrazioni per casi futuri se non sono richieste dai criteri di accettazione della storia corrente.
8. Preservare le modifiche esistenti non correlate e non integrare in `main` senza una richiesta esplicita.

## Prompt per una nuova sessione

```text
Continua lo sviluppo della roadmap di orchestrazione gestita di ARchetipo.

Repository:
- ARchetipo: /Users/stefanoleli/Project/ARchetipo
- ARcipelago: /Users/stefanoleli/Project/ARcipelago

Lavora esclusivamente sui branch `sleli` e non integrare in `main` senza una mia richiesta esplicita. Non consultare né ricreare il vecchio branch `codex/autopilot-backlog-20260723`.

Prima di agire:
1. leggi /Users/stefanoleli/Project/ARchetipo/docs/ROADMAP-MANAGED-ORCHESTRATION.md;
2. controlla branch e working tree di entrambi i repository;
3. esegui `archetipo config show` e `archetipo spec list` in entrambi;
4. leggi gli AGENTS.md applicabili;
5. considera autorevole lo stato restituito dal backlog, non quello riassunto nel prompt.

Confini di prodotto da rispettare:
- ARchetipo possiede UI, workspace, Archetipi, processo, skill e scelta dell'ExecutionProvider.
- ARcipelago possiede dispatch, runner, run remote, eventi, messaggi, approvazioni e cancellazione.
- ARcipelago è un provider di ARchetipo, non un modulo della sua UI.
- ARchetipo deve continuare a funzionare direttamente dentro Codex, Claude e gli altri coding agent supportati.
- Non introdurre un motore BPM/DAG generico: il processo resta definito da stati, azioni e skill.
- Implementa solo ciò che serve alla storia corrente e lascia aperte le estensioni attraverso confini semplici.

Il backlog precedente è completato:
- ARchetipo US-023–US-027: DONE.
- ARcipelago US-001–US-005: DONE.

La prossima storia da lavorare è ARchetipo US-028, `Configurare l'esecuzione e vedere le azioni nella UI`. Usa il processo ARchetipo completo: pianifica US-028, implementala e portala in review. Non iniziare US-029 finché US-028 non è DONE o finché non ti chiedo esplicitamente di continuare. Durante il lavoro usa le skill ARchetipo appropriate e verifica i criteri di accettazione con test proporzionati al rischio.

Alla fine riportami:
- stato della storia;
- funzionalità visibile consegnata;
- test eseguiti e risultato;
- eventuali decisioni o limiti rimasti;
- prossimo elemento raccomandato della roadmap.
```
