# ARchetipo — Documento dei Requisiti di Prodotto

**Autore:** ARchetipo
**Data:** 2026-06-19
**Versione:** 1.0

---

## Elevator Pitch

> Per **sviluppatori e product team che usano AI coding agent**, che hanno il problema di **mancanza di struttura, persistenza e tracciabilità nel processo di sviluppo**, **ARchetipo** è un **workflow spec-driven deterministico** che **trasforma ogni AI coding agent in una squadra disciplinata di prodotto, con ruoli specializzati, artefatti durevoli e transizioni di stato deterministiche**. A differenza di **prompt manuali non strutturati e checklist su file markdown**, il nostro prodotto **esegue le operazioni di workflow tramite una CLI Go deterministica con connector intercambiabili (file, GitHub, Jira), garantendo che lo stesso processo funzioni allo stesso modo su Claude Code, Codex, Gemini CLI, OpenCode e GitHub Copilot — senza dipendere da un tool AI specifico**.

---

## Visione

ARchetipo rende lo sviluppo software assistito da AI un processo deliberato, tracciabile e ripetibile quanto lo sviluppo tradizionale, senza rinunciare alla velocità dell'automazione. L'obiettivo è che ogni progetto software costruito con AI abbia una memoria persistente del proprio percorso: dal PRD iniziale fino all'ultima spec in DONE, con ogni transizione di stato registrata e ogni decisione architetturale documentata.

### Differenziatore di Prodotto

ARchetipo non è un prompt system: è un **motore deterministico**. La CLI Go (`archetipo`) è l'unico backend per tutte le operazioni di persistenza. Le skill (file markdown) guidano l'agente AI attraverso fasi standardizzate, ma le operazioni di I/O passano sempre dalla CLI. Questo garantisce:

1. **Portabilità tra tool AI**: lo stesso workflow funziona identico su Claude Code, Codex, Gemini CLI, OpenCode e GitHub Copilot.
2. **Connector intercambiabili**: il motore astrae la persistenza (file locale, GitHub Issues+Projects, Jira) — il processo di prodotto non cambia.
3. **Determinismo**: ogni transizione di stato è eseguita dalla CLI, non interpretata dall'agente. I contratti di errore sono tipizzati (`error.code`), non basati su messaggi.
4. **Lingua adattiva**: le skill rilevano automaticamente la lingua della conversazione e generano artefatti nella stessa lingua.

---

## Personas Utente

### Persona 1: Marco — Sviluppatore Indipendente

**Ruolo:** Full-stack developer freelance
**Età:** 31 | **Background:** Sviluppa progetti personali e per clienti, usa Claude Code quotidianamente.

**Obiettivi:**
- Trasformare idee grezze in software funzionante con un processo chiaro
- Mantenere traccia di cosa è stato fatto e cosa manca senza uscire dall'AI agent
- Avere artefatti di progetto (PRD, backlog, piani) versionabili con git
- Poter interrompere e riprendere il lavoro senza perdere il contesto

**Punti di Dolore:**
- Ogni nuova sessione di chat con l'AI agent riparte da zero: il contesto si perde
- I prompt isolati non hanno memoria del "perché" delle scelte fatte
- Man mano che il progetto cresce, tenere a mente lo stato di ogni feature diventa ingestibile
- Non esiste un processo di review strutturato: il codice va in produzione senza un gate umano chiaro

**Comportamenti e Strumenti:**
- Lavora in solo, spesso in sessioni da 2-4 ore
- Usa git per versionare tutto, preferisce file locali a servizi cloud
- Scrive prompt in italiano e vuole risposte in italiano
- Alterna fasi di sviluppo intenso a pause di giorni — ha bisogno di riprendere velocemente

**Motivazioni:** Autonomia, qualità del codice, soddisfazione di vedere il progetto crescere in modo ordinato
**Competenza Tecnologica:** Alta (sviluppatore professionista)

#### Percorso Cliente — Marco

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Scoperta | Legge di ARchetipo su Twitter/Reddit | "Un workflow per Claude Code? Finalmente qualcuno ci ha pensato" | Curiosità | Landing page chiara con esempio di flusso completo in 5 minuti |
| Valutazione | Installa con `npm i -g @techreloaded/archetipo` e lancia `archetipo init` | "Vediamo se funziona davvero o è solo fumo" | Scetticismo | `archetipo init` deve funzionare al primo colpo e mostrare subito valore |
| Primo Utilizzo | Lancia `/archetipo-inception` su un'idea che ha in mente da mesi | "Wow, mi sta facendo le domande giuste per definire il prodotto" | Sorpresa positiva | L'inception deve essere conversazionale ma concreta, producendo un PRD utilizzabile |
| Uso Regolare | Lancia `/archetipo-spec`, poi `/archetipo-plan US-001`, poi `/archetipo-implement US-001` | "Ogni comando lascia qualcosa di concreto. So sempre a che punto sono" | Fiducia | Il loop spec→plan→implement deve girare senza attriti; `archtetipo view` mostra lo stato |
| Advocacy | Mostra il progetto a un collega: "Guarda, ogni feature ha il suo piano e i test sono già scritti" | "Questo è il modo giusto di lavorare con l'AI" | Orgoglio | Il Kanban locale e la history delle transizioni raccontano una storia di progetto professionale |

---

### Persona 2: Sofia — Tech Lead / Product Owner

**Ruolo:** Tech lead in una startup di 8 persone
**Età:** 37 | **Background:** Coordina 3 sviluppatori che usano AI agent (Codex e Copilot). Deve garantire qualità e allineamento.

**Obiettivi:**
- Avere visibilità sullo stato di tutte le feature in sviluppo
- Garantire che ogni incremento passi attraverso un gate di review strutturato
- Poter tracciare l'avanzamento del backlog (velocity, colli di bottiglia)
- Integrare il workflow AI con il project tracking che il team già usa (GitHub Projects / Jira)

**Punti di Dolore:**
- Gli sviluppatori usano AI agent in modo non coordinato: ognuno ha il suo flusso, impossibile avere una visione d'insieme
- Il codice prodotto da AI agent arriva senza test strutturati né documentazione delle scelte
- Non sa quali feature sono bloccate, quali in review, quali pronte per il rilascio
- Il passaggio da "AI ha scritto il codice" a "il codice è in produzione" è opaco e rischioso

**Comportamenti e Strumenti:**
- Usa GitHub Projects o Jira per il tracking di sprint
- Fa code review su GitHub
- Tiene sync settimanali di allineamento
- Scrive in inglese con il team, ma alcuni sviluppatori preferiscono l'italiano

**Motivazioni:** Qualità del prodotto, prevedibilità delle delivery, crescita del team
**Competenza Tecnologica:** Medio-alta (viene dallo sviluppo, ora più lato gestione)

#### Percorso Cliente — Sofia

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Scoperta | Un membro del team le mostra ARchetipo dopo averlo provato | "Interessante, ma funziona con il nostro stack (GitHub + Jira)?" | Interesse cauto | Documentazione chiara sui connector GitHub e Jira |
| Valutazione | Configura `.archetipo/config.yaml` con `connector: github` e verifica che le spec appaiano come issue | "Ok, le spec sono issue reali su GitHub Projects. I miei PM possono vederle" | Sollievo | La trasparenza cross-tool (AI agent ↔ GitHub) è il fattore decisivo |
| Primo Utilizzo | Assegna una spec a uno sviluppatore: "Usa `/archetipo-implement US-005`" | "Lo sviluppatore esegue il comando e la issue si muove da PLANNED a IN PROGRESS automaticamente" | Fiducia crescente | La transizione automatica degli stati tra AI agent e GitHub Projects elimina l'attrito |
| Uso Regolare | Usa `archetipo metrics` prima dello sprint review per vedere completamento, WIP, cycle time | "Ho i numeri che mi servono senza chiedere a nessuno" | Controllo | Le metriche devono essere sufficienti per uno sprint review (non un sostituto di Jira) |
| Advocacy | Propone di adottare ARchetipo in un altro team: "Abbiamo ridotto il tempo tra idea e PR del 40%" | "Questo non è solo un tool per sviluppatori: è un processo di prodotto" | Convinta | Casi studio e metriche comparabili tra team |

---

## Insight dal Brainstorming

### Assunzioni Sfidate

Durante l'analisi del repository e del posizionamento esistente, sono emerse alcune assunzioni che meritano validazione:

1. **"Gli sviluppatori vogliono un processo strutturato"** — L'assunzione è che la struttura non venga percepita come overhead. Cosa succede se lo sviluppatore vuole solo "scrivere codice" e salta le fasi di plan? ARchetipo lo consente (le skill sono indipendenti), ma il valore pieno emerge solo con il flusso completo. È una scelta deliberata: flessibilità senza imporre cerimonie.
2. **"File locale è sufficiente per l'adozione iniziale"** — L'assunzione è che il connector `file` sia il punto di ingresso principale. Cosa succede se gli utenti si aspettano subito l'integrazione con GitHub/Jira? Il connector `file` azzera l'attrito iniziale (zero configurazione), ma il connector `github` è già disponibile.
3. **"Le skill come file markdown sono abbastanza espressive"** — L'assunzione è che le skill non abbiano bisogno di un runtime engine oltre alla CLI Go. Cosa succede se le skill diventano troppo complesse per essere descritte in markdown? Finora il design regge: la CLI è il motore, le skill sono la guida.

### Nuove Direzioni Scoperte

- **Worktree workflow**: Isolare ogni spec in un git worktree dedicato permette di eseguire più spec in parallelo senza conflitti. Questa feature (già implementata) è un differenziatore tecnico forte.
- **Autopilot**: L'esecuzione batch di plan+implement su più spec per volta trasforma ARchetipo da assistente interattivo a orchestratore autonomo.
- **Kanban locale via web server**: `archetipo view` avvia un server HTTP locale con una board Kanban interattiva — non serve un servizio cloud per visualizzare lo stato del progetto.
- **Auto-detection della lingua**: Le skill adottano automaticamente la lingua del progetto (PRD, backlog) o della conversazione, rendendo il prodotto naturale per team multilingue.

### Assunzioni da Validare

1. **Curva di apprendimento**: Quanto tempo serve a un nuovo utente per passare da `archetipo init` alla prima spec in `DONE`?
2. **Affidabilità su progetti esistenti**: ARchetipo funziona bene su greenfield. Su progetti legacy con migliaia di file, le skill di implementazione potrebbero aver bisogno di più contesto.
3. **Collaborazione multi-agente**: Più sviluppatori possono usare ARchetipo sullo stesso repository simultaneamente con il connector `file`? Il worktree workflow aiuta, ma il backlog file è un singolo YAML — potenziali conflitti di merge.
4. **Costo dei subagent**: Skill come `archetipo-implement` e `archetipo-autopilot` usano subagent. Su tool che non li supportano (Codex, Copilot), la qualità dell'output potrebbe degradare. **Domanda aperta**: qual è il subset di skill che garantisce la stessa qualità in assenza di subagent?

### Rischi Principali

| Rischio | Impatto | Probabilità | Mitigazione |
|---|---|---|---|
| **Adozione**: Gli sviluppatori percepiscono il workflow come overhead e abbandonano dopo il primo utilizzo | Alto | Media | Ogni skill è indipendente. Un utente può usare solo inception+implement, o solo plan+implement. Il valore incrementale deve essere visibile fin dal primo comando. |
| **Frammentazione tool AI**: Ogni tool AI (Claude Code, Codex, Gemini, Copilot) evolve in modo diverso. Le skill potrebbero rompersi su un tool specifico. | Medio | Alta | Le skill usano la CLI Go come unico backend. Il formato skill è puro markdown. Il rischio è mitigato dal disaccoppiamento: se un tool cambia il suo formato skill, va aggiornata solo la parte di installazione (`archetipo init`). |
| **Qualità dell'output AI**: La qualità del codice generato dipende dal modello sottostante. ARchetipo può guidare il processo ma non garantire la correttezza del codice. | Medio | Alta | Il gate di review umana (`/archetipo-review`) è l'ultima difesa. Inoltre, i piani di test (Mina) e la code review (Cesare) sono integrati nel flusso di implementazione. |
| **Concorrenza**: Altri tool simili (spec-driven workflow per AI) potrebbero emergere con funzionalità sovrapposte. | Basso | Media | Il differenziatore è il motore deterministico + connector multipli + agnosticismo rispetto al tool AI. Non è solo un set di prompt. |
| **Manutenzione della CLI Go**: La codebase della CLI deve rimanere stabile e testata. Breaking change nei connector impattano tutte le skill. | Alto | Bassa | La conformance suite (`internal/connector/conformance/`) garantisce che ogni implementazione del connector rispetti il contratto. Build e test sono automatizzati. |

---

## Ambito di Prodotto

### MVP — Minimum Viable Product

L'MVP attuale (già implementato) include:

- **CLI Go (`archetipo`)** con 17 sub-comandi che coprono l'intero ciclo spec-driven: init, config, view, prd write, spec (list, add, show, next, plan, start, review, request-changes, update, integrate, move), task done, metrics, doctor, version, update, uninstall.
- **Sette skill AI**:
  - `archetipo-inception`: Product discovery → PRD
  - `archetipo-spec`: PRD → backlog (creazione o estensione)
  - `archetipo-plan`: Spec → piano tecnico con task, dipendenze, strategia test
  - `archetipo-implement`: Piano → codice, test, review (team Ugo/Mina/Cesare)
  - `archetipo-review`: Review → DONE (gate umano) o rework
  - `archetipo-design`: Mockup HTML/CSS isolati in `docs/mockups/`
  - `archetipo-autopilot`: Esecuzione batch plan+implement su più spec
- **Connector `file`**: PRD, backlog, piani e review come file YAML/MD locali.
- **Connector `github`**: Backlog e piani come GitHub Issues + Projects v2, con transizioni di stato automatiche.
- **Worktree workflow**: Branch git + worktree isolato per ogni spec (opzionale, configurabile).
- **Kanban locale**: `archetipo view` avvia un web server su `127.0.0.1` con board Kanban interattiva e live reload.
- **Lingua adattiva**: Auto-detection italiano/inglese in tutte le skill.
- **Metriche**: `archetipo metrics` riporta avanzamento backlog, cycle/lead time, WIP, rework.
- **Subagent nativo**: Supporto per agenti isolati su Claude Code, Gemini CLI, Roo Code.

### Funzionalità di Crescita (Post-MVP)

- **Connector Jira**: Integrazione nativa con Jira Cloud REST API v3 per team enterprise.
- **Metriche avanzate**: Velocity predittiva, burn-down chart, analisi dei colli di bottiglia per epic.
- **UI Kanban estesa**: Drag-and-drop per riordinare le spec, editor inline del corpo spec, visualizzazione diff dalla board.
- **Template di skill personalizzabili**: Permettere ai team di estendere o sovrascrivere le skill predefinite per adattare il workflow.
- **Webhook / notifiche**: Notificare Slack/Discord quando una spec entra in REVIEW o DONE.
- **Report di sprint**: Generazione automatica di release notes e changelog dalle spec completate.

### Visione (Futuro)

- **Marketplace di skill**: Skill community-driven per domini specifici (sicurezza, accessibilità, compliance, mobile).
- **Connector aggiuntivi**: Linear, Notion, Trello, Azure DevOps.
- **Orchestrazione multi-agente**: Esecuzione parallela di più spec su worktree isolati con un singolo comando.
- **Agenti AI addestrati sul dominio**: Skill che includono conoscenza specifica di framework/librerie per migliorare la qualità dell'implementazione.
- **Integrazione CI/CD nativa**: Esecuzione automatica di test e metriche nella pipeline CI con commenti sulla PR.
- **Dashboard di progetto**: Vista aggregata multi-progetto per team che usano ARchetipo su più repository.

---

## Architettura Tecnica

> **Proposta da:** Leonardo (Architetto)

### Architettura del Sistema

ARchetipo è composto da tre strati principali:

1. **Skill Layer (Markdown)**: File `.md` che guidano l'AI agent attraverso fasi standardizzate del workflow di prodotto. Ogni skill carica il runtime condiviso, invoca comandi CLI specifici e produce artefatti persistenti.
2. **CLI Layer (Go)**: Binario deterministico che implementa tutte le operazioni pubbliche del workflow. Espone 17+ sub-comandi via Cobra. La CLI è l'unico backend per la persistenza: le skill non scrivono mai file direttamente.
3. **Connector Layer (Go interface)**: Astrae la persistenza. Tre implementazioni (filefs, github, jira) più una reference implementation in-memory per i test. Il connector è selezionato via `.archetipo/config.yaml`.

**Pattern Architetturale:** CLI monolitica con architettura a plugin (connector registry pattern). Le skill sono il "frontend conversazionale", la CLI è il "backend deterministico", i connector sono il "data layer".

**Componenti Principali:**

- **`cmd/archetipo/`**: Entry point del binario.
- **`internal/cli/`**: Comandi Cobra (uno per ogni operazione pubblica). Ogni comando è un file Go indipendente.
- **`internal/connector/`**: Interfaccia `Connector` + registry + implementazioni.
  - `filefs/`: Connector locale (YAML + Markdown).
  - `github/`: Connector GitHub (gh CLI + GraphQL).
  - `jira/`: Connector Jira Cloud (REST API v3).
  - `inmemory/`: Implementazione di riferimento per la conformance suite.
  - `conformance/`: Suite di test comportamentali condivisa tra tutte le implementazioni.
  - `specmeta/`: Logica condivisa per la validazione delle spec.
- **`internal/config/`**: Loader di `.archetipo/config.yaml` con fallback ai default.
- **`internal/domain/`**: Tipi dati canonici (Spec, Task, PlanInput, SetupInfo, ecc.) — agnostici rispetto al connector.
- **`internal/iox/`**: Envelope JSON su stdin/stdout/stderr + errori tipizzati.
- **`internal/gitwt/`**: Worktree workflow (branch + worktree per spec, diff, integrazione).
- **`internal/web/`**: Server HTTP per la board Kanban locale (`archetipo view`), con API REST e live reload via filesystem watcher.
- **`internal/metrics/`**: Calcolo metriche di backlog (completamento, cycle/lead time, WIP, rework).
- **`internal/version/`**: Versione del binario e notificatore di aggiornamento.

### Stack Tecnologico

| Livello | Tecnologia | Versione | Motivazione |
|---|---|---|---|
| Linguaggio | Go | 1.26 | Binario singolo, cross-compilazione nativa, performance. Ideale per una CLI distribuita come binario pre-compilato. |
| CLI Framework | Cobra | 1.10.2 | Standard de facto per CLI Go. Sub-comandi, flag, help auto-generato. |
| YAML | `gopkg.in/yaml.v3` | 3.0.1 | Parsing di config e file di artefatti. |
| File Watcher | `fsnotify` | 1.10.1 | Live reload della board Kanban. |
| Frontend Kanban | HTML/CSS/JS vanilla (embedded) | — | Serverless, zero dipendenze npm. Embedded nel binario Go via `embed.FS`. |
| Testing | `testing` stdlib + `go test` | — | Conformance suite per i connector. |
| CI/CD | GitHub Actions (GoReleaser) | — | Build cross-platform, test, lint. |
| Distribuzione | npm (7 pacchetti) | — | `@techreloaded/archetipo` + 6 sub-pacchetti per piattaforma. |
| Lint | `golangci-lint` | v2 | Qualità del codice Go. |

### Struttura del Progetto

**Pattern organizzativo:** Monorepo con moduli separati per CLI e pacchetti npm.

```text
/
├── .archetipo/               # Configurazione del workflow ARchetipo
│   ├── config.yaml           # Connector, percorsi, stati
│   └── shared-runtime.md     # Regole condivise per tutte le skill
├── .agents/skills/           # Skill disponibili per l'AI agent
│   ├── archetipo-inception/
│   ├── archetipo-spec/
│   ├── archetipo-plan/
│   ├── archetipo-implement/
│   ├── archetipo-review/
│   ├── archetipo-design/
│   └── archetipo-autopilot/
├── cli/                      # Modulo Go della CLI
│   ├── cmd/archetipo/        # Entry point
│   └── internal/             # Implementazione
│       ├── cli/              # Comandi Cobra
│       ├── config/           # Loader configurazione
│       ├── connector/        # Interfaccia + implementazioni
│       │   ├── conformance/  # Test suite condivisa
│       │   ├── filefs/       # Connector file locale
│       │   ├── github/       # Connector GitHub
│       │   ├── jira/         # Connector Jira
│       │   └── inmemory/     # Reference per test
│       ├── domain/           # Tipi dati canonici
│       ├── gitwt/            # Git worktree workflow
│       ├── iox/              # Envelope JSON I/O
│       ├── metrics/          # Metriche di backlog
│       ├── version/          # Versione e notifiche
│       └── web/              # Server Kanban locale
├── npm/                      # Pacchetti npm
│   ├── archetipo/            # Pacchetto principale + shim
│   ├── archetipo-darwin-arm64/
│   ├── archetipo-darwin-x64/
│   ├── archetipo-linux-arm64/
│   ├── archetipo-linux-x64/
│   ├── archetipo-win32-arm64/
│   └── archetipo-win32-x64/
├── scripts/                  # Build e publish npm
├── docs/                     # Documentazione (PRD, mockup, test)
└── .github/workflows/        # CI/CD
```

### Ambiente di Sviluppo

Lo sviluppo di ARchetipo richiede:

- **Go 1.26+**: Per compilare e testare la CLI.
- **Node.js 20+**: Per eseguire gli script di build e publish npm.
- **golangci-lint v2**: Per il linting del codice Go.
- **Git**: Per il versionamento e il worktree workflow.

Per contribuire alle skill:
- Le skill sono file markdown, editabili con qualsiasi editor.
- Le skill fanno riferimento a file nella stessa directory (`./references/`).
- Ogni skill ha una struttura standard: metadati YAML in testa, sezione Shared Runtime, flusso operativo.

**Strumenti richiesti:** Go 1.26, Node.js 20, git, golangci-lint v2

### CI/CD e Deployment

**Build tool:** GoReleaser (`.goreleaser.yaml`), con build per 3 OS × 2 architetture.

**Pipeline:**
1. `gofmt -l .` (deve essere vuoto)
2. `go vet ./...` (nessun errore)
3. `go build ./...` (compilazione pulita)
4. `go test ./...` (tutti i test passano)
5. `golangci-lint run --timeout 5m ./...` (0 issues)
6. Su tag `v*`: GoReleaser produce i binari → `scripts/build-npm.mjs` sincronizza nei 6 sub-pacchetti npm → `scripts/publish-npm.mjs` pubblica tutti i 7 pacchetti.

**Deployment:** Distribuzione globale via npm (`npm install -g @techreloaded/archetipo`). Lo shim Node.js in `npm/archetipo/bin/archetipo.js` risolve il sub-pacchetto binario corretto per la piattaforma e spawna la binary Go.

**Infrastruttura target:** npm registry (distribuzione), GitHub Releases (binari), GitHub Container Registry (opzionale per ambienti containerizzati).

### Decisioni Architetturali (ADR)

1. **CLI Go monolitica**: Invece di un server o di una libreria, un singolo binario CLI Go è la scelta più portabile e a basso attrito per un workflow che deve funzionare offline e su qualsiasi sistema. Cobra fornisce sub-comandi, flag e help consistenti.
2. **Connector come interfaccia Go + registry**: I connector sono implementazioni di un'interfaccia `Connector` standard. Il registry pattern (`init()` + `Register()`) permette di aggiungere nuovi connector senza modificare il codice della CLI. La conformance suite garantisce che ogni implementazione rispetti il contratto.
3. **Skill come Markdown (non codice)**: Le skill sono file `.md` che l'AI agent interpreta. Questo le rende indipendenti dal tool AI, facili da modificare e versionare, e riduce la superficie di attacco (nessun codice eseguibile nelle skill).
4. **Envelope JSON per I/O**: Tutta la comunicazione tra skill e CLI avviene via JSON envelope su stdin/stdout/stderr. Gli errori sono tipizzati (`error.code`) e mai basati sul messaggio. Questo permette alle skill di ramificare in modo deterministico.
5. **Worktree workflow opzionale**: Isolare ogni spec in un git worktree è potente per il parallelismo e la review, ma aggiunge complessità. È opzionale e disabilitato di default — l'utente lo attiva quando serve.
6. **Embedded web server per Kanban**: Invece di richiedere un database o un servizio esterno, `archetipo view` avvia un server HTTP su localhost. Gli asset statici (HTML/CSS/JS) sono embeddati nel binario Go. Zero dipendenze esterne per l'utente.
7. **Nessuna IA nella CLI**: La CLI Go non contiene chiamate a modelli AI. È puramente deterministica. L'AI vive solo nelle skill (interpretate dall'agente) e nei subagent.

---

## Requisiti Funzionali

### FR-01 — Inizializzazione del Progetto
`archetipo init` deve creare `.archetipo/config.yaml`, `.archetipo/shared-runtime.md` e copiare le skill nella directory del tool AI configurato (`--tool`). Deve funzionare in modalità interattiva e non interattiva.

### FR-02 — Scrittura PRD
`archetipo prd write` deve accettare il corpo del PRD via `--file` o stdin e salvarlo nel percorso configurato (`paths.prd`). Deve restituire un envelope `write_result` con `ok: true` e il riferimento al file creato.

### FR-03 — Creazione ed Estensione Backlog
`archetipo spec add --file specs.yaml` deve creare il backlog iniziale o estenderne uno esistente. In modalità estensione, deve saltare idempotentemente le spec con codice già presente (restituite in `skipped`).

### FR-04 — Visualizzazione Backlog
`archetipo spec list [--status STATUS]` deve restituire tutti gli elementi del backlog con metadati riassuntivi (codice, titolo, epic, priorità, punti, stato). Opzionalmente filtrato per stato.

### FR-05 — Dettaglio Spec e Task
`archetipo spec show US-XXX` deve restituire il corpo completo della spec e la lista dei suoi task. `data.workdir` deve contenere il percorso assoluto della worktree se attiva, altrimenti il project root.

### FR-06 — Selezione Automatica Spec
`archetipo spec next --status TODO` deve selezionare automaticamente la spec eleggibile con priorità più alta e restituirne i dettagli completi.

### FR-07 — Pianificazione Tecnica
`archetipo spec plan US-XXX --file plan.yaml` deve salvare il piano di implementazione (corpo markdown + task list) e portare la spec in stato `PLANNED`.

### FR-08 — Avvio Implementazione
`archetipo spec start US-XXX` deve portare la spec da `PLANNED` a `IN PROGRESS`. Se il worktree workflow è abilitato, deve creare branch git e worktree dedicati.

### FR-09 — Completamento Task
`archetipo task done US-XXX TASK-XX` deve marcare un singolo task come completato all'interno del piano della spec.

### FR-10 — Invio in Review
`archetipo spec review US-XXX` deve portare la spec in `REVIEW`, allegare un commento finale opzionale (`--file note.md`) e, se abilitato, eseguire auto-commit con Conventional Commit dei cambiamenti della worktree.

### FR-11 — Richiesta Modifiche (Rework)
`archetipo spec request-changes US-XXX --file feedback.json` deve riportare la spec da `REVIEW` a `TODO`, aggiungere il feedback strutturato al corpo della spec come sezione "Rework Feedback" e impostare il flag Rework.

### FR-12 — Integrazione e Completamento
`archetipo spec integrate US-XXX` deve fondere il branch worktree nel base, pulire la worktree e marcare la spec `DONE`. Disponibile solo quando il worktree workflow è attivo e la spec è in `REVIEW`.

### FR-13 — Aggiornamento Parziale Spec
`archetipo spec update US-XXX --file patch.yaml` deve applicare una patch parziale ai campi della spec (titolo, priorità, punti, scope, bloccato_da, corpo, epic, rework). Solo i campi con valore non-nil nel payload devono essere modificati.

### FR-14 — Spostamento nella Board
`archetipo spec move US-XXX --to review` deve riordinare o spostare una spec tra le colonne del workflow, opzionalmente con un'ancora di riordino (prima/dopo un'altra spec).

### FR-15 — Metriche di Progetto
`archetipo metrics` deve riportare: conteggi per stato, percentuale di completamento, dettaglio per epic, WIP attuale, conteggio rework, spec bloccate, cycle time e lead time medi (basati sulla history delle transizioni di stato).

### FR-16 — Kanban Locale
`archetipo view` deve avviare un server web su `127.0.0.1` (porta configurabile) che espone una board Kanban interattiva con API REST per board, metriche, dettaglio spec, modifica spec, salvataggio piano, spostamento card, diff, review. Deve includere live reload tramite filesystem watcher sul backlog.

### FR-17 — Aggiornamento e Disinstallazione
`archetipo update` deve aggiornare la CLI all'ultima versione. `archetipo uninstall` deve rimuovere i file installati da `archetipo init`.

### FR-18 — Diagnostica
`archetipo doctor` deve verificare: data directory, skill nel pacchetto e installate, configurazione di progetto, presenza di git e autenticazione `gh` (se pertinente).

### FR-19 — Auto-detection Lingua
Tutte le skill devono rilevare automaticamente la lingua di output (dal backlog, PRD o conversazione) e produrre artefatti nella lingua rilevata.

### FR-20 — Worktree Workflow
Quando abilitato (`worktree.enabled: true`), `archetipo spec start` deve creare un branch `archetipo/{code}` e un worktree in `.archetipo/worktrees/{code}/`. Il diff per la review deve essere calcolato come `git diff <fork_base>...<branch>`.

---

## Requisiti Non-Funzionali

### Sicurezza

- **N-SEC-01**: La CLI Go non deve mai scrivere token o segreti in `.archetipo/config.yaml`. I segreti devono essere letti da variabili d'ambiente (`JIRA_API_TOKEN`, `GITHUB_TOKEN`, `JIRA_EMAIL`).
- **N-SEC-02**: Il server Kanban (`archetipo view`) deve ascoltare solo su `127.0.0.1` (localhost), mai su interfacce pubbliche.
- **N-SEC-03**: Le skill (file markdown) non devono contenere codice eseguibile. Tutta la logica di persistenza è delegata alla CLI Go.
- **N-SEC-04**: I comandi che coinvolgono git (worktree, commit, merge) devono operare solo all'interno del repository del progetto e dei worktree creati da ARchetipo.

### Integrazioni

- **N-INT-01**: Il connector `github` deve usare la CLI `gh` autenticata con scope `repo` e `project`. Le query GraphQL devono essere mantenute in `cli/internal/connector/github/templates.go`.
- **N-INT-02**: Il connector `jira` deve usare Jira Cloud REST API v3 su un `http.Doer` iniettabile per facilitare il testing.
- **N-INT-03**: La CLI deve esporre tutte le operazioni pubbliche come sub-comandi Cobra stabili. Ogni cambiamento incompatibile è un breaking change e va versionato.
- **N-INT-04**: I binari di release devono essere firmati e distribuiti tramite npm e GitHub Releases.

### Performance

- **N-PERF-01**: I comandi CLI che leggono il backlog devono completarsi in meno di 500ms per backlog fino a 200 spec.
- **N-PERF-02**: Il server Kanban deve supportare fino a 20 client SSE simultanei per il live reload senza degrado.
- **N-PERF-03**: I comandi che interagiscono con git (worktree create, diff, merge) devono completarsi in meno di 5 secondi per repository fino a 10K file.

### Qualità

- **N-QUAL-01**: La conformance suite (`internal/connector/conformance/`) deve passare su tutte le implementazioni del connector (filefs, inmemory, github mock, jira mock).
- **N-QUAL-02**: Prima di ogni commit sulla CLI, devono passare: `gofmt -l .` (vuoto), `go vet ./...`, `go build ./...`, `go test ./...`, `golangci-lint run --timeout 5m ./...` (0 issues).
- **N-QUAL-03**: I test snapshot per le query GraphQL del connector github devono essere aggiornati prima di modificare i template.

### Usabilità

- **N-USA-01**: `archetipo init` deve funzionare al primo tentativo su macOS, Linux e Windows senza configurazione manuale.
- **N-USA-02**: Tutti i comandi CLI devono restituire messaggi di errore in inglese con codici errore tipizzati (`E_CONFIG`, `E_INPUT`, `E_CONNECTOR`, `E_PRECONDITION`) e hint in italiano o inglese in base alla lingua rilevata.
- **N-USA-03**: La documentazione di ogni skill deve dichiarare esplicitamente i sub-comandi CLI che utilizza e i codici errore su cui ramifica.

---

## Prossimi Passi

1. **Backlog** — Esegui `/archetipo-spec` per trasformare questo PRD in un backlog di spec
2. **Design** — Esegui `/archetipo-design` per mockup dell'interfaccia Kanban (se necessario)
3. **Validazione** — Verifica le assunzioni aperte con utenti reali e itera sul PRD

---

_PRD generato via ARchetipo Product Inception — 2026-06-19_
_Sessione condotta da: utente con il team ARchetipo_
