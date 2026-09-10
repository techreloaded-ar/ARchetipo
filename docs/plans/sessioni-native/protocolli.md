# Protocolli nativi Claude e Codex

Data delle prove: 2026-09-08. Scope: task 01, sola discovery; nessun refactoring di View.

## Baseline e fonti

- Claude Code installato: `2.1.263`.
- Codex installato: `codex-cli 0.146.0`.
- Claude documenta `--session-id`, `--resume`, stream JSON, model, permission host e il fatto che `--no-session-persistence` impedisce il resume nella [CLI reference ufficiale](https://code.claude.com/docs/en/cli-usage). Le skill di progetto vivono in `.claude/skills/<name>/SKILL.md`, sono invocabili come `/name` e `${CLAUDE_EFFORT}` espone l'effort effettivo secondo [Extend Claude with skills](https://code.claude.com/docs/en/slash-commands).
- Codex documenta handshake, `thread/start`, `thread/resume`, `turn/start`, override sticky per turno, approval, input e skill nell'[App Server ufficiale](https://developers.openai.com/codex/app-server/). Lo schema locale è rigenerabile con `codex app-server generate-json-schema --experimental --out <directory>`.

Le fonti descrivono il contratto disponibile; la matrice seguente marca `provata` solo una capability osservata sui binari installati. I fake non sono usati come prova vendor.

## Contratto da usare

### Claude

Il riferimento persistente è il `session_id` UUID. Un processo viene aperto con `--session-id <uuid>` e senza `--no-session-persistence`; dopo il rilascio del processo si riapre con `--resume <stesso-uuid>`. Il protocollo del processo resta NDJSON su stdin/stdout con:

1. frame `user` in ingresso;
2. frame `system/init`, che riporta `session_id`, `model`, `tools`, `skills` e `slash_commands` effettivi;
3. frame `assistant`/`user` durante il turno;
4. frame `result` a chiusura del turno;
5. `control_request`/`control_response` per permission e interrupt.

Il model e l'effort sono flag di avvio/resume del processo. Il model è riportato in `system/init` e nei frame `assistant`. L'effort non è un campo di `system/init`: la prova effettiva usa la sostituzione documentata `${CLAUDE_EFFORT}` dentro una skill. Cambiarli significa quindi riaprire lo stesso `session_id`; non va simulato con un prompt. Il fallback ammesso quando lo stream termina è un nuovo processo con `--resume`, mai un estratto della timeline.

Il comando attuale di ARchetipo include `--no-session-persistence`: è incompatibile con questo contratto e dovrà essere rimosso dal percorso conversazionale nel task 04, mantenendo separato l'eventuale batch non persistente.

### Codex

Il riferimento persistente è `thread.id`. Ogni connessione app-server esegue `initialize`, notifica `initialized`, quindi usa `thread/start` con `ephemeral:false` oppure `thread/resume {threadId}`. I turni successivi usano `turn/start` sullo stesso ID. `model/list` è il catalogo reale; `turn/start.model` e `turn/start.effort` cambiano il turno e diventano default sticky dei successivi. Una successiva `thread/resume` riporta `model` e `reasoningEffort`, quindi entrambi sono osservabili senza inferenze.

Le skill si ottengono con `skills/list {cwds, forceReload}`. Per invocarle, il contratto preferito include sia il marker `$name` nel testo sia l'input `{type:"skill", name, path}`; il solo marker funziona ma delega la risoluzione al model ed è più lento.

L'app-server corrente di ARchetipo apre `ephemeral:true`, passa l'effort come `thread/start.config.model_reasoning_effort`, usa cataloghi statici e declina ogni server request. Queste sono limitazioni dell'adapter attuale, non del protocollo vendor; i task 02/05 dovranno sostituirle con `ephemeral:false`, `model/list`, override per turno e routing delle server request.

## Matrice delle capability

| Capability | Claude 2.1.263 | Codex 0.146.0 | Evidenza / limite |
|---|---|---|---|
| Due turni nello stesso runtime | provata | provata | `TestLiveClaudeMultiTurn`; `TestLiveCodexDialogue` dopo interrupt apre un nuovo turno. |
| Rilascio runtime e resume stesso ID | provata | provata | I nuovi live probe riaprono un secondo processo e ricordano una stringa casuale non ripetuta nel prompt. |
| Model tra turni | provata | provata | Claude `sonnet -> opus`, riportato da `system/init`; Codex `gpt-5.6-sol -> gpt-5.6-terra`, riportato da start/resume. |
| Effort tra turni | provata | provata | Claude `low -> high` osservato tramite `${CLAUDE_EFFORT}` nella skill; Codex `low -> ultra` riportato da `thread/resume.reasoningEffort`. |
| Interrupt e nuovo turno | provata | provata | Claude: interrupt consegnato mentre un tool controllato era attivo, nuovo turno sullo stesso stream e successivo `--resume` dello stesso ID. Codex: `turn/interrupt`, `turn/completed`, nuovo `turn/start`. |
| Approval | provata | provata | Claude: richiesta Bash, allow e deny sullo stesso stream. Codex: server request reale `item/commandExecution/requestApproval`, con `accept` che scrive il file e `decline` che non lo scrive. |
| Richiesta input strutturata | assente sullo stream provato | provata | Claude con `--brief` non ha esposto `SendUserMessage` né una server request equivalente: il model ha posto testo libero. Codex, inizializzato con `capabilities.experimentalApi:true`, ha emesso `item/tool/requestUserInput` e ha ricevuto la risposta correlata all'ID JSON-RPC. |
| Catalogo skill reale | provata | provata | Claude `system/init.skills`; Codex `skills/list forceReload`. |
| Invocazione skill fixture | provata | provata | `/native-protocol-probe` e `$native-protocol-probe` hanno scritto `native-skill-marker.txt`. |
| Scrittura filesystem host | provata | provata | La skill fixture ha scritto nel `cwd` temporaneo per entrambi. |
| Server HTTP locale durante/dopo turno | provata | provata | Claude ha servito `HTTP_CLAUDE_OK` durante il turno e dopo la fine del batch. Codex ha servito `HTTP_CODEX_OK` durante il turn e dopo l'interrupt/`turn/completed`; la risorsa non era più raggiungibile dopo il rilascio dell'app-server. Entrambi i probe fermano esplicitamente l'eventuale PID residuo. |
| Continuità di un processo tool dopo rilascio runtime | provata | assente | Il server Claude era ancora vivo dopo la fine del processo batch. Con Codex il server sopravvive alla fine del turn, ma viene terminato dal rilascio dell'app-server. |
| Discovery ARcipelago | non verificata | non verificata | Nessuna prova eseguita; nessuna capability presunta. |

## Probe riproducibili

I test sono esclusi dalla suite ordinaria tramite build tag `liveprobe`, hanno `context.WithTimeout` e richiedono opt-in esplicito:

```bash
cd cli

LIVE_CLAUDE=1 go test -tags liveprobe \
  -run TestLiveClaudeResumeWithModelAndSkill -v -count=1 -timeout 10m \
  ./internal/execution/claude

LIVE_CODEX=1 go test -tags liveprobe \
  -run TestLiveCodexResumeWithModelEffortAndSkill -v -count=1 -timeout 10m \
  ./internal/execution/codex

LIVE_CLAUDE=1 go test -tags liveprobe \
  -run 'TestLiveClaude(Dialogue|MultiTurn|AsksAndAcceptsAPermissionDecision)$' \
  -v -count=1 -timeout 15m ./internal/execution/claude

LIVE_CODEX=1 go test -tags liveprobe \
  -run TestLiveCodexDialogue -v -count=1 -timeout 5m \
  ./internal/execution/codex

LIVE_CLAUDE=1 go test -tags liveprobe \
  -run TestLiveClaudeInterruptThenResume -v -count=1 -timeout 10m \
  ./internal/execution/claude

LIVE_CODEX=1 go test -tags liveprobe \
  -run 'TestLiveCodex(ApprovalAndUserInput|HTTPServerLifecycle)$' \
  -v -count=1 -timeout 10m ./internal/execution/codex
```

I probe creano solo sandbox temporanee e contenuti casuali di fixture. Non stampano credenziali né conservano transcript personali. I transcript vendor prodotti dalla persistenza contengono esclusivamente i prompt di fixture.

## Scambi minimi

Codex persistente:

```json
{"id":1,"method":"initialize","params":{"clientInfo":{"name":"archetipo","version":"1"}}}
{"method":"initialized","params":{}}
{"id":2,"method":"thread/start","params":{"cwd":"/workspace","ephemeral":false}}
{"id":3,"method":"turn/start","params":{"threadId":"<id>","input":[{"type":"text","text":"..."}],"model":"<model>","effort":"medium"}}
```

Dopo restart:

```json
{"id":1,"method":"initialize","params":{"clientInfo":{"name":"archetipo","version":"1"}}}
{"method":"initialized","params":{}}
{"id":2,"method":"thread/resume","params":{"threadId":"<stesso-id>"}}
{"id":3,"method":"turn/start","params":{"threadId":"<stesso-id>","input":[{"type":"text","text":"..."}]}}
```

Claude persistente:

```text
claude -p --input-format stream-json --output-format stream-json --verbose \
  --replay-user-messages --session-id <uuid> --model sonnet --effort low

claude -p --input-format stream-json --output-format stream-json --verbose \
  --replay-user-messages --resume <stesso-uuid> --model opus --effort high
```

Non aggiungere `--no-session-persistence` al percorso conversazionale.

## Limiti osservati

- Claude non ha esposto una richiesta d'input strutturata sullo stream provato: finché una versione o modalità documentata non la rende osservabile, View deve trattare le domande testuali come testo e non inventare un contratto equivalente.
- Con Codex, un processo tool foreground resta vivo dopo `turn/completed`, ma viene terminato quando si chiude l'app-server; il lifecycle della sessione deve quindi possedere esplicitamente quello del runtime.
- L'app-server corrente di ARchetipo declina le server request automaticamente. Il probe intercetta il protocollo reale solo nel test; il routing applicativo resta lavoro dei task successivi.
- ARcipelago non è stato provato e nessuna capability gli viene attribuita.
