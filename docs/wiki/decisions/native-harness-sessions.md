---
type: decision
title: Le conversazioni di View sono sessioni native dell'harness
description: Una conversazione del viewer è una sessione durevole del vero harness di sviluppo, con contesto nativo, scelte per turno e azioni di processo nello stesso thread
status: generated
decision_status: accepted
sources:
    - path: cli/internal/execution/session.go
      role: implementation
    - path: cli/internal/execution/session_test.go
      role: verification
    - path: cli/internal/execution/claude/native_session.go
      role: implementation
    - path: cli/internal/execution/codex/native_session.go
      role: implementation
    - path: cli/internal/conversationlog/record.go
      role: implementation
    - path: cli/internal/conversationlog/file_store.go
      role: implementation
    - path: cli/internal/web/native_session.go
      role: implementation
    - path: cli/internal/web/native_runtime.go
      role: implementation
    - path: cli/internal/web/session_action.go
      role: implementation
    - path: cli/internal/web/session_action_test.go
      role: verification
    - path: cli/internal/web/recovery_test.go
      role: verification
    - path: cli/internal/web/conversation_resume.go
      role: implementation
    - path: test/e2e/session-action-view-smoke.mjs
      role: verification
    - path: test/e2e/conversation-view-smoke.mjs
      role: verification
---
# Le conversazioni di View sono sessioni native dell'harness

<!-- archetipo:wiki section=context -->
## Contesto

La prima versione delle conversazioni di `archetipo view` era un dispatch travestito da chat. Il processo dell'agente veniva aperto con `--no-session-persistence` per Claude e `ephemeral:true` per Codex, quindi il contesto moriva con il processo; un timeout globale chiudeva la conversazione; il prompt vietava all'agente di agire e gli chiedeva soltanto di *proporre* un passo del processo; e un'azione ARchetipo era un secondo agente avviato accanto al thread, la cui ricevuta terminava insieme l'esecuzione e la conversazione.

Ognuna di queste scelte aveva una ragione locale, e tutte insieme producevano un prodotto che non era ciò che serve: una persona che lavora in View vuole parlare con lo stesso agente per giorni, cambiare modello ed effort quando serve, usare gli strumenti e le skill che l'harness ha davvero, e vedere una skill di processo lavorare nel thread da cui l'ha invocata. Un contesto che scade e un agente che può soltanto proporre non lo permettono.

Il [confine dei provider di esecuzione](execution-provider-boundary.md) non cambia: resta il contratto del percorso batch e dei record execution, e le sessioni native gli si affiancano invece di sostituirlo. Le azioni che una conversazione può portare avanti restano quelle del [Template di processo del workspace](workspace-process-template.md).

Il vincolo tecnico che rendeva possibile il cambiamento è arrivato dagli harness stessi: Claude Code espone `--session-id`/`--resume` su stream JSON, Codex CLI espone `thread/start`/`thread/resume` con `model/list` e `skills/list` sull'app-server. Il contesto persistente è dell'harness, non di ARchetipo, e può essere ripreso per riferimento.

<!-- archetipo:wiki section=decision -->
## Decisione

Una conversazione di View **è** una sessione nativa dell'harness. `execution.SessionProvider` è il confine provider-neutro che la descrive, e distingue cinque identità che prima erano confuse:

- la **conversazione**, identità di ARchetipo, il cui id non cambia mai;
- la **sessione nativa**, identità dell'harness (`claude-session`, `codex.thread`), conservata in `session.native`;
- la **connection**, cioè il processo runtime attaccato in questo momento;
- il **turno**, che porta il proprio modello, le proprie opzioni e — quando l'ha avviato un'azione — il proprio `execution_id`;
- la **execution**, cioè il record di un'azione di processo.

Una conversazione può contenere molte execution; una execution può attraversare più turni. Rilasciare la connection non chiude la conversazione, interrompere un turno non elimina la sessione, e la ricevuta di un'azione non termina il thread.

I fatti che ne discendono, tutti implementati:

- **Nessuna scadenza.** Non esiste timeout globale né di inattività per una conversazione. `timeout_seconds` resta un limite del percorso batch, che conserva il proprio lifecycle single-turn e non deve essere usato come implementazione delle conversazioni.
- **Il resume è nativo.** Un record che possiede un riferimento nativo viene ripreso con `--resume <uuid>` o `thread/resume`, senza reinviare timeline né riassunti. Il vecchio resume da transcript sopravvive soltanto per i record legacy, che non hanno mai avuto un riferimento nativo: è un gesto esplicito, dichiarato nella risposta HTTP e nell'indice, e non il resume normale.
- **La conversazione può agire.** Il prompt non vieta più modifiche, tool, skill o comandi in scrittura e non impone il passaggio proposta/conferma. Le policy di permesso restano quelle native dell'harness, e le sue richieste arrivano alla persona come approval del pannello.
- **Un'azione lavora nel thread.** `POST /api/spec/{code}/execution` e `POST /api/workspace/execution` accettano un `conversation_id` e, per un provider con sessioni native, avviano un turno nella sessione esistente invece di generare un secondo agente. Il record execution conserva identità, verifica degli effetti e rollback; la conversazione resta aperta e scrivibile durante e dopo.
- **Le skill vengono dal runtime.** La discovery è quella dell'harness — `skills/list` per Codex, `system/init` per Claude — e non una scansione cumulativa delle directory dei provider. Un catalogo non ancora annunciato è dichiarato sconosciuto, non completato per ipotesi.
- **Le scelte sono del turno.** Modello e opzioni scelti durante un turno valgono dal successivo, sono validati contro il catalogo della sessione e non spostano il default del workspace; il default del workspace, a sua volta, non sposta una sessione già aperta.
- **Il possesso è per conversazione.** Un solo processo View alla volta possiede il runtime di una conversazione, attraverso il lock di conversazione che il journal già usava. Un secondo viewer la legge normalmente e viene rifiutato soltanto quando prova a scriverci.
- **Cancellare riguarda i dati ARchetipo.** `DELETE /api/workspace/conversations/{id}` rilascia il runtime; `DELETE .../{id}/record` elimina metadati e timeline di ARchetipo e risponde `native_session_preserved: true`. Nessuno dei due distrugge il transcript nativo del vendor o i file di lavoro.

La durabilità è su filesystem e non in memoria: `.archetipo/conversations/<id>.json` porta il record versionato, `<id>.events.jsonl` la timeline append-only paginabile, e ogni comando è scritto `UNSENT`, attraversa il confine di crash come `UNCERTAIN` e diventa `CONFIRMED` soltanto dalla risposta o dall'osservazione dello stesso turno. Una consegna incerta non viene mai ritentata automaticamente.

<!-- archetipo:wiki section=alternatives -->
## Alternative considerate

**Conservare il modello read-only con proposta e conferma obbligatorie.** Garantiva che una conversazione non producesse mai effetti non tracciati, ma al prezzo di un prodotto che non fa ciò che serve: nessuna skill di processo può lavorare nel thread se il thread non può agire. La garanzia che restava utile — un'azione ha un record — è stata conservata dove conta, cioè sui controlli di processo, senza estenderla a ogni gesto autonomo del modello.

**Un riassunto ARchetipo al posto del resume nativo.** Sarebbe stato indipendente dall'harness, ma è esattamente ciò che il prodotto non vuole: un riassunto è una perdita di contesto mascherata da continuità. Il contesto e la compaction restano dell'harness; ARchetipo conserva riferimenti e timeline.

**Un bridge universale che intercetti ogni skill scelta dal modello per produrne un record execution.** Avrebbe conservato l'equivalenza storica «ogni azione è una execution» al costo di un livello di intercettazione fragile per ogni harness. È stata scartata: la board rilegge lo stato reale del workspace anche dopo modifiche prodotte da skill native, che è la stessa garanzia ottenuta senza il livello.

**Un process manager di ARchetipo per i job avviati dai tool.** Avrebbe promesso che un server di sviluppo sopravvive a un riavvio di View. Non è stato introdotto: la sopravvivenza di un job dipende dall'harness e i due misurati non si comportano allo stesso modo. Ciò che viene promesso è che la conversazione torni e che da lì il servizio si possa verificare, fermare o riavviare.

**Un blocco globale o worktree obbligatorie per più conversazioni sullo stesso workspace.** Scartate perché gli harness non lo fanno: viene serializzato il possesso del runtime e i comandi della stessa sessione, non l'accesso al workspace.

<!-- archetipo:wiki section=consequences -->
## Conseguenze

Positive: il contesto sopravvive al riavvio di View; una persona può cambiare modello ed effort a metà lavoro senza toccare la configurazione del workspace; le skill offerte sono quelle che il runtime ha davvero; le azioni di processo e la conversazione libera sono lo stesso thread, quindi non c'è più un secondo agente da seguire in un pannello separato.

Costi e limiti, tutti espliciti:

- La **sopravvivenza di un job nativo dipende dall'harness**. Claude lascia vivere ciò che il comando ha scollegato (`nohup … &`) e uccide ciò che il turno tiene in primo piano; Codex fa l'opposto sull'interrupt e non lascia affatto in vita ciò che viene messo in background. Il rilascio del runtime uccide in entrambi i casi i processi in primo piano. Un job che sopravvive lo fa fuori da ogni contabilità di ARchetipo.
- Il **possesso si basa sul pid**, come tutti i lock di conversazione: vale per due viewer sulla stessa macchina, non per due macchine che montano lo stesso workspace via rete.
- **Claude non espone input strutturato** sullo stream provato: le sue domande arrivano come testo. La capability `turn.input` è dichiarata soltanto da Codex.
- **Claude non pubblica descrizione, path e scope delle skill**: il catalogo arriva dall'intersezione fra `system/init.skills` e `slash_commands`, e prima del primo `system/init` è dichiarato non ancora noto.
- Il percorso **remoto non è consegnato**: `arcipelago` non implementa `SessionProvider` e conserva il `Conversationalist` legacy e il percorso run batch. Un runner remoto non ha implicitamente i file o il `localhost` del browser.
- La **proposta d'azione** resta raggiungibile — la rotta, la card e il record esistono e sono provati — ma il prompt della conversazione libera non insegna più all'agente la grammatica della riga JSON, perché insegnargliela apparteneva al modello read-only. Un agente reale non la emette spontaneamente.
- I **dati precedenti** restano leggibili e non vengono distrutti. Dove manca una sessione nativa il limite è dichiarato dall'API e dall'indice.

<!-- archetipo:wiki section=verification -->
## Verifica

Il contratto e le sue transizioni sono provati senza credenziali dal fake di [session_test.go](../../../cli/internal/execution/session_test.go), che attraversa creazione, due turni, interrupt, rilascio e resume dello stesso riferimento nativo da un provider instance nuovo, e verifica che entrambi gli adapter locali espongano `SessionProvider` mentre `arcipelago` non lo fa.

La durabilità, il possesso e il recovery sono in [recovery_test.go](../../../cli/internal/web/recovery_test.go): inattività prolungata, restart con un'azione in volo, restart senza il provider originale, secondo viewer sullo stesso workspace, hold di un viewer morto, crash fra sottomissione e conferma, crash durante l'append del journal, working directory sparita e assenza di credenziali nell'API e sul disco. L'azione nella sessione è in [session_action_test.go](../../../cli/internal/web/session_action_test.go): record distinti, secondo avvio rifiutato con 409, ricevuta correlata all'azione corrente e non a una precedente, turno interrotto che chiude l'azione come fallita.

Sul percorso browser, [session-action-view-smoke.mjs](../../../test/e2e/session-action-view-smoke.mjs) prova che `plan` e `implement` si susseguono in una sola conversazione, sulla stessa sessione nativa, con un cambio di modello in mezzo e un messaggio libero dopo; [conversation-view-smoke.mjs](../../../test/e2e/conversation-view-smoke.mjs) prova l'invocazione streaming senza `--no-session-persistence`, il thread persistente di Codex attraverso due `turn/start`, e che la conversazione non è una execution.

Sul runtime reale, i probe `TestLiveClaudeNativeSessionProvider` e `TestLiveCodexNativeSessionProvider` (build tag `liveprobe`, variabili `LIVE_CLAUDE`/`LIVE_CODEX`) provano scrittura via tool, rilascio, resume dello stesso identificativo e memoria nativa; `TestLiveClaudeHTTPServerLifecycle` e `TestLiveCodexHTTPServerLifecycle` misurano il ciclo di vita di un server di sviluppo avviato dall'agente. La matrice completa delle misure è in `docs/plans/sessioni-native/accettazione.md`.
