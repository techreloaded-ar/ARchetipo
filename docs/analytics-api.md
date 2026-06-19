# ARchetipo Analytics API — Contratto HTTP

## Endpoint

```
POST /v1/events
```

L'endpoint accetta eventi telemetrici nel formato `archetipo.analytics/v1`.
Non richiede autenticazione. La protezione anti-abuso è basata su rate limiting,
validazione schema strict e anomaly detection.

## Content-Type

```
Content-Type: application/json
```

Le richieste con Content-Type diverso da `application/json` ricevono `400 Bad Request`.

## Formato richiesta

Il body può essere un **oggetto JSON singolo** oppure un **array JSON di oggetti**
(invio batch). Ogni oggetto deve essere conforme allo schema `archetipo.analytics/v1`.

### Schema evento `archetipo.analytics/v1`

| Campo | Tipo | Obbligatorio | Descrizione |
|-------|------|:---:|-------------|
| `schema` | string | ✓ | Deve essere `"archetipo.analytics/v1"` |
| `event` | string | ✓ | Nome dell'evento (es. `"cli.invocation"`, `"spec.created"`) |
| `tool` | string | | Nome del tool (es. `"claude-code"`, `"codex"`) |
| `tool_version` | string | | Versione del tool |
| `os` | string | | Sistema operativo (`darwin`, `linux`, `windows`) |
| `arch` | string | | Architettura CPU (`arm64`, `amd64`) |
| `archetipo_version` | string | | Versione della CLI ARchetipo |
| `session_id` | string | | Identificativo anonimo di sessione |
| `timestamp` | string | | Timestamp RFC3339 dell'evento |
| `duration_ms` | number | | Durata dell'operazione in millisecondi |
| `success` | boolean | | `true` se l'operazione è riuscita |
| `error_code` | string | | Codice errore in caso di fallimento |
| `connector` | string | | Connector utilizzato (`file`, `github`, `jira`) |
| `spec_code` | string | | Codice spec coinvolta (es. `"US-001"`) |
| `properties` | object | | Mappa libera di proprietà aggiuntive |

### Campi vietati

I seguenti campi sono **rigettati** a priori per motivi di sicurezza e privacy,
anche se presenti nel JSON:

`path`, `cwd`, `hostname`, `username`, `user`, `email`, `token`, `password`,
`secret`, `key`, `api_key`, `credential`, `credentials`, `env`, `environment`,
`home`, `homedir`, `ip`, `ip_address`, `machine_id`, `device_id`, `mac`,
`mac_address`

Qualsiasi campo non presente nella tabella "Schema evento" e non nella lista
dei campi vietati è considerato **sconosciuto** e produce un errore `400`.

## Codici di stato

### `202 Accepted`

L'evento (o batch) è stato accettato e verrà processato.

**Body risposta:**
```json
{
  "status": "accepted"
}
```

### `400 Bad Request`

La richiesta non è valida. Possibili cause:

- **Schema mancante**: il campo `schema` è assente
- **Schema errato**: il campo `schema` non è `"archetipo.analytics/v1"`
- **Campo vietato**: il JSON contiene un campo nella denylist
- **Campo sconosciuto**: il JSON contiene un campo non riconosciuto
- **Evento mancante**: il campo `event` è assente
- **JSON non valido**: il body non è JSON valido
- **Content-Type errato**: l'header Content-Type non è `application/json`

**Body risposta:**
```json
{
  "error": "validation_error",
  "detail": "field \"hostname\" is forbidden for privacy/security reasons"
}
```

In caso di batch, l'intero batch viene rigettato se anche un solo evento non è
valido (fail-atomic: nessun evento del batch viene archiviato).

### `429 Too Many Requests`

Il rate limit per l'origine è stato superato.

**Header:**
```
Retry-After: <secondi>
```

**Body risposta:** _(vuoto)_

## Rate Limiting

Ogni risposta include gli header di rate limiting:

| Header | Descrizione |
|--------|-------------|
| `X-RateLimit-Limit` | Numero massimo di richieste per finestra |
| `X-RateLimit-Remaining` | Richieste rimanenti nella finestra corrente |
| `X-RateLimit-Reset` | Timestamp UNIX di reset della finestra |

**Default:** 60 richieste/minuto con burst di 10.

Il rate limiting è per origine (IP del client). L'IP raw **non** viene mai
persistito oltre i log tecnici minimi necessari al funzionamento del rate
limiting. Lo storage persistente non contiene mai l'IP del client.

## Note sulla privacy

- **Nessuna autenticazione**: l'endpoint non richiede token, API key o segreti
- **IP anonimizzato**: l'IP del client non è mai archiviato nello storage.
  Il rate limiter usa internamente un hash dell'IP come chiave
- **TTL log tecnici**: i log grezzi contenenti IP vengono eliminati entro 7 giorni
- **Solo campi allowlist**: vengono accettati esclusivamente i campi documentati
  nella tabella schema. Campi sconosciuti o potenzialmente sensibili sono rigettati

## Esempi

### Evento valido

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "schema": "archetipo.analytics/v1",
    "event": "cli.invocation",
    "tool": "claude-code",
    "tool_version": "1.2.0",
    "os": "darwin",
    "arch": "arm64",
    "archetipo_version": "1.0.0",
    "success": true,
    "duration_ms": 150,
    "connector": "file"
  }'
```

**Risposta:** `202 Accepted`
```json
{"status":"accepted"}
```

### Batch valido

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -d '[
    {"schema":"archetipo.analytics/v1","event":"cli.invocation","tool":"test"},
    {"schema":"archetipo.analytics/v1","event":"spec.created","tool":"test"}
  ]'
```

**Risposta:** `202 Accepted`
```json
{"status":"accepted"}
```

### Evento con campo vietato

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -d '{"schema":"archetipo.analytics/v1","event":"x","hostname":"myhost"}'
```

**Risposta:** `400 Bad Request`
```json
{
  "error": "validation_error",
  "detail": "field \"hostname\" is forbidden for privacy/security reasons"
}
```

### Evento con campo sconosciuto

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -d '{"schema":"archetipo.analytics/v1","event":"x","custom_field":"value"}'
```

**Risposta:** `400 Bad Request`
```json
{
  "error": "validation_error",
  "detail": "field \"custom_field\" is not recognized in schema archetipo.analytics/v1"
}
```

### Rate limit superato

```bash
# Dopo aver inviato più di 60 richieste in un minuto:
curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -d '{"schema":"archetipo.analytics/v1","event":"x"}'
```

**Risposta:** `429 Too Many Requests`
_Header:_ `Retry-After: 45`
