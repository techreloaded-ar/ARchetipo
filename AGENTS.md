# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Progetto

ARchetipo è un set di skill per AI coding agent (Claude Code, Codex, Cursor, Gemini CLI, OpenCode, GitHub Copilot) che supportano il processo di ideazione, analisi e pianificazione di un progetto software.

## Struttura del repository

```
skills/                  # Skill principali (una dir per skill)
  <skill-name>/
    SKILL.md             # Definizione della skill
    references/          # File di supporto caricati dalla skill
skills-extra/            # Skill extra (stessa struttura)
.archetipo/              # File installati nel progetto target (mirror della struttura target)
  config.yaml            # Template di configurazione per il progetto target
  shared-runtime.md      # Regole condivise (Language Policy, Persona, ecc.)
cli/                     # Modulo Go che implementa la CLI `archetipo`
  cmd/archetipo/         # Entry point del binario
  internal/
    cli/                 # Sub-comandi cobra (superficie pubblica della CLI)
    domain/              # Tipi dati condivisi
    connector/           # Interfaccia + due implementazioni (filefs, github)
    config/              # Loader di .archetipo/config.yaml
    iox/                 # Envelope JSON stdin/stdout/stderr
npm/                     # Pacchetto npm (@techreloaded/archetipo + 6 sub-package per piattaforma)
scripts/                 # Build e publish dei pacchetti npm
```

## Architettura connector

Le skill non gestiscono direttamente la persistenza e non eseguono operazioni di connector "interpretando" istruzioni. Il flusso è sempre:

1. La skill legge `.archetipo/shared-runtime.md` per envelope JSON, regole sugli errori e disciplina di invocazione.
2. La skill invoca `archetipo <subcmd>` (binario Go installato globalmente via `npm i -g @techreloaded/archetipo`).
3. La CLI legge `.archetipo/config.yaml` per scegliere il connector (`file` o `github`) ed esegue l'operazione in modo deterministico.

Le skill devono incorporare esplicitamente i sub-comandi CLI che usano davvero, con i relativi payload, envelope attesi ed `error.code` rilevanti. Non esiste un file separato che descrive l'intero protocollo.

## Regole per skill author

- Chiama solo i sub-comandi che la skill usa realmente.
- I template di contenuto (PRD, body delle storie, plan body, body dei sub-issue) sono prodotti dalla skill e passati alla CLI via stdin. La CLI persiste il payload, non lo arricchisce.
- La logica di validazione e post-processing degli output JSON va nella skill.
- I sub-comandi no-op sono espliciti: per esempio `comment post` ritorna `ok: true` anche con `connector: file`. La skill non deve mai ramificare sul tipo di connector.
- Branch sull'`error.code` del JSON envelope, non sul `message`.
- Caricare `.archetipo/shared-runtime.md` **una sola volta** all'avvio della skill.

## Regole per chi modifica la CLI

- Le 13 operazioni pubbliche della CLI sono stabili: ogni cambiamento incompatibile è un breaking change e va versionato.
- Mantenere la conformance suite (`cli/internal/connector/conformance/`) verde su tutte le implementazioni: file, github, inmemory.
- Tutte le query GraphQL del connector github vivono in `cli/internal/connector/github/templates.go`. Aggiungere snapshot test prima di modificarle.
- Distribuzione: il binario è versionato insieme alle skill (un solo tag per repo). Su tag `v*` il workflow `release.yml` esegue GoReleaser per produrre le binary in `cli/dist/`, poi `scripts/build-npm.mjs` sincronizza le binary nei 6 sotto-pacchetti `@techreloaded/archetipo-{os}-{arch}` e le skill nel pacchetto principale `@techreloaded/archetipo`, infine `scripts/publish-npm.mjs` pubblica tutti i 7 pacchetti su npm.
- **Prima di consegnare modifiche**, esegui gli stessi controlli della CI in locale per evitare build rossi:

  ```bash
  cd cli
  gofmt -l .          # deve essere vuoto
  go vet ./...        # nessun errore
  go build ./...      # compilazione pulita
  go test ./...       # tutti i test passano
  golangci-lint run --timeout 5m ./...   # 0 issues
  ```

  Se `golangci-lint` non è installato: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`

## Test E2E (`test/e2e/`)

Questa repo ha una harness E2E locale in Node.js che esercita la CLI compilata dal sorgente e, per alcuni scenari, un AI agent reale.

### Runner principale

- Comando: `npm run test:e2e` (equivale a `node ./test/e2e/run.mjs`).
- Opzioni utili: `--scenario <id>` / `--scenarios <id1,id2>`, `--config <path>`, `--timeout-ms <ms>`.
  - Esempio: `npm run test:e2e -- --scenario worktree-from-plan-to-implement-integrate`.
- Il runner compila sempre la CLI Go in `test/e2e/.bin/archetipo` (`go build -o ... ./cmd/archetipo`), quindi richiede Go installato.
- Per ogni scenario crea un sandbox sotto `test/workspaces/<scenario>/runs/<timestamp>/sandbox`, copia lì la CLI in `sandbox/bin/`, setta `ARCHETIPO_DATA_DIR` alla root repo e mette `sandbox/bin` in `PATH`.
- Ogni run produce `report.html` e `summary.json` nella directory run. I workspace e la binaria E2E sono generati/ignorati (`test/workspaces/*`, `test/e2e/.bin/`) e non vanno committati.
- Timeout default: 20 minuti per step; heartbeat ogni 30 secondi per step lunghi.
- Errori di auth/credenziali nei comandi vengono classificati come `skip`; fallimenti reali o timeout come `fail`.

### Formato `run.yaml`

`test/e2e/run.yaml` contiene due sezioni:

- `agents`: definisce backend eseguibili con `tool`, `command`, `model`, `args` e opzionale `env_required`.
  - `args` supporta interpolazione `{model}`, `{prompt}`, `{sandboxDir}`.
  - Tool supportati e root skill installate: `claude -> .claude/skills`, `codex -> .agents/skills`, `gemini -> .gemini/skills`, `opencode -> .opencode/skills`, `copilot -> .github/skills`, `pi -> .pi/skills`.
- `scenarios`: ogni scenario punta a un agent e può avere:
  - `fixture`: directory da sovrapporre al sandbox dopo `archetipo init`.
  - `prompts`: prompt/skill da invocare tramite agent; il nome skill è derivato dal prefisso `/...` e usato anche per verificare che l'installazione abbia copiato la skill.
  - `env_required`: override dei requisiti env dell'agent.
  - `archetipo_pre_commands`: comandi CLI eseguiti prima dei prompt.
  - `archetipo_post_commands`: comandi CLI eseguiti dopo i prompt.
  - `verify_integrate`: codici spec per cui verificare l'integrazione worktree.
  - `verify_wiki_bootstrap`: aspettative su pagine core, PRD archiviato, stati draft/needs-review e contenuti mirati; esegue anche `wiki validate --profile bootstrap`.
- I pre/post command sono divisi con `line.split(/\s+/)`: evitare argomenti che richiedono quoting shell complesso.

### Sequenza di uno scenario `run.mjs`

1. Verifica che `agent.command` esista e che le variabili `env_required` siano presenti.
2. Esegue `archetipo init --tool <tool> --connector file --yes` nel sandbox come baseline non interattiva.
3. Verifica `.archetipo/config.yaml`, `.archetipo/shared-runtime.md` e le skill richieste dai prompt.
4. Se presente, sovrappone la fixture. La `.archetipo/config.yaml` della fixture è autoritativa: decide connector, worktree, paths, ecc.; non aggiungere flag al runner per questo.
5. Inizializza un repo git nel sandbox su branch `main`, configura identità locale e crea un commit base vuoto.
6. Esegue eventuali `archetipo_pre_commands` usando sempre la CLI copiata nel sandbox.
7. Esegue i prompt tramite agent con args interpolati.
8. Per `verify_integrate`, cattura prima dei post-command branch/worktree/tip con `spec show` + `git rev-parse`.
9. Esegue eventuali `archetipo_post_commands`.
10. Verifica l'integrazione: spec in `DONE`, tip pre-integrate raggiungibile da `main`, branch per-spec cancellato, worktree directory rimossa e non più presente in `git worktree list --porcelain`.

### Scenari attuali

- `inception-creates-valid-prd`: fixture `fixtures/inception`, prompt `/archetipo-inception`, poi `validate prd`; verifica che la skill generi e persista un PRD strutturalmente valido.
- `wiki-bootstrap-codebase-only`: fixture `fixtures/wiki-codebase`, prompt `/archetipo-wiki`; verifica una mappa codebase-first completa senza PRD e senza promozione dei draft.
- `wiki-bootstrap-prd-conflict`: fixture `fixtures/wiki-prd-conflict`, prompt `/archetipo-wiki`; verifica archiviazione del PRD, autorità del codice sullo stato corrente e conflitto marcato `needs-review`.
- `from-prd-to-plan`: fixture `fixtures/prd`, prompt `/archetipo-spec` e `/archetipo-plan US-001`; copre PRD -> backlog/spec -> piano.
- `jira-init`: fixture `fixtures/jira-prd`, attualmente senza prompt; usa config connector `jira`.
- `from-plan-to-implement`: fixture `fixtures/plan`, prompt `/archetipo-implement US-001`; worktree disabilitato.
- `worktree-from-plan-to-implement-integrate`: fixture `fixtures/worktree-plan`, prompt `/archetipo-implement US-001`, poi `spec integrate US-001` e verifica integrazione.
- `worktree-implement-no-integrate`: fixture `fixtures/worktree-plan`, pre-command `spec start US-001`, poi `/archetipo-implement US-001`; lascia il lavoro senza integrazione.

### Fixture disponibili

- `fixtures/inception`: connector `file`, worktree disabilitato, senza PRD iniziale; usata per verificare la generazione via `/archetipo-inception`.
- `fixtures/wiki-codebase`: piccolo servizio TypeScript/Express senza PRD, con route e test, usato per il bootstrap Wiki codebase-first.
- `fixtures/wiki-prd-conflict`: servizio TypeScript/Express con PRD intenzionalmente incoerente (Python/FastAPI/MongoDB), usato per verificare la gestione dei conflitti.
- `fixtures/prd`: connector `file`, worktree disabilitato, PRD `docs/PRD.md` sul prodotto match5.
- `fixtures/plan`: connector `file`, worktree disabilitato, backlog/spec/plan `US-001` che chiede di creare `hello.txt` con `Hello from ARchetipo`.
- `fixtures/worktree-plan`: come `plan`, ma con `worktree.enabled: true`, `base: main`, `dir: .archetipo/worktrees`, `branch_prefix: archetipo/`.
- `fixtures/jira-prd`: connector `jira`, `base_url: https://agilereloaded.atlassian.net/`, `story_type: Task`, `subtask_type: Sub-task`, `priority_map`; `project_key` e `status_map` sono omessi intenzionalmente per lasciare alla CLI auto-discovery/auto-matching.

### Smoke test standalone

- `node ./test/e2e/validate-inception-smoke.mjs`: compila CLI, inizializza sandbox file/pi, scrive un PRD invalido, verifica `archetipo validate prd` con exit `0`, `kind=validation_result` e `data.ok=false` includendo `PRD_PLACEHOLDER_LEFT` e `PRD_MISSING_SECTION`, poi scrive un PRD valido e verifica `kind=validation_result` con `data.ok=true`. Produce report HTML. Opzioni: `--workspace-root`, `--cleanup`. Nota: l'help cita `npm run test:validate-inception`, ma al momento non c'è uno script package corrispondente.
- `npm run test:view-delete-smoke`: compila CLI, inizializza sandbox, aggiunge due spec, semina plan/review per `US-901`, avvia `archetipo view` su porta random e verifica via HTTP API che `DELETE /api/spec/US-901` rimuova la card, lasci `US-902`, restituisca poi 404 su `US-901` e cancelli spec/plan/review artifact.
- `npm run test:wiki-smoke`: compila la CLI, ispeziona una codebase sandbox, inizializza la Wiki, crea una pagina draft e verifica validate, search, catalog senza promozione, publish a `verified` e rigenerazione di `index.md`.

Quando aggiungi o modifichi E2E, preferisci fixture esplicite con `.archetipo/config.yaml` completa, usa `env_required` per credenziali esterne, mantieni i report generati fuori dal commit e aggiorna questa sezione se cambia la semantica del runner.

## Installazione (per utenti finali)

Percorso principale (qualsiasi sistema con Node.js):

```bash
npm i -g @techreloaded/archetipo     # CLI globale nel PATH
archetipo init [--tool …] [--connector …]
```

Lo shim Node in `npm/archetipo/bin/archetipo.js` risolve il sub-package binario per la piattaforma corrente, setta `ARCHETIPO_DATA_DIR` e spawna la binary Go. Le skill bundle sono in `npm/archetipo/skills/` e vengono copiate da `archetipo init` verso `.{tool}/skills/` nel progetto.

## Note operative

- `.archetipo/config.yaml` in questo repo è un **template**: viene copiato nel progetto target dell'utente sotto `.archetipo/config.yaml`
- Il connector `file` è il default e usa file markdown locali. Il connector `github` richiede `gh` CLI autenticato
- Per i test E2E locali vedi la sezione `Test E2E (test/e2e/)` sopra
