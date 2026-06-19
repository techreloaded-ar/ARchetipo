# ARchetipo — Contratto Analytics

**Versione:** 1.0
**Schema evento:** `archetipo.analytics/v1`
**Ultima modifica:** 2026-06-19

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

1. Durante `archetipo init`, l'utente vede una richiesta chiara:

   ```
   Aiutaci a migliorare ARchetipo inviando telemetria anonima?
   - Nessun dato personale o di progetto viene raccolto
   - Puoi disabilitarla in qualsiasi momento con 'archetipo analytics disable'
   - Leggi docs/analytics.md per l'elenco completo dei dati inviati
   [Sì / No]
   ```

2. Se l'utente accetta, il flag `analytics.consent` viene salvato come `true` nel `.archetipo/config.yaml` del progetto.
3. Se l'utente rifiuta (o salta `init`), il flag rimane `false` (default) e **nessun evento viene inviato**.
4. Il consenso è **per-progetto**: ogni progetto ha il proprio flag indipendente.
5. Il consenso può essere modificato in qualsiasi momento con `archetipo analytics enable|disable` (US-002).

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
  "version": "<versione binario>",
  "os": "<sistema_operativo>",
  "arch": "<architettura_cpu>",
  "connector": "<connector_in_uso>",
  "success": true,
  "error_code": "<E_CODE>",
  "exit_code": 0,
  "duration_ms": 1234,
  "ci": false,
  "anonymous_installation_id": "<UUID v4>"
}
```

### Descrizione semantica dei campi

| Campo | Significato |
|---|---|
| `schema` | Versione dello schema evento. Valore fisso: `"archetipo.analytics/v1"`. Permette al backend di validare e versionare la deserializzazione. |
| `event` | Tipo di evento. Attualmente l'unico evento è `command_completed`. Eventi futuri (es. `cli_started`) saranno aggiunti qui. |
| `timestamp` | Data/ora dell'evento in formato ISO 8601 UTC (es. `"2026-06-19T13:00:00Z"`). Non contiene timezone locale. |
| `command` | Sottocomando eseguito (es. `spec.plan`, `spec.show`, `config.show`). Identificatore stabile, non il comando raw. |
| `version` | Versione del binario ARchetipo (es. `"1.2.3"`). |
| `os` | Sistema operativo rilevato da Go (`runtime.GOOS`: `darwin`, `linux`, `windows`). |
| `arch` | Architettura CPU rilevata da Go (`runtime.GOARCH`: `amd64`, `arm64`). |
| `connector` | Connector in uso per questa esecuzione (`file`, `github`). |
| `success` | `true` se il comando è terminato con exit code 0, `false` altrimenti. |
| `error_code` | Codice errore stabile (es. `E_INVALID_INPUT`, `E_CONNECTOR`). Vuoto (`""`) se `success` è `true`. Non contiene il messaggio di errore raw. |
| `exit_code` | Exit code numerico del processo (0, 1, 2, 3, 4). |
| `duration_ms` | Durata dell'esecuzione in millisecondi (intero). |
| `ci` | `true` se l'ambiente di esecuzione è rilevato come CI (es. `CI=true`, `GITHUB_ACTIONS=true`). `false` altrimenti. |
| `anonymous_installation_id` | UUID v4 generato localmente dopo consenso. **Opzionale**: assente (`null` o campo omesso) se il consenso non è stato dato. |

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
| `command` | `string` | Sottocomando eseguito. Identificatore stabile (es. `"spec.plan"`, `"spec.show"`). |
| `version` | `string` | Versione del binario ARchetipo (es. `"1.2.3"`). |
| `os` | `string` | Sistema operativo: `"darwin"`, `"linux"`, `"windows"`. |
| `arch` | `string` | Architettura CPU: `"amd64"`, `"arm64"`. |
| `connector` | `string` | Connector in uso: `"file"`, `"github"`. |
| `success` | `boolean` | `true` se il comando è terminato con successo (exit code 0). |
| `error_code` | `string` | Codice errore stabile (es. `"E_INVALID_INPUT"`). Stringa vuota `""` se `success` è `true`. |
| `exit_code` | `number` | Exit code numerico del processo (0, 1, 2, 3, 4). |
| `duration_ms` | `number` | Durata dell'esecuzione in millisecondi (intero). |
| `ci` | `boolean` | `true` se eseguito in ambiente CI, `false` altrimenti. |
| `anonymous_installation_id` | `string` (opzionale) | UUID v4 generato localmente dopo consenso. **Opzionale:** presente solo se `analytics.consent` è `true`. |

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

Il flag può essere modificato in qualsiasi momento tramite i comandi (da implementare in US-002):

```bash
archetipo analytics enable   # imposta consent: true, genera UUID se assente
archetipo analytics disable  # imposta consent: false, non cancella l'UUID
```

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

Questo JSON rappresenta un evento `command_completed` valido dopo che l'utente ha eseguito `archetipo spec plan US-005` e il comando è fallito con un errore di input. Tutti i valori sono fittizi e non contengono dati reali del workspace di sviluppo.

```json
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
  "timestamp": "2026-06-19T13:00:00Z",
  "command": "spec.plan",
  "version": "1.2.3",
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
  "timestamp": "2026-06-19T13:05:00Z",
  "command": "spec.show",
  "version": "1.2.3",
  "os": "linux",
  "arch": "amd64",
  "connector": "github",
  "success": true,
  "error_code": "",
  "exit_code": 0,
  "duration_ms": 856,
  "ci": true
}
```

### Esempio NON valido — campi vietati

Il JSON seguente contiene campi esplicitamente vietati. **Non deve mai essere inviato** al backend di telemetria. I commenti spiegano perché ogni campo è inaccettabile.

```jsonc
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
  "timestamp": "2026-06-19T13:10:00Z",
  "command": "spec.plan",
  "version": "1.2.3",
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
