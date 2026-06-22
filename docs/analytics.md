# ARchetipo — Contratto Analytics

**Versione:** 1.1
**Schema evento:** `archetipo.analytics/v1`
**Ultima modifica:** 2026-06-22

---

## 1. Finalità della telemetria

ARchetipo raccoglie telemetria anonima con i seguenti obiettivi:

- **Migliorare il prodotto:** capire quali comandi vengono usati più spesso, quali falliscono e con quali codici di errore, per dare priorità a fix e nuove funzionalità.
- **Misurare l'affidabilità:** tracciare tassi di successo/fallimento per comando, durata media e sistema operativo/architettura, per rilevare regressioni.
- **Supportare decisioni di design:** dati aggregati su connector più usati, piattaforme prevalenti e flussi tipici informano la roadmap senza compromettere la privacy.

### Principi non negoziabili

| Principio | Descrizione |
|---|---|
| **Trasparenza** | Ogni evento inviato è documentato qui. L'utente può ispezionare il payload prima dell'invio. Nessun campo nascosto. |
| **Raccolta minima** | Inviamo solo i campi necessari agli obiettivi sopra. Qualsiasi campo non elencato nella tabella campi ammessi non può essere aggiunto senza revisione privacy. |
| **Privacy-preserving** | Nessun dato personale, nessun percorso filesystem, nessun identificatore di repository. L'identificatore di installazione è anonimo e generato localmente. |
| **Consenso esplicito** | La telemetria è **opt-in**. Nessun evento viene inviato senza consenso attivo dell'utente. |
| **Nessun tracciamento** | Non tracciamo utenti nel tempo. `anonymous_installation_id` è l'unico campo di correlazione e non è derivato da dati identificativi. |

---

## 2. Meccanismo di consenso opt-in

La telemetria ARchetipo richiede consenso esplicito dell'utente:

1. Durante `archetipo init` interattivo, l'utente vede una richiesta chiara:

   ```
   Aiutaci a migliorare ARchetipo inviando telemetria anonima?
   - Nessun dato personale o di progetto viene raccolto
   - Puoi disabilitarla in qualsiasi momento con 'archetipo analytics disable'
   - Leggi docs/analytics.md per l'elenco completo dei dati inviati
   [s/N]
   ```

2. Se l'utente accetta, il flag `analytics.consent` viene salvato come `true` nel `.archetipo/config.yaml` del progetto e viene generato un UUID v4 anonimo (`anonymous_installation_id`).
3. Se l'utente rifiuta (o salta `init`), il flag rimane `false` (default) e **nessun evento viene inviato**.
4. In ambienti non interattivi (CI, pipe), si può usare il flag `--analytics=on|off` per impostare il consenso senza prompt. Il flag `--yes` riguarda solo il prompt di sovrascrittura `config.yaml`, non il consenso telemetria.
5. Il consenso è **per-progetto**: ogni progetto ha il proprio flag indipendente.
6. Il consenso può essere modificato in qualsiasi momento con `archetipo analytics enable|disable`.

### Esempio: sezione analytics nel `.archetipo/config.yaml`

```yaml
analytics:
  consent: true               # false di default, true dopo opt-in
  anonymous_installation_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"  # UUID v4, generato dopo consenso
```

Dopo il consenso, ARchetipo genera un UUID v4 casuale e lo persiste nel config. Questo UUID è usato come `anonymous_installation_id` in ogni evento inviato da questo progetto.

---

## 3. Schema evento `archetipo.analytics/v1`

Ogni evento di telemetria è un oggetto JSON con schema `archetipo.analytics/v1`.

### Struttura

```json
{
  "schema": "archetipo.analytics/v1",
  "event": "<tipo_evento>",
  "timestamp": "<ISO 8601>",
  "command": "<sottocomando>",
  "tool": "<tool_invocante>",
  "tool_version": "<versione_tool>",
  "archetipo_version": "<versione_binario>",
  "os": "<sistema_operativo>",
  "arch": "<architettura_cpu>",
  "connector": "<connector_in_uso>",
  "session_id": "<UUID v4>",
  "success": true,
  "error_code": "<E_CODE>",
  "exit_code": 0,
  "duration_ms": 1234,
  "ci": false,
  "anonymous_installation_id": "<UUID v4>",
  "spec_code": "<codice_spec>",
  "args": {
    "<nome_flag>": "<valore_flag>",
    "_0": "<arg_posizionale>"
  },
  "properties": {}
}
```

### Descrizione semantica dei campi

| Campo | Significato |
|---|---|
| `schema` | Versione dello schema evento. Valore fisso: `"archetipo.analytics/v1"`. |
| `event` | Tipo di evento. Attualmente: `command_completed`. Eventi futuri (es. `cli_started`) saranno aggiunti qui. |
| `timestamp` | Data/ora dell'evento in formato ISO 8601 UTC (es. `"2026-06-22T12:00:00Z"`). |
| `command` | Sottocomando eseguito (es. `spec.plan`, `spec.show`). Identificatore stabile, non il comando raw. |
| `tool` | Tool AI invocante (es. `claude`, `pi`). **Opzionale**: la CLI non sa quale tool l'ha invocata; riservato per uso futuro. |
| `tool_version` | Versione del tool AI invocante. **Opzionale**, come `tool`. |
| `archetipo_version` | Versione del binario ARchetipo (es. `"1.2.3"`). |
| `os` | Sistema operativo (`runtime.GOOS`: `darwin`, `linux`, `windows`). |
| `arch` | Architettura CPU (`runtime.GOARCH`: `amd64`, `arm64`). |
| `connector` | Connector in uso (`file`, `github`, `jira`). |
| `session_id` | UUID v4 generato per-invocazione per correlare eventi di una stessa run. |
| `success` | `true` se il comando è terminato con exit code 0, `false` altrimenti. |
| `error_code` | Codice errore stabile (es. `E_INVALID_INPUT`). Vuoto se `success` è `true`. |
| `exit_code` | Exit code numerico del processo (0, 1, 2, 3, 4). |
| `duration_ms` | Durata dell'esecuzione in millisecondi (intero). |
| `ci` | `true` se rilevato ambiente CI (es. `CI=true`, `GITHUB_ACTIONS=true`). |
| `anonymous_installation_id` | UUID v4 generato al primo consenso. Presente solo se `analytics.consent` è `true`. |
| `spec_code` | Codice spec (es. `US-001`). **Riservato**: non popolato da `command_completed`; per eventi futuri di spec-lifecycle. |
| `args` | Argomenti del comando: flag impostati dall'utente e argomenti posizionali. **Vedi sezione dedicata sotto.** |
| `properties` | Mappa opzionale per dati estensibili (es. `{"plan_size": 5}`). Soggetta a revisione privacy. |

### Campo `args`

Il campo `args` è una mappa opzionale che cattura gli argomenti con cui il comando è stato invocato. Il campo è omesso quando il comando non ha ricevuto argomenti.

**Formato:** ogni chiave è il nome di un flag o la posizione di un argomento posizionale.

| Tipo chiave | Formato | Esempio |
|---|---|---|
| Flag safe (valore reale) | `"<nome_flag>": <valore>` | `"status": "TODO"` |
| Flag content-sensitive (solo presenza) | `"<nome_flag>": true` | `"file": true` |
| StringSlice | `"<nome_flag>": ["a", "b"]` | `"tool": ["claude", "pi"]` |
| Argomento posizionale | `"_<indice>": "<valore>"` | `"_0": "US-005"` |

**Flag content-sensitive:** i flag `--file` e `--commit-summary` i cui valori conterrebbero path filesystem o testo libero dell'utente registrano solo la presenza (`true`), mai il valore effettivo.

**Flag safe:** tutti gli altri flag registrano il loro valore reale. Esempi: `--status`, `--to`, `--tool`, `--connector`, `--check`, `--yes`, `--port`, `--commit-type`, ecc.

**Argomenti posizionali:** registrati con chiavi `_0`, `_1`, `_2`... (es. `spec plan US-005` produce `"_0": "US-005"`).

**Esempio:** `archetipo spec list --status TODO` produce:
```json
{
  "args": {
    "status": "TODO"
  }
}
```

**Esempio con flag content-sensitive:** `archetipo spec plan US-005 --file plan.yaml` produce:
```json
{
  "args": {
    "file": true,
    "_0": "US-005"
  }
}
```

**Esempio con più argomenti:** `archetipo init --tool claude,pi --connector github --analytics on` produce:
```json
{
  "args": {
    "tool": ["claude", "pi"],
    "connector": "github",
    "analytics": "on"
  }
}
```

---

## 4. Retention

- **Lato client:** gli eventi sono inviati in fire-and-forget. Nessuna coda persistente locale. Se l'invio fallisce, l'evento viene scartato.
- **Lato backend:** gli eventi sono conservati per **30 giorni** dalla ricezione. Dopo questo periodo vengono eliminati automaticamente.
- **Aggregati anonimi** (es. conteggi per comando, percentuali di successo per OS) possono essere conservati oltre i 30 giorni in forma aggregata e non riconducibile a singole installazioni.

---

## 5. Campi ammessi (allowlist)

I seguenti campi sono gli unici ammessi nel payload di un evento `archetipo.analytics/v1`. Qualsiasi campo non elencato qui è vietato.

| Campo | Tipo | Descrizione |
|---|---|---|
| `schema` | `string` | Versione dello schema evento. Valore fisso: `"archetipo.analytics/v1"`. |
| `event` | `string` | Tipo di evento. Es. `"command_completed"`. |
| `timestamp` | `string` | Timestamp ISO 8601 UTC. |
| `command` | `string` | Sottocomando eseguito (es. `"spec.plan"`, `"spec.show"`). |
| `tool` | `string` (opzionale) | Tool AI invocante. Riservato, non popolato dalla CLI. |
| `tool_version` | `string` (opzionale) | Versione del tool AI. Riservato, non popolato dalla CLI. |
| `archetipo_version` | `string` | Versione del binario ARchetipo (es. `"1.2.3"`). |
| `os` | `string` | Sistema operativo: `"darwin"`, `"linux"`, `"windows"`. |
| `arch` | `string` | Architettura CPU: `"amd64"`, `"arm64"`. |
| `connector` | `string` | Connector in uso: `"file"`, `"github"`, `"jira"`. |
| `session_id` | `string` | UUID v4 per-invocazione per correlare eventi di una stessa run. |
| `success` | `boolean` | `true` se il comando è terminato con successo (exit code 0). |
| `error_code` | `string` | Codice errore stabile (es. `"E_INVALID_INPUT"`). Vuoto se `success` è `true`. |
| `exit_code` | `number` | Exit code numerico del processo (0, 1, 2, 3, 4). |
| `duration_ms` | `number` | Durata dell'esecuzione in millisecondi (intero). |
| `ci` | `boolean` | `true` se eseguito in ambiente CI, `false` altrimenti. |
| `anonymous_installation_id` | `string` (opzionale) | UUID v4 generato al primo consenso. Presente solo se `analytics.consent` è `true`. |
| `spec_code` | `string` (opzionale) | Codice spec. **Riservato**: non popolato da `command_completed`; per eventi futuri di spec-lifecycle. |
| `args` | `object` (opzionale) | Argomenti del comando: flag impostati e argomenti posizionali. Vedi sezione 3 per i dettagli. |
| `properties` | `object` (opzionale) | Mappa opzionale per dati estensibili. |

---

## 6. Campi vietati (denylist)

I seguenti campi sono **esplicitamente vietati** in qualsiasi evento di telemetria. La loro presenza in un payload costituisce una violazione della policy privacy.

| Campo vietato | Motivazione privacy |
|---|---|
| `path` | Identificherebbe il filesystem locale dell'utente, esponendo struttura directory e potenzialmente nomi utente. |
| `cwd` | Esporrebbe la directory di lavoro corrente, rivelando il percorso del progetto e potenzialmente il nome utente del filesystem. |
| `project_root` | Identificherebbe univocamente il progetto sul filesystem, violando l'anonimato dell'installazione. |
| `repo_name` | Rivelerebbe il nome del repository, potenzialmente contenente informazioni sul progetto o sul cliente. |
| `git_remote` | Esporrebbe l'URL del remote Git, inclusi hostname (es. GitHub Enterprise on-prem) e nome organizzazione. |
| `hostname` | Identificherebbe la macchina dell'utente, potenzialmente includendo nome utente, reparto o informazioni aziendali. |
| `username` | Dato personale diretto. Viola il principio di anonimato e potenzialmente il GDPR. |
| `email` | Dato personale diretto. Viola il principio di anonimato e potenzialmente il GDPR. |
| `token` | Contiene credenziali di autenticazione. L'invio accidentale costituirebbe una falla di sicurezza. |
| `issue_url` | Rivelerebbe l'URL completa dell'issue/repository, inclusi hostname, organizzazione e nome repo. |
| `prd_content` | Conterrebbe il contenuto del PRD: visione prodotto, strategia, requisiti — informazioni progettuali riservate. |
| `spec_content` | Conterrebbe il corpo della specifica: user story, criteri di accettazione, dettagli implementativi riservati. |
| `plan_content` | Conterrebbe il piano di implementazione: task, stime, dettagli tecnici interni. |
| `stdin_payload` | Conterrebbe l'input passato alla CLI via stdin, potenzialmente inclusi contenuti progettuali riservati. |
| `stdout_payload` | Conterrebbe l'output generato dalla CLI, potenzialmente inclusi path, nomi file, contenuti di progetto. |
| `stderr_payload` | Conterrebbe l'output di errore della CLI, potenzialmente inclusi path locali e messaggi di sistema. |
| `error_message_raw` | Conterrebbe il messaggio di errore testuale completo, che può includere path, nomi file, dettagli di sistema. Usare `error_code` (codice stabile) al suo posto. |

---

## 7. Identificatore anonimo di installazione (`anonymous_installation_id`)

`anonymous_installation_id` è l'unico campo che permette di correlare eventi provenienti dalla stessa installazione. La sua generazione e utilizzo seguono regole stringenti per garantire l'anonimato.

### Generazione

- **Algoritmo:** UUID v4 (random), generato con `crypto/rand` o equivalente crittograficamente sicuro.
- **Momento della generazione:** subito dopo che l'utente ha dato il consenso esplicito (durante `archetipo init` o al primo `archetipo analytics enable`).
- **Persistenza:** salvato nel `.archetipo/config.yaml` del progetto sotto `analytics.anonymous_installation_id`.
- **Univocità:** ogni progetto ha il proprio UUID indipendente. Due progetti sulla stessa macchina hanno UUID diversi.

### Cosa NON è

`anonymous_installation_id` **non è mai** derivato da:

| Fonte | Perché è vietato |
|---|---|
| MAC address | Identificherebbe la macchina fisica in modo permanente e tracciabile. |
| Hostname | Spesso contiene nome utente, reparto o informazioni aziendali. |
| Percorso del repository | Rivelerebbe la struttura del filesystem e potenzialmente il nome del progetto. |
| URL del remote Git | Identificherebbe organizzazione e repository. |
| Hash di uno qualsiasi dei campi sopra | Un hash deterministico è pseudonimo, non anonimo: permette comunque il tracciamento. |

### Visibilità

- Il campo è **opzionale** nel payload dell'evento.
- Se `analytics.consent` è `false`, il campo è omesso (non inviato come `null` o stringa vuota).
- Se `analytics.consent` è `true`, il campo è sempre presente con il valore UUID salvato nel config.

---

## 8. Flag `analytics.consent` nel `.archetipo/config.yaml`

Il consenso alla telemetria è gestito tramite un flag booleano nel file di configurazione del progetto.

### Posizione

```yaml
# .archetipo/config.yaml
analytics:
  consent: false              # default: false
  anonymous_installation_id: ""  # popolato dopo il consenso
```

### Comportamento

| `analytics.consent` | Comportamento |
|---|---|
| `false` (default) | Nessun evento viene inviato. `anonymous_installation_id` non viene generato. |
| `true` | Gli eventi vengono inviati in fire-and-forget al backend. `anonymous_installation_id` è presente in ogni evento. |

### Modifica

Il flag può essere modificato in qualsiasi momento tramite i comandi:

```bash
archetipo analytics enable   # imposta consent: true, genera UUID se assente
archetipo analytics disable  # imposta consent: false, non cancella l'UUID
```

**Nota:** `analytics enable` invia l'evento di enable al server (il consenso diventa true prima dell'invio). `analytics disable` **non** invia l'evento di disable (il consenso diventa false prima dell'invio, rispettando il nuovo stato di opt-out).

### Esempio completo della sezione analytics

```yaml
analytics:
  consent: true
  anonymous_installation_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
```

**Nota:** disabilitare il consenso (`disable`) non cancella l'UUID. Se l'utente riabilita la telemetria, lo stesso UUID viene riutilizzato per mantenere la continuità delle metriche aggregate (senza creare un nuovo identificatore a ogni toggle).

---

## 9. Fixture JSON di esempio

### Evento valido — `command_completed`

Questo JSON rappresenta un evento `command_completed` valido dopo che l'utente ha eseguito `archetipo spec plan US-005` e il comando è fallito con un errore di input.

```json
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
  "timestamp": "2026-06-22T12:00:00Z",
  "command": "spec.plan",
  "archetipo_version": "1.2.3",
  "session_id": "e4f5a6b7-c8d9-4e0f-8a1b-2c3d4e5f6a7b",
  "os": "darwin",
  "arch": "arm64",
  "connector": "file",
  "success": false,
  "error_code": "E_INVALID_INPUT",
  "exit_code": 2,
  "duration_ms": 1234,
  "ci": false,
  "anonymous_installation_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}
```

### Variante senza consenso (senza `anonymous_installation_id`)

Quando `analytics.consent` è `false`, il campo `anonymous_installation_id` è omesso:

```json
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
  "timestamp": "2026-06-22T12:05:00Z",
  "command": "spec.show",
  "archetipo_version": "1.2.3",
  "session_id": "f1a2b3c4-d5e6-4f7a-8b9c-0d1e2f3a4b5c",
  "os": "linux",
  "arch": "amd64",
  "connector": "github",
  "success": true,
  "exit_code": 0,
  "duration_ms": 856,
  "ci": true
}
```

### Evento valido con `args` — `command_completed`

Questo JSON rappresenta un evento dopo `archetipo spec list --status TODO`:

```json
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
  "timestamp": "2026-06-22T12:15:00Z",
  "command": "spec.list",
  "archetipo_version": "1.2.3",
  "session_id": "b1c2d3e4-f5a6-4b7c-8d9e-0f1a2b3c4d5e",
  "os": "darwin",
  "arch": "arm64",
  "connector": "file",
  "success": true,
  "exit_code": 0,
  "duration_ms": 234,
  "ci": false,
  "anonymous_installation_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "args": {
    "status": "TODO"
  }
}
```

### Evento valido con flag content-sensitive — `command_completed`

Questo JSON rappresenta un evento dopo `archetipo spec plan US-005 --file /path/to/plan.yaml`. Notare che `file` registra solo presenza (`true`), non il percorso.

```json
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
  "timestamp": "2026-06-22T12:20:00Z",
  "command": "spec.plan",
  "archetipo_version": "1.2.3",
  "session_id": "c2d3e4f5-a6b7-4c8d-9e0f-1a2b3c4d5e6f",
  "os": "darwin",
  "arch": "arm64",
  "connector": "file",
  "success": true,
  "exit_code": 0,
  "duration_ms": 567,
  "ci": false,
  "anonymous_installation_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "args": {
    "file": true,
    "_0": "US-005"
  }
}
```

### Esempio NON valido — campi vietati

Il JSON seguente contiene campi esplicitamente vietati. **Non deve mai essere inviato** al backend di telemetria. I commenti spiegano perché ogni campo è inaccettabile.

```jsonc
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
  "timestamp": "2026-06-22T12:10:00Z",
  "command": "spec.plan",
  "archetipo_version": "1.2.3",
  "session_id": "abcd-efgh",
  "os": "darwin",
  "arch": "arm64",
  "connector": "file",
  "success": false,
  "error_code": "E_INVALID_INPUT",
  "exit_code": 2,
  "duration_ms": 1234,
  "ci": false,
  "anonymous_installation_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",

  // ❌ VIETATO: path — identificherebbe il filesystem locale dell'utente
  "path": "/home/dev/projects/my-app/src/main.go",

  // ❌ VIETATO: cwd — esporrebbe la directory di lavoro, rivelando percorso progetto e nome utente
  "cwd": "/home/dev/projects/my-app",

  // ❌ VIETATO: project_root — identificherebbe univocamente il progetto sul filesystem
  "project_root": "/home/dev/projects/my-app",

  // ❌ VIETATO: repo_name — rivelerebbe il nome del repository
  "repo_name": "my-client-project",

  // ❌ VIETATO: git_remote — esporrebbe l'URL del remote, incluso hostname e organizzazione
  "git_remote": "https://github.com/my-org/my-client-project.git",

  // ❌ VIETATO: hostname — identificherebbe la macchina dell'utente
  "hostname": "dev-laptop-42",

  // ❌ VIETATO: username — dato personale diretto, viola anonimato e GDPR
  "username": "mario.rossi",

  // ❌ VIETATO: email — dato personale diretto, viola anonimato e GDPR
  "email": "mario.rossi@example.com",

  // ❌ VIETATO: token — conterrebbe credenziali di autenticazione (falla di sicurezza)
  "token": "ghp_xxxxxxxxxxxxxxxxxxxx",

  // ❌ VIETATO: issue_url — rivelerebbe URL completa con hostname, org, repo
  "issue_url": "https://github.com/my-org/my-client-project/issues/42",

  // ❌ VIETATO: prd_content — conterrebbe visione prodotto, strategia, requisiti riservati
  "prd_content": "## Vision\nIl prodotto MyApp rivoluzionerà il mercato...",

  // ❌ VIETATO: spec_content — conterrebbe user story e criteri di accettazione riservati
  "spec_content": "Come utente voglio poter gestire il mio account...",

  // ❌ VIETATO: plan_content — conterrebbe task, stime e dettagli tecnici interni
  "plan_content": "TASK-01: Implementare endpoint GET /api/users...",

  // ❌ VIETATO: stdin_payload — conterrebbe input passato alla CLI via stdin
  "stdin_payload": "{\"title\": \"Nuova feature segreta\"}",

  // ❌ VIETATO: stdout_payload — conterrebbe output generato dalla CLI
  "stdout_payload": "{\"ok\": true, \"path\": \"/home/dev/projects/my-app/src/main.go\"}",

  // ❌ VIETATO: stderr_payload — conterrebbe errori con path locali
  "stderr_payload": "Error: file not found at /home/dev/projects/my-app/config.yaml",

  // ❌ VIETATO: error_message_raw — conterrebbe messaggio testuale con possibili path. Usare error_code.
  "error_message_raw": "validation failed for field 'title' in /home/dev/projects/my-app/spec.yaml: missing required field"
}
```

**Regola d'oro:** se un campo non compare nella tabella [Campi ammessi](#5-campi-ammessi-allowlist), non va inviato. Punto.
