# ARchetipo — Recap del branch `feature/analytics`

**Data:** 2026-06-24
**Branch:** `feature/analytics`
**Base:** `3c068be` (merge-base con `main`)
**Scope:** introduzione del sistema di telemetria anonima opt-in end-to-end (contratto, client Go, instrumentazione CLI, comando consenso, endpoint ingest HTTPS, validazione di rilascio)
**Stato:** tutte le spec dell'epic analytics (`US-001`…`US-007`) sono in `DONE` su questo branch; **l'integrazione di analytics nel prodotto è sospesa** su decisione del maintainer, con l'intenzione di riprenderla in futuro senza perdere il filo del discorso.

> Questo documento è pensato come **bussola per la ripresa futura**: descrive cosa è stato costruito, perché è stato scelto così, cosa resta da decidere e dove si trova ogni pezzo. È autocontenuto: una sessione futura può leggerlo e ripartire dal punto esatto in cui ci si è fermati.

---

## TL;DR

- Aggiunta la **telemetria anonima e opt-in** ad ARchetipo: la CLI può invocare, a fine comando, un evento `command_completed` a un endpoint HTTP configurable.
- **Privacy by design**: allowlist di campi, denylist esplicita, niente path/repo/token/contenuti. Consenso per-progetto sul `.archetipo/config.yaml`, default `false`.
- **Backend di ingest** (Go, package `cli/internal/analytics/ingest`): endpoint `POST /v1/events` con validazione schema strict, rate limiting token-bucket per origine (IP hashed), storage in-memory con TTL 7 giorni. Pensato per girare come binario separato (es. `analytics-server`); il server è referenziato dalla CLI via HTTP.
- **Consenso gestibile** via `archetipo analytics status|enable|disable` e richiesto interattivamente in `archetipo init`.
- **Documentazione trasparente** per l'utente finale in `docs/analytics.md` (contratto + privacy), `docs/analytics-api.md` (contratto HTTP) e sezioni Telemetria in `README.md` / `README.it.md`.
- **Validazione di rilascio** (US-007) come suite di regression privacy + gate CI (`cli/ci-check.sh`).
- **TODO al momento della sospensione**: il worktree workflow è stato **abilitato di default** nel config template (vedi "Decisions & TODO al momento della sospensione").

---

## 1. Perché questo branch esiste

In assenza di qualsiasi segnale d'uso è difficile capire quali comandi vengono effettivamente adottati, su quali connector/piattaforme, e dove si concentrano gli errori. Il branch introduce un canale di telemetria **minimale, anonimo e consensuale** per alimentare decisioni di prodotto basate su dati aggregati — senza compromettere la privacy degli utenti, attrito di adoption o l'agnosticismo rispetto al tool AI.

I principi non negoziabili assunti durante il lavoro (v. `docs/analytics.md` §1):

| Principio | Significato pratico |
|---|---|
| Trasparenza | Ogni evento inviato è documentato; l'utente può ispezionarlo; nessun campo nascosto. |
| Raccolta minima | Solo i campi necessari; qualsiasi nuova aggiunta richiede revisione privacy. |
| Privacy-preserving | Niente path/repo/hostname/username/token/contenuti. ID installazione random locale. |
| Consenso esplicito | Opt-in; default spento; modificabile per-progetto. |
| Nessun tracciamento | `anonymous_installation_id` è l'unico correlatore e non deriva da fingerprint. |

---

## 2. Mappa del lavoro (spec dell'epic)

Tutte le spec vivono in `.archetipo/specs/US-00X.yaml`, epic `EP-000` (Foundation). Tutte in stato `DONE`.

| Spec | Punti | Titolo | Cosa consegna |
|---|---|---|---|
| **US-001** | 3 | Definire contratto analytics, policy privacy e consenso | `docs/analytics.md`: schema `archetipo.analytics/v1`, allowlist, denylist, meccanismo consenso opt-in, fixture JSON valide e non valide. Solo contratto, zero codice rete. |
| **US-002** | 3 | Aggiungere comando `archetipo analytics` | Gruppo Cobra `analytics` con subcomandi `status`/`enable`/`disable`; persistenza del consenso su `.archetipo/config.yaml` per-progetto; envelope `iox`. |
| **US-003** | 5 | Implementare client analytics Go | Package `cli/internal/analytics` (`settings.go`, `client.go`, `event.go`): `Settings`, `Client.Send` HTTP fire-and-forget, fail-silent, timeout, no env, no segreti. |
| **US-004** | 3 | Instrumentazione comandi CLI | Hook centralizzato in `Execute` (`root.go`): cattura `duration_ms`, `success`, `exit_code`, `error_code`, `connector`; nessuna instrumentazione per-sottocomando. |
| **US-005** | 3 | Telemetria comprensibile e controllabile | Prompt di consenso in `archetipo init` (no `--yes`), riga `analytics` in `archetipo doctor`, sezione Telemetria in README + `.archetipo/config.yaml`. |
| **US-006** | 5 | Endpoint HTTPS ingest analytics | Package `cli/internal/analytics/ingest` (`server.go`, `handler.go`, `schema.go`, `ratelimit.go`, `storage.go`, `types.go`): `POST /v1/events`, validazione strict, rate limit, storage TTL, IP mai persistito. |
| **US-007** | 5 | Validazione end-to-end per release | Gate CI (`cli/ci-check.sh`), regression privacy (assert no forbidden fields nel payload), test offline/failure, tracciamento argomenti (`args.go`). |

**Ordine di dipendenza applicato:** US-001 → (US-002, US-006) → US-003 → (US-004 ⊂ US-001+US-003; US-005 ⊂ US-001+US-002+US-003) → US-007 ⊂ US-004+US-005+US-006.

> Nota storica: US-006 è stata sviluppata parallelamente a US-002 (merge `archetipo/US-006` unificato in `10d9986` con `analytics_cmd.go` per consenso + serve), poi US-003 dipendeva da entrambe. L'ordine delle AC riflette la dipendenza logica, non cronologica.

---

## 3. Cosa è stato aggiunto (per componente)

### 3.1 Documentazione (`docs/`)

| File | Ruolo | Punti chiave |
|---|---|---|
| `docs/PRD_analytics.md` | PRD dell'inception di analytics (prodotto, personas, MVP, architettura, FR/NFR). | Valido anche come documento di *prodotto*; descrive finalità e differenziazione. |
| `docs/analytics.md` | **Contratto v1.1** lato client: schema evento, finalità, principi, allowlist, denylist, `anonymous_installation_id`, consenso opt-in, retention. | Fonte di verità per "cosa entra in un evento". Include fixture JSON valide e un esempio commentato di evento NON valido con tutti i campi vietati. |
| `docs/analytics-api.md` | **Contratto HTTP** lato backend: `POST /v1/events`, Content-Type, batch, codici stato (`202`/`400`/`429`), header rate limit, privacy. | Documentato in modo che la CLI open source possa integrare senza leggere il codice del backend. |

### 3.2 CLI — superficie del comando analytics

- `archetipo analytics status` → envelope con `enabled`, `source` (`project_config|default`), `endpoint` (redatto/nome canale, mai URL con token), `anonymous_installation_id_present`.
- `archetipo analytics enable` → imposta `analytics.consent: true` nel `.archetipo/config.yaml` del progetto, genera UUID v4 se assente.
- `archetipo analytics disable` → imposta `consent: false`, **non** cancella l'UUID (per riuso al riattivarlo senza spezzare la continuità aggregate). `disable` non invia l'evento di disable.

> **Comando `analytics serve`**: c'è stato un merge (`10d9986` "unifica analytics_cmd.go (consenso + serve)"). Lo struct del file racchiude sia i subcomandi di consenso sia quello per far girare localmente l'ingest server. Da verificare al momento della ripresa se `serve` è destinato a restare pubblico o è un helper di sviluppo.

### 3.3 CLI — instrumentazione automatica (`root.go`, `args.go`)

- In `Execute` (vedi `cli/internal/cli/root.go`):
  - cattura `time.Now()` prima del comando;
  - dopo `root.Execute()`, se `Settings.Enabled == true`, costruisce l'evento `command_completed` e lo invia;
  - `command` normalizzato come `spec.list` / `config.show` (punti → `.`);
  - `success` = `err == nil`;
  - `exit_code` da `exitCodeFor(err)` (0–4);
  - `error_code` solo da `*iox.CodedError` (es. `E_INVALID_INPUT`); vuoto altrimenti;
  - `connector` risolto best-effort da `config.Load`, fallback `"unknown"`;
  - **nessuna instrumentazione per-sottocomando** (grep su `spec_cmd.go`, `task_cmd.go` ecc. pulito);
  - **fail-silent**: errori del client non mutano exit code, stdout/stderr, né causano panic.
- **Tracciamento argomenti** (`cli/internal/cli/args.go`, commit `6378a37` "aggiungi tracciamento argomenti comandi"):
  - `extractArgs(cmd)` produce `args` map;
  - solo flag **esplicitamente settati** dall'utente (`cmd.Flags().Visit`);
  - flag **content-sensitive** (`file`, `commit-summary`) → solo presenza `true`, mai il valore;
  - flag normali → valore reale (le `StringSlice` diventano array JSON puliti);
  - argomenti posizionali → chiavi `_0`, `_1`, …;
  - `nil` (campo omesso via `omitempty`) se non ci sono argomenti.

### 3.4 CLI — sezioni nelle skill esistenti

- `archetipo init` interattivo: dopo selezione tool/connector e prima di installare le skill, **prompt di consenso** con testo di `docs/analytics.md` (s/N, default n). La risposta → `analytics.consent` in `.archetipo/config.yaml`.
- `archetipo init --yes`: **NON** attiva gli analytics; il flag risponde solo ai prompt di overwrite del config, non al consenso telemetria. (Verificare anche `--analytics=on|off` descritto nel contratto: al momento della ripresa, confermare che sia cablato/rispettato.)
- `archetipo doctor`: nuova riga `analytics` con stato (`enabled`/`disabled`) e provenienza (`project_config`/`default`).

### 3.5 Client analytics Go (`cli/internal/analytics/`)

Package autonomo, zero dipendenze da config/env, progettato per essere iniettato dal chiamante.

| File | Contenuto | Note |
|---|---|---|
| `settings.go` | Tipo `Settings{Enabled, Endpoint, Timeout, UserAgent}` + default documentati. | `Enabled=false`, `Timeout=2s`, `Endpoint=localhost` (test), `UserAgent="archetipo-analytics/<version>"` via `cli/internal/version`. |
| `client.go` | `NewClient(Settings) *Client`, `Send(ctx, Event) error`. | Fire-and-forget, `httptest`-testable, `User-Agent` header, Content-Type `application/json`. Errori di rete/non-2xx/timeout: loggati su `Log` (`io.Discard` di default), **mai** propagati, **mai** panic, **mai** alterano exit code/output. |
| `event.go` | Struct `Event` con **esattamente** i campi dell'allowlist US-001, `omitempty` ovunque. | Nessun `map[string]any` "aperto": `Args` è l'unica mappa, documentata. |

### 3.6 Backend ingest (`cli/internal/analytics/ingest/`)

Server HTTP pensato per girare come binario separato (es. un `analytics-server`); il CLI lo raggiunge solo via HTTP. Dipendenze iniettate (`EventStore`, `Clock`) → test deterministici.

| File | Contenuto |
|---|---|
| `server.go` | `Server` + `ServerConfig` + `DefaultServerConfig` (`127.0.0.1:8080`, rate default, TTL 7gg). Bind locale. |
| `handler.go` | Handler `POST /v1/events`: accept singolo/array (batch fail-atomic), 202/400/429. |
| `schema.go` | Validazione strict `archetipo.analytics/v1`: allowlist + denylist (path, cwd, hostname, username, email, token, …). Campi sconosciuti → 400. |
| `ratelimit.go` | `TokenBucket` per origine (IP hashed come chiave), cleanup bucket idle, `Retry-After`, header `X-RateLimit-*`. Default 60 req/min, burst 10. |
| `storage.go` | `MemoryStore` con TTL + background cleanup. `StoredEvent` **non contiene IP**. Interfaccia `EventStore` per iniettare SQLiteStore futuro. |
| `types.go` | Tipi evento normalizzati lato storage (`AnalyticsEvent`, `StoredEvent`). |

### 3.7 Utility & CI

- `cli/internal/uuid/uuid.go` — UUID v4 via `crypto/rand` (RFC 9562), usato per `anonymous_installation_id` e `session_id`.
- `cli/ci-check.sh` — script one-shot che replica i gate di CI: `gofmt -l .` (vuoto), `go vet ./...`, `go build ./...`, `go test ./...`, `golangci-lint run --timeout 5m ./...` (0 issues). Da usare **prima di ogni commit sulla CLI** (anche per la ripresa).
- Modifiche a `.archetipo/config.yaml` (template): aggiunta sezione `analytics` commentata con `# consent: false`; **worktree workflow ora `enabled: true`** (pressione esterna, v. §6).

### 3.8 Modifiche ai README

`README.md` e `README.it.md`: nuova sezione **Telemetry / Telemetria** con elenco "cosa raccolto" vs "cosa mai raccolto" e istruzioni di opt-out (`archetipo analytics disable` o edit del config).

### 3.9 Test

Aggiunti ~3.700+ righe di test (la `cli/internal/cli/cli_test.go` passa a 1.048 righe; `args_test.go` 241; i test `analytics/*` totalizzano oltre 1.100 righe). Copertura:

- **Client** (`client_test.go`, `event_test.go`, `settings_test.go`): httptest server, `Enabled=false` → zero chiamate, server down → `Send` non errore, timeout, marshal allowlist-only, default `Settings`.
- **CLI analytics command** (`analytics_cmd_test.go`): default/enable/disable/idempotenza, progetto con config esistente.
- **Root/Args** (`cli_test.go`, `args_test.go`): payload corretti per multipli comandi, consenso off → no invio, endpoint down → output/exit invariati, flag content-sensitive → solo presenza, posizionali → `_0`/`_1`.
- **Ingest** (`handler_test.go`, `schema_test.go`, `ratelimit_test.go`, `server_test.go`, `storage_test.go`): 202 ok, 400 per forbidden/unknown, 429 rate limit, batch fail-atomic, TTL cleanup, IP mai persistito.
- **UUID** (`uuid_test.go`): format v4, variant/version bits.
- **Config** (`config_test.go`): load `.archetipo/config.yaml` con sezione analytics.

---

## 4. Contratto evento `archetipo.analytics/v1` (sintesi)

Fonte di verità: `docs/analytics.md` §3 e §5.

### Campi ammessi (allowlist)

```
schema, event, timestamp, command, tool*, tool_version*, archetipo_version,
os, arch, connector, session_id, success, error_code, exit_code,
duration_ms, ci, anonymous_installation_id*, spec_code*, args*, properties*
```
(* opzionali; `spec_code` è **riservato** — non popolato da `command_completed`, per eventi futuri spec-lifecycle.)

### Campi vietati (denylist)

`path`, `cwd`, `project_root`, `repo_name`, `git_remote`, `hostname`, `username`, `email`, `token`, `issue_url`, `prd_content`, `spec_content`, `plan_content`, `stdin_payload`, `stdout_payload`, `stderr_payload`, `error_message_raw`. (Lato backend: anche `password`, `secret`, `key`, `api_key`, `credential`, `env`, `environment`, `home`, `homedir`, `ip`, `ip_address`, `machine_id`, `device_id`, `mac`, `mac_address`, `user`.)

### `args`: regole di redazione

- flag safe → valore reale (`"status": "TODO"`);
- flag content-sensitive (`file`, `commit-summary`) → solo presenza (`true`);
- StringSlice → array JSON (`"tool": ["claude","pi"]`);
- posizionali → `"_0": "US-005"`.

### Esempio completo

```json
{
  "schema": "archetipo.analytics/v1",
  "event": "command_completed",
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
  "args": { "status": "TODO" }
}
```

---

## 5. Privacy model — sintesi operativa

1. **Consenso**: opt-in, per-progetto, su `.archetipo/config.yaml`. Default `false`. Richiesto in `init` interattivo; `--yes` NON lo attiva. Modificabile con `analytics enable|disable`.
2. **ID anonimo**: UUID v4 random (non fingerprint), generato al primo consenso, persistito, non cancellato dal `disable` (continuità aggregate).
3. **Minimalità**: solo allowlist; aggiunte richiedono revisione privacy + aggiornamento di `analytics.md`, schema backend e test.
4. **Niente rete senza consenso**: `Client.Send` ritorna `nil` immediatamente se `Enabled=false`. Endpoint down/4xx/5xx/timeout → fail-silent (no panic, no exit code, no stderr).
5. **Inlining zero contenuti**: PRD/spec/plan/stdin/stdout/stderr/error raw mai raccolti.
6. **Backend**: `POST /v1/events` no-auth; rate limit per-IP hashato; storage TTL; IP raw mai persistito oltre log tecnici minimi (TTL 7gg). Batch fail-atomic.

---

## 6. Decisions & TODO al momento della sospensione

Punti che **una sessione futura deve rivedere** prima di "riattivare" analytics:

1. **`worktree.enabled` passato a `true` nel template di config** (`.archetipo/config.yaml`). Modifica non strettamente legata ad analytics; se non voluta nella release, riportare a `false` o separare i due temi in PR distinte.
2. **Endpoint reale di produzione non configurato**: il `Settings.Endpoint` default è `localhost` (per test). Per andare in produzione bisogna (a) scegliere l'host del binario ingest, (b) esporre `analytics.endpoint` in `config.yaml` o env, (c) impostarlo lato client senza hardcodare nel sorgente (AC US-003 vieta hardcoding).
3. **`analytics serve`** (merge `10d9986`): chiarire se è un comando pubblico, un helper dev o da rimuovere. Verificare che non diventi superficie stabile non documentata.
4. **Flag `--analytics=on|off`** (documentato in `analytics.md` §2 per ambienti non interattivi/CI): confermare che `archetipo init` lo rispetti effettivamente e che i test lo coprano.
5. **Schermo `/v1/events` con TLS**: il contratto parla di "HTTPS"; verificare che il server ingest sia esposto dietro TLS/reverse proxy in produzione e che il `Client` usi `https://` (oggi endpoint è `localhost`, in chiaro).
6. **Retention lato backend**: `DefaultServerConfig` TTL = 7 giorni per `MemoryStore`. Il contratto parla di retention lato backend di **30 giorni** (con TTL 7gg solo per i log raw IP — `analytics-api.md` "Note sulla privacy"). Allineare MemoryStore/SQLiteStore production al valore di contratto (30gg per gli eventi, 7gg max per log raw con IP).
7. **Aggregati e dashboard**: nessuna UI di consumo dei dati aggregati è stata costruita (lato backend). È il prossimo step naturale di prodotto post-rilascio.
8. **Eventi `spec-lifecycle` futuri**: `spec_code` è riservato e non popolato. Quando si vorranno eventi di ciclo spec (es. `spec.created`, `spec.moved`), estendere allowlist + handler + test.
9. **Campo `properties`**: mappa libera; prima di popolarla in produzione, definire una sotto-allowlist per evitare che diventi backdoor di dati sensibili (consigliato prima di qualsiasi uso reale).
10. **Versionamento schema**: `archetipo.analytics/v1` è fisso. Una eventuale v2 richiede nuovo campo `schema`,andler separato con backward-compat, e bump documentato.

---

## 7. Come riprendere in futuro (checklist operativa)

Per "riattivare" analytics partendo da questo branch, idealmente:

1. **Partire puliti**: creare un branch `feature/analytics-resume` da `feature/analytics` (o cherry-pick se `main` è andato avanti). Mantenere la cronologia delle spec DONE.
2. **Validare la suite**: dal repo root
   ```bash
   cd cli && ./ci-check.sh
   ```
   deve uscire 0 (gofmt vuoto, vet/build/test ok, golangci-lint 0 issues). È il gate di US-007 e la prima cosa da fare.
3. **Rileggere il contratto**: `docs/analytics.md` + `docs/analytics-api.md`. Sono la singola fonte di verità; qualunque modifica al payload deve aggiornarli in sincrono.
4. **Decidere i §6 TODO** (almeno: 2 endpoint produzione, 4 flag `--analytics`, 5 TLS, 6 retention 30gg).
5. **Smoke test end-to-end locale**:
   - `archetipo analytics enable` → verificare `analytics.consent: true` + UUID in `.archetipo/config.yaml`.
   - Lanciare il server ingest locale (verifica `archetipo analytics serve` o un binario dedicato) su `127.0.0.1:8080`.
   - Impostare l'endpoint nel config (o in `Settings`) verso il server locale.
   - Eseguire `archetipo spec list` e altri comandi → osservare il `POST /v1/events` ricevuto, validare il payload contro allowlist, confermare assenza di campi vietati (la regression test di US-007 codifica questo check).
   - `archetipo analytics disable` → confermare nessuna ulteriore chiamata di rete.
6. **Aggiornare i test di regression** se il contratto evolve (allowlist/denylist) o se si popola `spec_code`/`properties`.
7. **Aggiornare README/docs** se cambia qualcosa di user-facing (consenso, opt-out, new fields).
8. **PR checklist pre-merge**: `ci-check.sh` verde; spec US-001…US-007 confermate DONE; review del diff per campi sensibili; CHANGELOG/release notes con la sezione Telemetria.

---

## 8. Inventario file (diff vs base `3c068be`)

```
docs/PRD_analytics.md                            (434)  PRD inception analytics
docs/analytics.md                               (484)  Contratto client v1.1 (allowlist/denylist/consenso)
docs/analytics-api.md                           (242)  Contratto HTTP POST /v1/events
.archetipo/specs/US-000.yaml … US-007.yaml      (+8)   Backlog epic analytics (tutte DONE su questo branch)
.archetipo/config.yaml                           (-16+16)  Sezione analytics + worktree.enabled: true
README.md / README.it.md                         (+30 each)  Sezione Telemetry
cli/ci-check.sh                                  (69)   Gate CI one-shot
cli/internal/analytics/
  client.go (66)  client_test.go (205)
  event.go  (37)  event_test.go  (182)
  settings.go (55) settings_test.go (74)
cli/internal/analytics/ingest/
  server.go (101)    server_test.go    (307)
  handler.go (221)   handler_test.go   (311)
  schema.go  (203)   schema_test.go    (287)
  ratelimit.go (163) ratelimit_test.go (179)
  storage.go  (98)   storage_test.go   (184)
  types.go   (160)
cli/internal/cli/
  analytics_cmd.go (241)  analytics_cmd_test.go (392)
  args.go (62)  args_test.go (241)
  cli_test.go (1048)       ← esteso con test instrumentazione/consenso/privacy
  doctor_cmd.go (+25)      ← riga analytics
  init_project_cmd.go (+80) ← prompt consenso
  root.go (+163)           ← hook Execute + init client analytics
cli/internal/config/config.go (+169) / config_test.go (+160)  ← sezione analytics
cli/internal/uuid/uuid.go (18) / uuid_test.go (28)
cli/internal/connector/jira/workflow_provisioner.go (-10)      ← touch correlato (build)
```

Totale: **+7.111 / −17** righe, 45 file toccati, 35 nuovi.

---

## 9. Fonti primarie lette per compilare questo recap

- `docs/analytics.md` (contratto v1.1)
- `docs/analytics-api.md` (contratto HTTP)
- `docs/PRD_analytics.md` (PRD inception)
- `.archetipo/specs/US-001.yaml`…`US-007.yaml` (backlog + history delle transizioni)
- sorgenti: `cli/internal/analytics/{client,event,settings}.go`, `cli/internal/analytics/ingest/{server,handler,schema,ratelimit,storage,types}.go`, `cli/internal/cli/{root,args,analytics_cmd}.go`, `cli/internal/uuid/uuid.go`
- diff `.archetipo/config.yaml`, `README.md`, `README.it.md`

---

_Recap generato il 2026-06-24 dal branch `feature/analytics`. Mantiene il filo del discorso per una ripresa futura delle analytics senza dover ricostruire il contesto._