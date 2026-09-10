# Contratto delle sessioni native

Stato: contratto backend introdotto dal task 02. I tipi autorevoli sono in
`cli/internal/execution/session.go`; questo documento ne descrive l'uso e il
mapping verso i protocolli provati in `protocolli.md`.

Il contratto non contiene regole delle skill o del processo ARchetipo. Una
sessione può esistere senza spec ed execution; un turn può collegarsi a una
execution tramite `execution_id`, ma le due identità e i due lifecycle restano
distinti.

## Identità e configurazione durevole

`SessionMetadata` è il dato da persistere per riprendere una conversazione:

```json
{
  "conversation_id": "conversation-01J...",
  "provider_id": "codex",
  "environment": {
    "working_dir": "/workspace/ARchetipo",
    "provider_config": {
      "command": "codex",
      "sandbox": "workspace-write"
    },
    "location": "local"
  },
  "native": {
    "kind": "codex.thread",
    "id": "019c..."
  }
}
```

`conversation_id` appartiene a View. `native.id` appartiene al provider e non
coincide né con il conversation ID né con un execution ID. `native.attributes`
può conservare ulteriori riferimenti non segreti necessari a un provider
remoto, per esempio task e runner; non contiene credenziali.

`environment` congela provider config non segreta, cwd e luogo di esecuzione
della sessione. Cambiare il default del workspace non riscrive questi dati. Le
credenziali restano nelle environment variable nominate dalla configurazione.

## Lifecycle separati

`SessionSnapshot` espone dimensioni indipendenti:

- `archive`: `OPEN` o `ARCHIVED`; non dice nulla sul runtime;
- `connection`: `CONNECTED`, `DISCONNECTED` o `RELEASED`; identifica
  l'attachment corrente anche con `connection_id`;
- `recovery`: `RESUMABLE` o `UNRECOVERABLE`; una disconnessione è recuperabile
  soltanto quando questo campo vale `RESUMABLE`;
- `work`: `IDLE`, `TURN_ACTIVE`, `WAITING_INPUT` o `WAITING_APPROVAL`;
- `current_turn.state`: stato del turn osservato, inclusi `COMPLETED`,
  `INTERRUPTED` e `FAILED`.

Non esiste un enum complessivo: archiviare non interrompe, perdere la connection
non conclude il turn per inferenza e interrompere un turn non elimina la
sessione. `ReleaseSession` rilascia il runtime corrente e conserva il riferimento
nativo. Una sessione `UNRECOVERABLE` resta descrivibile ma non viene ricostruita
da un estratto della timeline.

## Operazioni

Un provider che implementa `SessionProvider` offre un solo confine per:

1. `DiscoverSession`: capability, model, skill e ambiente effettivi;
2. `CreateSession`: creazione del riferimento nativo durevole;
3. `ResumeSession`: nuova connection sul riferimento esistente;
4. `ReadSession`: stato osservato, input e approval pendenti;
5. `StartTurn`: nuovo turn con model, options e skill richiesti;
6. `StreamSessionEvents`: timeline tramite `RunEvent` e cursore monotono;
7. `SteerTurn`, `RespondSessionInput`, `RespondSessionApproval` e
   `InterruptTurn`: comandi correlati al turn attivo;
8. `ReleaseSession`: rilascio della connection, non cancellazione del contesto.

L'interfaccia resta opzionale rispetto a `Provider`. `SessionProviderFor`
restituisce `false` per gli adapter non ancora migrati; quindi Claude, Codex e
ARcipelago correnti non pubblicano capability di sessione native soltanto perché
possiedono un vecchio percorso di conversation o run.

Le capability scoperte sono granulari: `session.resume`, `turn.model`,
`turn.options`, `skill.discovery`, `skill.invoke`, `turn.input`,
`turn.approval`, `turn.steering` e `turn.interrupt`. Creazione, lettura eventi e
rilascio costituiscono il contratto base. Il provider dichiara solo ciò che il
runtime e l'ambiente correnti supportano; in particolare ARcipelago non riceve
capability per analogia con i provider locali.

Esempio di discovery locale Codex, quando l'adapter sarà implementato:

```json
{
  "capabilities": [
    "session.resume",
    "skill.discovery",
    "skill.invoke",
    "turn.approval",
    "turn.input",
    "turn.interrupt",
    "turn.model",
    "turn.options",
    "turn.steering"
  ],
  "models": [{"id": "gpt-5.6-sol", "default": true}],
  "skills": [{
    "name": "archetipo-plan",
    "path": "/workspace/.agents/skills/archetipo-plan/SKILL.md"
  }],
  "environment": {
    "working_dir": "/workspace",
    "location": "local"
  }
}
```

Per Claude, `turn.input` resta assente finché lo stream effettivo non espone una
richiesta strutturata. Le domande testuali restano eventi di testo e non vengono
promosse artificialmente a `PendingInput`.

## Turn, invii ed eventi

Il core assegna `turn_id` e un nuovo `submission_id` a ogni invio. Il provider
può aggiungere `native_id` al turn e `native_delivery_id` alla consegna. Un turn
può nominare facoltativamente una execution gestita:

```json
{
  "session": {"conversation_id": "conversation-01J...", "provider_id": "codex"},
  "turn_id": "turn-01J...",
  "submission_id": "submission-01J...",
  "execution_id": "execution-01J...",
  "message": "$archetipo-plan US-001",
  "model": "gpt-5.6-terra",
  "options": {"effort": "high"},
  "skills": [{
    "name": "archetipo-plan",
    "path": "/workspace/.agents/skills/archetipo-plan/SKILL.md"
  }]
}
```

`SessionTurn.requested` conserva la scelta inviata; `applied` conserva quella
accettata e riportata dal runtime. Non si aggiorna `applied` in anticipo. Un
cambio selezionato durante un turn attivo appartiene al successivo; lo steering
usa invece `SteerTurn` sul turn corrente.

`SessionDelivery.state` vale:

- `UNSENT`: il provider sa che l'invio non ha attraversato il confine; un retry
  esplicito può usare una nuova decisione del core;
- `CONFIRMED`: il protocollo ha correlato la consegna;
- `UNCERTAIN`: il processo può aver ricevuto l'invio, ma manca conferma. Il core
  persiste la correlazione e riconcilia la sessione prima di qualsiasi nuova
  azione; non reinvia automaticamente un comando che può aver prodotto effetti.

I metodi di comando restituiscono `SessionDelivery` anche insieme a un error:
dopo un errore di trasporto il caller deve leggere `state`, non dedurre che
l'invio sia `UNSENT`. Steering usa `message`; le risposte alle approval usano
`interaction_id` e `option_id`; le risposte strutturate usano `interaction_id`
e `payload`.

`RunEvent` resta l'evento comune di timeline e aggiunge i campi opzionali
`turn_id` e `submission_id`. Il suo `id` monotono resta l'unico cursore. I campi
nuovi correlano eventi, turn e invii senza trasformare `seq` in un identificatore
e senza far coincidere la timeline con una execution.

`PendingApproval` è riusato dal ponte dei run quando il significato coincide.
`PendingInput.payload` preserva invece la richiesta strutturata del provider:
il core non inventa uno schema comune più ricco del protocollo disponibile.

## Mapping sui protocolli provati

| Contratto ARchetipo | Codex app-server | Claude stream JSON |
| --- | --- | --- |
| `NativeSessionReference` | `thread.id`, kind `codex.thread` | `session_id`, kind `claude.session` |
| `CreateSession` | `thread/start` con `ephemeral:false` | processo con `--session-id`, senza `--no-session-persistence` |
| `ResumeSession` | `thread/resume {threadId}` dopo handshake | nuovo processo con `--resume <session_id>` |
| `StartTurn` | `turn/start`, con `model`, `effort` e input `skill` | frame `user`; se model/effort cambiano, reopen con `--resume` e relativi flag |
| `SteerTurn` | `turn/steer` soltanto con turn attivo | frame `user` sullo stream attivo |
| `InterruptTurn` | `turn/interrupt` | `control_request` di interrupt |
| `PendingApproval` | server request `item/commandExecution/requestApproval` | ponte `can_use_tool`/control request |
| `PendingInput` | `item/tool/requestUserInput` | capability non dichiarata nella versione provata |
| `DiscoverSession` | `model/list`, `skills/list {cwds, forceReload}` | `system/init` per model, tools e skills; effort richiesto conservato dal core |
| `RunEvent` | notification e item del turn tradotti | frame `assistant`, `user`, tool e `result` tradotti |
| `ReleaseSession` | chiusura dell'app-server, thread persistito | chiusura del processo, sessione persistita |

Per Codex gli override di model/effort sono sticky nel thread, ma ogni
`SessionTurn` registra comunque richiesto e applicato. Per Claude il cambio
richiede il reopen provato dello stesso `session_id`: è una nuova connection,
non una nuova conversazione.

Il vecchio `Conversationalist` rimane temporaneamente per leggere e servire le
conversation legacy durante la migrazione dei task successivi. È marcato come
legacy e il suo `Context` è descritto come seed parziale, mai come resume nativo;
non è un secondo contratto destinato a nuovi adapter.
