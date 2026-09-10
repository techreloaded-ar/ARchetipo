# Accettazione delle sessioni native

Relazione finale del task 12. Revisione di partenza: `c78e37f` sul branch `feature/sessioni-native`. Data delle misure: 2026-09-10.

**Verdetto in una riga: il percorso locale è pronto al rilascio; l'obiettivo completo sui tre provider non è raggiunto, perché i task 10 e 11 non sono stati consegnati.**

Le due cose vanno tenute separate e questo documento le tiene separate ovunque. «Pronto al rilascio locale» significa che una persona, dal browser, apre `archetipo view` su un workspace servito da Claude Code o Codex CLI e ottiene tutto ciò che la direzione di prodotto promette. «Obiettivo completo» comprendeva anche ARcipelago, che resta sul percorso precedente.

---

## 1. Che cosa è stato verificato, e come

Nessun task è stato considerato completo perché esiste il suo file di handoff. Per ciascuno la verifica è stata: leggere il codice corrente al punto che il criterio nomina, poi eseguire una prova che fallisca se quel codice non c'è.

| Task | Criterio essenziale | Verificato su | Esito |
| --- | --- | --- | --- |
| 01 | I contratti nativi sono provati sui runtime installati | `claude/live_probe_test.go`, `codex/live_probe_test.go`, `protocolli.md` | **confermato** — i probe compilano e i due `NativeSessionProvider` passano sul runtime reale (§4) |
| 02 | Esiste un contratto compilabile di sessione durevole | `execution/session.go`, `session_test.go` | **confermato** — il fake attraversa creazione, due turni, interrupt, rilascio e resume da un provider instance nuovo |
| 03 | Conversazione e timeline sono durevoli e riprendibili | `conversationlog/`, `web/native_session.go` | **confermato** — record versionato + `events.jsonl`, lock cross-process, oltre 2.000 eventi paginati |
| 04 | Claude implementa `SessionProvider` | `claude/native_session.go` | **confermato** — `--session-id`/`--resume`, mai `--no-session-persistence` né `--continue` sul percorso conversazione |
| 05 | Codex implementa `SessionProvider` | `codex/native_session.go` | **confermato** — `thread/start` con `ephemeral:false`, `thread/resume`, `model/list`, `skills/list` |
| 06 | View usa sessione e scelte per turno | `web/native_session.go`, `assets/conversation.js` | **confermato** — `model-choice` dalla sessione, `next-turn` validato, steering solo con capability |
| 07 | Le skill offerte sono quelle del runtime | `claude/native_session.go`, `codex/native_session.go` | **confermato** — e misurato dal vivo: 34 skill del runtime reale, `skills_known:true` (§3) |
| 08 | Un'azione lavora nella sessione esistente | `web/session_action.go`, `session_action_test.go` | **confermato** — e misurato dal vivo: `plan` chiude `SUCCEEDED` senza spostare la sessione (§3) |
| 09 | Recovery, filesystem e processi | `web/native_runtime.go`, `recovery_test.go` | **confermato** — e misurato dal vivo: restart di View e memoria nativa conservata (§3) |
| 10 | Hub runner ARcipelago | — | **non consegnato**: nessun esito, nessun codice |
| 11 | Adapter ARcipelago | `arcipelago/provider.go` | **non consegnato**: `SessionProviderFor(arcipelago)` è `false`, ed è un test a impedirne la smentita |

I task 10 e 11 sono dichiarati incompleti come il prompt del task 12 prevede per una consegna solo locale.

---

## 2. Matrice provider × capacità

Le capacità di sessione sono quelle che l'adapter dichiara in `DiscoverSession` e che il runtime conferma; le capability di processo sono quelle di `Provider.Capabilities`. Niente è stato dedotto: una casella «no» è una casella misurata o una capacità che l'adapter non implementa.

| Capacità | `claude` | `codex` | `arcipelago` |
| --- | --- | --- | --- |
| Sessione nativa durevole (`SessionProvider`) | **sì** — `claude-session` | **sì** — `codex.thread` | **no** |
| Riferimento e resume nativi | `--session-id <uuid>` / `--resume` | `thread/start ephemeral:false` / `thread/resume` | n/a |
| `session.resume` | sì | sì | n/a |
| `turn.model` | sì | sì | n/a |
| `turn.options` (effort) | sì | sì | n/a |
| `skill.discovery` | sì, da `system/init` | sì, da `skills/list` | n/a |
| `skills_known` prima del primo turno | **false** — Claude non offre una richiesta di discovery | **true** | n/a |
| Notifica di cambio catalogo | no | sì (`skills/changed`) | n/a |
| `skill.invoke` | sì — `/name` | sì — marker `$name` + input `skill` | n/a |
| `turn.input` (input strutturato) | **no** — il runtime provato non lo espone; le domande arrivano come testo | sì — `item/tool/requestUserInput` | n/a |
| `turn.approval` | sì — `can_use_tool` su stdio | sì — approval command/file | n/a |
| `turn.steering` | sì | sì — `turn/steer` | n/a |
| `turn.interrupt` | sì | sì | n/a |
| Ambiente della sessione | `local` | `local` | remoto (hub) |
| Capability di processo | `spec.plan`, `spec.implement`, `spec.review`, `workspace.inception`, `workspace.backlog`, `workspace.spec-draft` | `spec.plan`, `spec.implement`, `spec.review` | `spec.plan`, `spec.implement` |
| Azione di processo dentro il thread | sì | sì | **no** — run separata, percorso batch invariato |
| Conversazione libera | sì | sì | sì, ma sul `Conversationalist` legacy |

Due asimmetrie meritano di essere lette, perché non sono difetti di ARchetipo:

- **Claude non espone input strutturato** e **non pubblica descrizione, path e scope delle skill**: il catalogo nasce dall'intersezione fra `system/init.skills` e `slash_commands`, quindi porta i nomi e nulla più, e prima del primo `system/init` è dichiarato *non ancora noto* invece di essere completato per ipotesi.
- **Codex non dichiara le capability di workspace** (`inception`, `backlog`, `spec-draft`). Su un workspace il cui default è Codex quei tre pulsanti non sono offerti, ed è corretto: la capability è del provider, non della UI.

### Ciclo di vita di un server di sviluppo

Misurato dai due probe live gemelli (`TestLiveClaudeHTTPServerLifecycle`, `TestLiveCodexHTTPServerLifecycle`), riportato dall'esito 09 e non rimisurato qui perché nulla nel task 12 lo tocca:

| Caso | Claude | Codex |
| --- | --- | --- |
| Server scollegato (`nohup … &`): vivo dopo la fine del turno | **sì** | **no** |
| Server scollegato: vivo dopo l'interrupt di un altro turno | sì | no |
| Server scollegato: vivo dopo il rilascio del runtime | sì | no |
| Server in primo piano: vivo dopo l'interrupt del proprio turno | **no** | **sì** |
| Server in primo piano: vivo dopo il rilascio del runtime | no | no |
| Sessione ripresa e usata dall'interno per agire sul servizio | verificato, e **fermato** | verificato, e **riavviato** |

La conseguenza è la promessa che il prodotto può fare: **non** che un server di sviluppo sopravviva a un riavvio di View, ma che la conversazione torni e che da dentro il servizio si possa verificare, fermare o riavviare.

---

## 3. Prove osservate da View sul runtime reale

Lo scenario è in [`accettazione-live.mjs`](accettazione-live.mjs) e si esegue con `node docs/plans/sessioni-native/accettazione-live.mjs`. Nulla è finto: la CLI è compilata da sorgente, il workspace è un sandbox temporaneo inizializzato con `archetipo init --tool claude`, il viewer è `archetipo view`, il provider è il vero `claude` e l'agente è **Claude Code reale con credenziali reali**. Gli oracoli sono il filesystem, il record durevole su disco, il pid del processo viewer e il modello che il runtime dichiara di avere applicato.

Sei affermazioni, tutte verdi nell'esecuzione registrata:

| # | Affermazione | Oracolo osservato |
| --- | --- | --- |
| AC-1 | Una conversazione libera usa i tool veri sul filesystem dell'host | `PROVA.txt` con `NATIVE_VIEW_OK` è comparso nella radice del workspace — nessuna proposta, nessuna conferma, nessuna execution |
| AC-2 | Le skill offerte sono quelle del runtime della sessione | catalogo di **34** skill con `skills_known:true`, fra cui `archetipo-plan`, `archetipo-implement` e le skill personali dell'utente che nessuna scansione di `.claude/skills` del progetto avrebbe trovato |
| AC-3 | Modello ed effort scelti valgono dal turno successivo | `applied.model` è passato da `claude-haiku-4-5-20251001` a `claude-sonnet-5` — è il *runtime* a dichiararlo, non la richiesta — e `.archetipo/config.yaml` porta ancora `model: haiku` |
| AC-4 | Un'azione di processo lavora nella stessa sessione nativa | `POST /api/spec/US-001/execution {"action":"plan","conversation_id":…}` ha chiuso `SUCCEEDED`, la spec è `PLANNED` con **3 task** e 1.792 caratteri di piano su disco, il riferimento nativo è lo stesso di prima, e la conversazione è rimasta `ACTIVE` |
| AC-5 | La sessione torna dopo il riavvio di View | viewer ucciso e riavviato (**pid diverso**), stessa conversazione, stesso `session.native.id`, e alla domanda «che cosa avevi scritto in PROVA.txt?» l'agente ha risposto senza rileggere il file: la memoria è nativa |
| AC-6 | Rilasciare non distrugge il contesto | `DELETE` ha risposto `CLOSED / RELEASED` e `.archetipo/conversations/<id>.json` è ancora su disco con il suo riferimento nativo |

Osservazioni della corsa registrata: `conversation_id = conv-30743800…`, `native_session_id = 4ee9a1cc-0944-492f-bf1c-82728514d06e`, `viewer_pid = 15198 → 16154`, `capabilities = [session.resume, skill.discovery, skill.invoke, turn.approval, turn.interrupt, turn.model, turn.options, turn.steering]` — si noti l'assenza di `turn.input`, coerente con la matrice.

Le due prove che questo scenario **non** copre — l'interrupt di un turno e il ciclo di vita di un server di sviluppo — sono coperte sul runtime reale da `TestLiveClaudeInterruptThenResume` e dalle due sonde HTTP gemelle, e non sono state duplicate qui.

---

## 4. Verifiche eseguite

Runtime osservati: Go `1.26.4 darwin/arm64`, Node.js `v24.16.0`, golangci-lint `2.13.2`, Claude Code `2.1.267`, Codex CLI `0.146.0`, Google Chrome nel percorso standard macOS.

### Check comuni e conformance

```text
gofmt -l .                              PASS — nessun output
go vet ./...                            PASS
go build ./...                          PASS
go test ./...                           PASS — interamente verde, conformance dei connector inclusa
golangci-lint run --timeout 5m ./...    PASS — 0 issues
go vet -tags liveprobe ./internal/execution/claude/ ./internal/execution/codex/   PASS
```

`go test ./...` è **interamente verde**. I due fallimenti che gli esiti 01–08 riportavano come preesistenti (`TestClaudeWorkspaceWithoutProviderStillPlansThroughTheSkill`, `TestCodexWorkspaceWithoutProviderStillPlansThroughTheSkill`) non erano dovuti alla modifica utente in `.archetipo/config.yaml` ma a un difetto di prodotto — `init` installava la configurazione viva di questo repository al posto del template — corretto in `641e448`. Il config dell'utente non è stato toccato: resta modificato nel working tree esattamente come lo era all'inizio del task.

### Test credential-free

```text
npm run test:web        PASS — 365 test, 0 failure
npm run test:e2e:unit   PASS — 50 test, 0 failure
```

### Smoke di View — tutti e sedici verdi

```text
npm run test:view-page-load-smoke            PASS — 5 affermazioni, zero eccezioni JS
npm run test:view-conversation-smoke         PASS — 7 affermazioni
npm run test:view-conversation-history-smoke PASS — 5 affermazioni
npm run test:view-conversation-multi-smoke   PASS — 6 affermazioni
npm run test:view-session-action-smoke       PASS — 7 affermazioni
npm run test:view-conversation-action-smoke  PASS — 5 affermazioni
npm run test:view-conversation-run-smoke     PASS — 4 affermazioni
npm run test:view-inception-smoke            PASS — 8 affermazioni
npm run test:view-backlog-smoke              PASS — 8 affermazioni
npm run test:view-spec-draft-smoke           PASS — 10 affermazioni
npm run test:view-run-model-smoke            PASS — 7 affermazioni
npm run test:view-workspace-root-smoke       PASS — 4 affermazioni
npm run test:view-workspace-runs-smoke       PASS — 6 affermazioni
npm run test:view-plan-smoke                 PASS
npm run test:view-run-smoke                  PASS
npm run test:view-execution-smoke            PASS
```

**Nessun fallimento preesistente resta.** I tre smoke che gli esiti 08 e 09 registravano come rossi — `conversation-history`, `conversation-action`, `conversation-run` — sono verdi: due erano difetti di prodotto e sono stati corretti, il terzo era un'affermazione del modello precedente ed è stata riscritta sulla semantica nativa.

### E2E live mirati

```text
LIVE_CLAUDE=1 … TestLiveClaudeNativeSessionProvider   PASS — 14,7 s
LIVE_CODEX=1  … TestLiveCodexNativeSessionProvider    PASS — 32,8 s
node docs/plans/sessioni-native/accettazione-live.mjs PASS — 6 affermazioni, Claude Code reale
```

Il probe live Codex è stato eseguito con esito positivo: il workspace, senza crediti al momento del task 07, ne ha di nuovo.

### Prove non eseguite

- **Ogni prova che riguardi ARcipelago come provider di sessioni native.** Non esiste il codice: i task 10 e 11 non sono stati consegnati. Non è una prova saltata, è una capacità assente.
- Le sonde HTTP del ciclo di vita di un server di sviluppo non sono state rimisurate in questo task: l'esito 09 le ha eseguite su questa stessa revisione del codice e nulla qui le tocca. La loro matrice è riportata al §2 come misura di quel task, non di questo.
- **Windows.** Gli smoke che usano i fake POSIX si dichiarano skip su Windows, e nessuno è stato eseguito lì. `GOOS=windows go test -c` compila.

---

## 5. Che cosa è stato eliminato o corretto in questo task

- **Il commento del resume** in `web/conversation_resume.go` diceva ancora che «nulla della sessione originale viene riaperto». Per un record nativo è falso da tre task, e un commento falso in testa alla funzione che decide fra resume nativo e seed legacy è il posto peggiore in cui lasciarlo.
- **Il commento sull'apertura di una conversazione** in `web/conversation.go` diceva che il vocabolario di processo viaggia sulla richiesta «perché l'agente può proporre un'azione». Sul percorso nativo non viaggia affatto, ed è ora scritto perché.
- **`execution.FormatConversationActions`** è stata rimossa: era il renderer del blocco «proponi, non agire» del vecchio prompt, e non aveva più nessun chiamante.
- **La risposta del rilascio di una conversazione nativa** dichiarava `connection: CONNECTED` sulla stessa richiesta che aveva appena rilasciato il runtime. La proiezione è calcolata *prima* del rilascio — deve esserlo, perché dopo il viewer non possiede più la conversazione — e ciò che il rilascio cambia va riscritto a mano, come già si faceva per lo stato. Corretto, con un test che fallisce se la correzione viene tolta.
- **`TestCompletedLocalAdaptersExposeNativeSessions`** copriva soltanto Codex: una regressione che avesse tolto a Claude i metodi di `SessionProvider` lo avrebbe lasciato verde. Ora copre entrambi.
- **Nessun test copriva la compatibilità dei record precedenti.** `TestALegacyRecordOnDiskStaysReadableAndDeclaresItsLimit` scrive a mano un record nella forma che il vecchio viewer scriveva — senza `version`, senza `session`, con gli eventi dentro — e verifica che sia letto intero, che `Native()` resti falso, che leggerlo non lo riscriva e che resti nell'indice.

Non è stato eliminato ciò che non impone il vecchio modello: `ConversationRequest.ProcessActions` resta perché `arcipelago` non ha ancora sessioni native e la forma della sua richiesta è dei task 10 e 11; `conversation_action.go` resta, con il commento che dichiara perché lì «execution = conversazione» è letteralmente vera; e il reader dei dati legacy resta intatto, che è una protezione dei dati e non una restrizione di prodotto.

---

## 6. Documentazione aggiornata

- **Wiki**: nuova pagina [`decisions/native-harness-sessions.md`](../../wiki/decisions/native-harness-sessions.md) con contesto, decisione, alternative scartate, conseguenze e verifica. È in stato `generated` e **non** è stata marcata reviewed: la verifica prevista dal repository non è stata eseguita, e attribuirle uno stato non verificato sarebbe esattamente ciò che il task vieta. `wiki catalog` ha rigenerato indice e log; `wiki validate` risponde `ok: true`.
- **Documentazione utente**: `README.md` e `README.it.md` hanno una sezione nuova sulle conversazioni come sessioni native, con i due limiti dichiarati (sopravvivenza dei job dipendente dall'harness, percorso remoto assente).
- **`AGENTS.md`**: nuova sezione «Native harness sessions» con le cinque identità e le regole che ne discendono; descrizioni di `test:view-conversation-smoke` e `test:view-conversation-multi-smoke` riscritte sul comportamento reale — la prima citava ancora `--no-session-persistence`, la seconda un `maxLiveConversations` che non esiste più; aggiunte le voci mancanti di `conversation-history`, `conversation-action` e `conversation-run`; corretta la nota operativa su `config.template.yaml`.
- **Note di rilascio e migrazione**: [`docs/releases/sessioni-native.md`](../../releases/sessioni-native.md) — versionamento, che cosa cambia per chi usa View, migrazione dei dati esistenti, che cosa non è incluso. Nessun tag, nessun publish, nessun deploy eseguito.

Sulla policy di versionamento: **nessuna operazione CLI pubblica è cambiata in modo incompatibile**, quindi il rilascio è minor e la policy di breaking change non si applica. L'unico cambiamento di layout è interno al packaging (`config.template.yaml`), e il pacchetto npm continua a chiamarlo `runtime/config.yaml`.

---

## 7. Pulizia, compatibilità e config dell'utente

- **Risorse**: nessun processo `archetipo view`, `fake-claude` o `fake-codex` è rimasto vivo al termine. Tutti gli smoke sono stati eseguiti con `--cleanup` e non hanno lasciato directory di run; le directory più vecchie sotto `test/workspaces/` sono preesistenti e ignorate da Git. Lo scenario live usa un sandbox in `os.tmpdir()` e lo rimuove, salvo `--keep`.
- **Record precedenti**: leggibili, non riscritti e non migrati, ora con un test che lo prova.
- **Config dell'utente**: `.archetipo/config.yaml` non è stato toccato. Resta modificato nel working tree com'era all'inizio (mtime 3 settembre) e non entra nel commit. Le modifiche preesistenti dell'utente nei mockup e nel probe console sono state preservate.

---

## 8. Limiti residui

1. **Il percorso remoto non esiste.** `arcipelago` non implementa `SessionProvider`: conserva `Conversationalist` e il percorso run batch. È il confine dell'obiettivo completo, ed è tutto ciò che manca per raggiungerlo.
2. **La proposta d'azione è raggiungibile ma non insegnata.** La rotta, la card, il record e il vincolo di processo esistono e sono provati dallo smoke; ma il prompt di una conversazione libera nativa non contiene più la grammatica della riga JSON, perché insegnarla apparteneva al modello read-only in cui proporre era l'unica cosa che l'agente potesse fare. Un agente reale non la emette spontaneamente. Riabilitarla è una scelta di prodotto — dire all'agente il vocabolario in una sessione che può anche agire — e non è stata fatta qui.
3. **Una conversazione nativa non riceve nessun prompt di apertura ARchetipo.** L'harness legge da sé le istruzioni del workspace, il che è coerente con «il contesto è dell'harness»; ma significa che il contesto ARchetipo di una conversazione libera è quello che l'harness trova sul filesystem, non quello che ARchetipo gli dice.
4. **Il possesso si basa sul pid.** Due viewer sulla stessa macchina sono coperti; due macchine che montano lo stesso workspace via rete no, come già non lo erano per il journal.
5. **Una consegna `UNCERTAIN` resta bloccante** quando la sessione non può essere interrogata. È deliberato: nessun effetto viene rilanciato.
6. **Un viewer che non possiede una conversazione non lo mostra prima che la persona scriva.** Il rifiuto è chiaro al primo messaggio; sondare il possesso a ogni GET costerebbe un lock sul percorso che il browser interroga di continuo.
7. **`workspaceSession.stop` ha un budget di 5 secondi per conversazione.** Un provider che non risponde entro quel budget lascia il proprio processo vivo e lo shutdown prosegue.
