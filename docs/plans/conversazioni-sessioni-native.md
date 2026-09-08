# Conversazioni di View come sessioni native degli harness

Analisi del 2026-09-08, sul codice a `8fd5ee5`. Proposta di evoluzione, non implementazione.

Decisione confermata dall'utente: anche le skill di processo devono lavorare nella stessa conversazione dalla quale vengono invocate.

Aggiornamento di indirizzo del 2026-09-08: l'utente ha chiarito che questa è una nuova direzione di prodotto che sostituisce le precedenti scelte di business. Il piano operativo e i prompt in [sessioni-native/README.md](sessioni-native/README.md) prevalgono sulle proposte architetturali di questa prima analisi. In particolare, il tracking obbligatorio di ogni invocazione nativa di skill non è più un requisito: le azioni gestite da ARchetipo conservano la verifica degli effetti, mentre le conversazioni possono lavorare liberamente senza spec o execution. Interfacce, test e decisioni Wiki superati possono essere sostituiti; proteggere i dati esistenti non richiede conservare il vecchio comportamento.

## Obiettivo e fattibilità

View diventa un client delle sessioni native dei provider: una conversazione mantiene identità e contesto attraverso molti turni, cambi di modello/effort, periodi di inattività e riavvii di View. L'harness esegue i propri tool, legge e modifica file, usa le skill che ha effettivamente disponibili. ARchetipo conserva le responsabilità di processo, la board e la verifica degli effetti delle execution.

È fattibile per Claude Code e Codex usando i runtime già integrati. Per ARcipelago dipende dal contratto del runner e dell'hub: l'adapter attuale non dimostra resume persistente, selezione del modello per turno o discovery delle skill. Non serve introdurre un altro agent loop, una memoria costruita con prompt o un executor shell generico.

La continuità garantita è quella nativa dell'harness, compresa la sua gestione della context window e della compaction. Non equivale a reinviare letteralmente ogni token della storia a ogni messaggio. Persistenza della sessione, disponibilità del filesystem e sopravvivenza dei processi avviati sono tre proprietà distinte.

## Evidenze nel repository

| Area | Comportamento attuale | Conseguenza |
| --- | --- | --- |
| `execution.Conversationalist` | Espone soltanto apertura e chiusura; `ConversationRequest.Context` è testo | Non esiste un handle persistente per riprendere la sessione nativa |
| `web/conversation_resume.go` | Apre un nuovo ID; conserva solo testo utente/agente, comprime gli spazi e taglia a 20.000 rune | La ripresa attuale è una nuova conversazione con un estratto della precedente |
| `execution/claude/conversation.go` | `context.WithTimeout(..., cfg.Timeout)`; default 3.600 secondi | Scade anche una conversazione inattiva |
| `execution/claude/prompt.go` | Passa `--no-session-persistence` | Disabilita deliberatamente il resume nativo |
| `execution/claude/streamjson.go` | Gestisce turni e approvazioni; non acquisisce `session_id` nel tipo `frame` | Buona base interattiva, manca l'identità durevole |
| `execution/codex/provider.go` | Non implementa `Conversationalist`; esegue plan, implement e review | Codex non è ancora un provider di conversazioni libere |
| `execution/codex/appserver.go` | `ephemeral: true`; `Send` usa `turn/steer`; le richieste server sono rifiutate | Mancano persistenza, nuovi turni interattivi e ponte per le approvazioni |
| `execution/conversationprompt.go` | Vieta di agire, usare skill ARchetipo e comandi che scrivono | Il comportamento richiesto contraddice il contratto attuale |
| `web/conversation_state.go` | Modello e opzioni risolti all'apertura e conservati invariati | Il selettore non può cambiare un turno successivo |
| `conversationlog/record.go` | Salva metadati e timeline, senza riferimento alla sessione nativa | Non basta per riprendere l'harness dopo un restart |
| `web/conversation_journal.go` e `web/conversation.go` | La lettura aggiorna il journal, sostituendo `Events` con la finestra in memoria; Claude limita quella finestra a 2.000 eventi | La storia durevole dipende dalla lettura e può perdere gli eventi più vecchi |
| `web/conversation_action.go` | Associa la conversazione all'ID di una execution e la sigilla al termine | Una conversazione persistente richiede una relazione con più execution successive |
| `execution/arcipelago/conversation.go` | Crea un task remoto, mantiene la mappa verso il run ID in memoria e segue lo stream | Riaggancio dello stream e resume di una sessione terminata sono operazioni diverse |

La precedente unificazione run/conversazioni, descritta in `unificazione-run-conversazioni.md`, fornisce timeline e approvazioni riutilizzabili. Non risolve ancora la relazione fra una conversazione durevole e molte execution né il loro recupero dopo un crash.

## Modello proposto

```text
Conversazione ARchetipo — identità stabile, workspace, provider
  └─ Sessione nativa — riferimento durevole interpretato dal provider
       ├─ Turno: messaggio, modello/effort effettivi, tool, risultato
       ├─ Turno: invocazione di una skill
       └─ Turno successivo, anche dopo giorni

Execution ARchetipo — eventuale azione di processo nella conversazione
  └─ uno o più turni, ricevuta e verifica degli effetti
```

Una sessione nativa può richiedere diversi processi del runtime nel tempo. Riavviare il processo con lo stesso session ID non crea una nuova conversazione e non deve produrre un nuovo thread nella rail.

### Identità e storage

- Conservare `conversation_id`, provider originario e configurazione non segreta necessaria a ritrovarlo, directory effettiva e riferimento nativo. Per il remoto, anche workspace/runner e identificatori di task/sessione. Il riferimento può essere opaco al core; non deve contenere credenziali.
- Conservare la versione del formato e la provenienza del runtime. Il default del workspace vale per le nuove conversazioni: cambiarlo non deve spostare quelle esistenti a un altro provider o endpoint.
- Affidare all'harness la persistenza del contesto nativo; mantenere il journal ARchetipo come timeline e metadati di processo. Il journal non sostituisce i file nativi, che devono restare disponibili all'utente/runtime corretti.
- Scrivere gli eventi alla ricezione, indipendentemente da browser, polling o thread selezionato. Separare la finestra in RAM dalla storia durevole. Una soluzione coerente con lo storage attuale è metadati JSON atomici più eventi JSONL in append, con recupero dell'ultima riga incompleta e lettura paginata.
- Identificatori degli eventi del provider, generazione del collegamento e cursori della timeline devono consentire deduplica e ordine anche dopo un restart. Un nuovo processo non può azzerare il cursore della conversazione.
- Persistenza di invio, identificazione del turno ed eventuale riaggancio devono distinguere consegna confermata da esito incerto. Dopo un crash non reinviare automaticamente un comando con effetti: prima riconciliare con la sessione nativa; se il protocollo non offre una prova, esporre l'incertezza.

### Ciclo di vita

- La conversazione non ha una scadenza di inattività o una durata massima globale.
- Separare lo stato archivistico della conversazione, il collegamento al runtime e lo stato del turno. Un runtime disconnesso non significa che la conversazione sia conclusa; un errore di turno non rende il thread inutilizzabile.
- Un nuovo messaggio avvia il turno nella sessione collegata oppure riprende quella persistita prima di avviarlo. Apertura concorrente e invio vanno serializzati per conversazione, anche in presenza di più tab; più processi View sullo stesso workspace richiedono un lock di proprietà della sessione.
- Mantenere timeout tecnici per avvio, handshake, chiamate e shutdown. I limiti delle execution batch restano separati dalla durata della conversazione. Nel percorso interattivo un'attesa dell'utente non consuma un timeout globale che distrugge la sessione.
- «Interrompi» ferma il turno. «Archivia» organizza la conversazione. «Elimina» ha una semantica distinta e deve dichiarare se elimina anche la sessione nativa. Chiudere una scheda browser non cancella il lavoro.
- Al restart di View riconciliare sessione, turno e execution eventualmente aperta. Una execution non può restare `RUNNING` per sempre né diventare riuscita perché il processo non esiste più.
- Non introdurre inizialmente un daemon universale o uno scheduler di sospensione automatica. La sopravvivenza del lavoro attivo alla chiusura del processo View è una proprietà separata da verificare per ciascun harness; il primo requisito è poter riprendere il contesto dopo il restart.

### Modello ed effort

La selezione appartiene alla conversazione e viene applicata al prossimo turno. Ogni turno registra il modello e le opzioni effettivamente accettati. Il workspace conserva i propri default.

Se un turno sta lavorando, il selettore può predisporre il prossimo senza cambiare retroattivamente quello in corso. Un messaggio di steering appartiene al turno corrente; non deve fingere di applicare un effort nuovo. Distinguere impostazioni richieste da impostazioni applicate e non aggiornare queste ultime prima dell'accettazione del provider.

Cambiare modello dentro lo stesso harness è parte dell'obiettivo. Passare da Claude a Codex è un'altra operazione: non esiste nel contratto verificato una memoria nativa portabile fra i due. Un eventuale trasferimento futuro dovrà dichiarare la perdita di continuità e non presentarsi come resume nativo.

### Filesystem e processi

Rimuovere il divieto generale dal prompt di conversazione e usare tool, istruzioni, MCP, plugin e policy del runtime effettivo. View deve trasportare approvazioni e richieste d'input supportate senza negarle silenziosamente. Le policy di accesso continuano a essere applicate dall'harness; il testo del prompt non costituisce un sandbox.

Per «avvia il server di sviluppo» il risultato atteso è un comando eseguito nella directory corretta, una verifica di disponibilità, un URL raggiungibile nell'ambiente dichiarato e la possibilità di controllare/fermare il processo. Il ritorno della shell da solo non prova che il server funzioni. Usare i job nativi del runtime quando disponibili, verificando se sopravvivono a fine turno, interrupt, rilascio del runtime e restart di View. Non scaricare automaticamente un runtime che possiede job ancora necessari.

Con provider locali il filesystem è quello dell'host di View, secondo i permessi del processo; non necessariamente quello del browser. Con ARcipelago è quello del runner. Un server avviato su un runner remoto non diventa raggiungibile sul `localhost` dell'utente senza port forwarding o un endpoint esposto. Accesso ai file locali richiede un runner sulla stessa macchina o un mount esplicito, non soltanto un percorso nel prompt.

Se una skill crea un worktree, la directory effettiva e i percorsi di approvazione devono seguire il lavoro reale. La rimozione del worktree a integrazione conclusa richiede un rientro esplicito al workspace prima del turno successivo. La continuità del contesto non autorizza a riusare alla cieca un `cwd` scomparso.

## Adattamento dei provider

| Capacità | Codex | Claude Code | ARcipelago |
| --- | --- | --- | --- |
| Persistenza/ripresa | Thread non effimero e `thread/resume` | Persistenza abilitata, acquisizione `session_id`, `--resume <id>` | Verificare/estendere hub e runner; il solo run ID vivo non basta |
| Turni successivi | `turn/start` sul medesimo thread, `turn/steer` solo durante un turno | Messaggi stream-json nella sessione; resume se il processo va riaperto | Serve un contratto per avvio turno distinto dal messaggio a un run attivo |
| Modello/effort | Override su `turn/start` | Comandi di controllo supportati dalla versione, oppure riapertura con `--resume` e flag | Capability del runner effettivo da esporre attraverso l'hub |
| Skill | `skills/list`, invocazione con input skill | Catalogo della sessione/handshake disponibile nella versione, invocazione nativa `/nome` | Catalogo prodotto sul runner e associato all'harness effettivo |
| Tool/permessi | Riutilizzare app-server, implementare richieste server e approvazioni | Riutilizzare stream-json e ponte `can_use_tool` esistente | Riutilizzare i comandi remoti e dichiarare il luogo di esecuzione |

Verifica locale senza inferenza: `codex-cli 0.146.0` genera schemi che includono `ThreadResumeParams`, `TurnStartParams.model`, `TurnStartParams.effort` e `SkillsListParams`. `claude 2.1.263 --help` espone resume, session ID, modello ed effort. Queste verifiche non provano ancora il funzionamento end-to-end.

La documentazione Codex conferma thread persistenti, override per turno e discovery/invocazione delle skill. [Codex App Server](https://learn.chatgpt.com/docs/app-server).

Claude documenta resume per ID, skill in print mode e comandi `/model` e `/effort` con argomento. Va provata la loro semantica sul canale stream-json usato qui, inclusa la conferma del cambio; il fallback con resume della stessa sessione evita di dipendere dal cambio a caldo. [Claude Code programmatico](https://code.claude.com/docs/en/headless), [CLI reference](https://code.claude.com/docs/en/cli-reference).

### Contratto comune minimo

Estendere il confine esistente con un contratto opzionale per sessioni durevoli, senza imporre nuove responsabilità ai connector o alterare tutte le implementazioni di `Provider`.

Operazioni necessarie: creare/riprendere una sessione, inviare un turno con opzioni e skill, leggere eventi/stato, interrompere un turno, rilasciare il runtime e interrogare le capacità disponibili. Riutilizzare `RunCollaborator`, eventi e approvazioni dove la semantica coincide; non chiamare `CancelRun` per significare indistintamente stop, archiviazione e disconnessione.

Le capability devono descrivere quanto il provider può fare nell'ambiente effettivo: resume, selezione del modello per turno, skill, steering, approvazioni e posizione di esecuzione. La UI disegna il contratto; non decide con `if provider == ...`. Nessuna emulazione nascosta con transcript quando manca il resume.

## Skill disponibili e processo ARchetipo

La lista deve provenire dall'harness che esegue quella conversazione, nel suo workspace e con le sue impostazioni. Una skill installata soltanto in `.claude/skills` non compare in una conversazione Codex, salvo che sia stata resa disponibile anche a Codex. Una skill condivisa o installata per entrambi può comparire per entrambi. Non basta leggere il nome nel frontmatter per attribuirla a un provider.

Distinguere il suggerimento del modello dal menu deterministico del composer: View può proporre le skill attraverso il catalogo anche se il modello non le suggerisce spontaneamente. Rispettare disabilitazioni, origine, namespace dei plugin, invocabilità esplicita e directory. Un file esistente non prova che il runtime lo abbia caricato. Claude documenta discovery tramite le impostazioni del runtime. [Skill nel Claude Agent SDK](https://code.claude.com/docs/en/agent-sdk/skills).

Codex espone un'API dedicata; per Claude va verificata la fonte programmatica del catalogo nella versione supportata, compresi i comandi/skill caricati in inizializzazione. Evitare una scansione comune e indiscriminata di tutte le directory degli harness, che mostrerebbe skill non utilizzabili o ignorerebbe plugin e precedenze.

Il ciclo per una skill ordinaria è: selezione nel composer, verifica della disponibilità, invocazione nativa nel prossimo turno, strumenti e risultato nella stessa timeline.

Per una skill di processo occorre anche:

1. Risolvere dal Template l'azione e la spec, usando lo stato corrente per il preflight.
2. Creare un record execution associato alla conversazione, con identità distinta; una conversazione può accumularne diversi.
3. Inviare il prompt di azione alla sessione esistente, riutilizzando le regole delle skill e la costruzione delle ricevute. Non chiamare un secondo `Provider.Execute` che apre un altro agente.
4. Lasciare che l'azione attraversi più turni se serve una risposta umana. Finire un turno non significa finire l'azione.
5. Correlare la ricevuta all'azione corrente e verificare gli effetti reali attraverso lo strato che già possiede il connector. Testo casuale o una vecchia ricevuta non possono chiudere l'execution.
6. Chiudere solo l'execution. Il turno successivo continua nella stessa sessione e nello stesso thread.

I pulsanti di processo, le proposte confermate e l'invocazione esplicita della corrispondente skill devono convergere sullo stesso percorso. Per richieste in linguaggio naturale o attivazione autonoma di una skill di processo, prima dell'effetto serve una registrazione strutturata dell'azione: usare un hook di invocazione supportato oppure un bridge verso il servizio ARchetipo. La modalità concreta va scelta nello spike; una sola istruzione nel prompt non garantisce che ogni invocazione passi dal tracking. Non ricostruire retroattivamente il successo da una frase dell'agente.

Le skill restano responsabili dei propri comandi CLI e payload. I provider non imparano le regole di transizione della board. La disponibilità di una skill generica e l'ammissibilità di un'azione di processo sono due verifiche distinte.

## Piano incrementale e verifiche

### 0. Prove dei protocolli, prima del refactoring esteso

Su fixture isolate, con le versioni effettivamente supportate, verificare due turni nella stessa sessione, restart del runtime, resume dopo restart di View, cambi di modello/effort e discovery/invocazione delle skill. Per Claude identificare anche il messaggio che conferma modello e catalogo; per Codex provare il ponte di approvazioni e input. Per entrambi avviare un piccolo server, provarne la raggiungibilità e fermarlo.

Verificare con ARcipelago proprietà della sessione, persistenza del runner, retention della storia, modello/effort per turno, catalogo delle skill e recupero di un task già creato ma non ancora assegnato. Per questo documento è stato letto solo l'adapter ARchetipo: non è stata verificata l'implementazione dell'hub.

Uscita: matrice di capacità provate e versione minima; decisione documentata sul collegamento delle skill di processo al tracking. Nessuna promessa di parità basata soltanto sulle API dei vendor.

### 1. Identità durevole e journal indipendente dalla UI

Estendere record/store; migrare i file esistenti; collegare persistenza al flusso di eventi; conservare tutta la storia su disco con lettura paginata; introdurre riferimenti nativi e recupero dei turni interrotti. Correggere la relazione fra conversazione, runtime ed execution.

Verifica: più di 2.000 eventi con browser chiuso, restart, storia completa senza duplicati; invio concorrente da due tab; crash prima/dopo la consegna; configurazione default modificata senza cambiare provider a una conversazione esistente.

### 2. Sessioni locali persistenti e operative

Claude: abilitare la persistenza, catturare l'ID nativo, riprendere per ID, eliminare il timeout globale delle conversazioni, conservare il ponte di approvazioni. Codex: implementare conversazioni libere, thread persistenti, resume, più turni, stop del solo turno e approvazioni/input. Rivedere il prompt e consentire gli strumenti secondo le policy configurate.

Verifica: stessa conversazione e stesso ID nativo dopo riavvio; contesto ripreso senza estratto costruito da ARchetipo; idle oltre il vecchio timeout; un turno interrotto seguito da un nuovo messaggio; modifica di un file e ciclo avvio/controllo/stop del server. Sessione nativa mancante produce un errore recuperabile e nessun reinvio nascosto.

### 3. Composer e catalogo delle skill

Abilitare modello/effort per il prossimo turno, mostrare stato effettivo e ambiente di esecuzione, rendere disponibile il catalogo skill della sessione. Riutilizzare i componenti del selettore e i renderer esistenti; la logica resta nel backend/provider.

Verifica: cambio a idle, scelta durante un turno attivo, modello senza effort, rifiuto del provider senza falsificare la selezione applicata; skill solo Claude assente in Codex, skill condivisa presente in entrambi, skill disabilitata non invocabile dal selettore. Browser smoke reale con click su selettore e invocazione, non soltanto test HTTP o test sul testo di `app.js`.

### 4. Azioni di processo nella stessa sessione

Separare nel servizio il lifecycle dell'execution dal possesso del processo dell'agente; riutilizzare preflight, record, ricevute e verifica degli effetti con una sessione esistente. Collegare pulsanti e invocazioni native al percorso unico. Definire e verificare il passaggio di directory quando le skill creano/rimuovono worktree.

Verifica: conversazione → `/archetipo-plan` → domanda umana → piano persistito → cambio modello → `/archetipo-implement` → ulteriore messaggio. Stesso thread e sessione, record execution distinti, nessun secondo agente, esiti verificati, nessuna doppia invocazione su retry. Coprire anche skill di processo invocata dal modello e crash con execution ancora aperta.

### 5. ARcipelago e compatibilità complessiva

Implementare solo le capability confermate nello spike; estendere hub/runner dove necessario. Persistenza della mappa locale non basta se il runner elimina il contesto: l'ID deve continuare a riferirsi a una sessione recuperabile. Riprendere il collegamento remoto non deve creare un nuovo task per errore.

Verifica: runner offline e ritorno, restart dell'hub/View, cambio modello, catalogo proveniente dal runner, filesystem effettivo e URL del server. Finché una capacità manca, View deve dichiararla indisponibile senza simulare continuità.

### Compatibilità e criteri di consegna

Le vecchie conversazioni hanno perso deliberatamente la persistenza nativa: non è possibile trasformarle retroattivamente in sessioni complete. Restano consultabili. Un comando esplicito può creare una sessione a partire dal testo disponibile, dichiarando che è una ricostruzione parziale; non è il percorso ordinario di ripresa delle nuove conversazioni.

Conservare le 14 operazioni CLI pubbliche e i contratti batch; introdurre cambi interni/opzionali e rotte HTTP additive dove possibile. Pianificare la transizione del vecchio endpoint `/resume`, del significato di chiusura e dei record con `execution_id == conversation_id`, mantenendo leggibili i dati esistenti.

Per l'implementazione eseguire i check richiesti da `AGENTS.md`: gofmt, vet, build, test, golangci-lint e conformance dei connector. Aggiungere test dei protocolli con fake controllati, test web e smoke browser; i fake non sostituiscono le prove dei runtime reali della fase 0. Non occorre aspettare giorni nei test: simulare il trascorrere del tempo e riavviare davvero i processi.

## Confini della prima consegna

L'obiettivo include sessioni riprendibili, turni con modello/effort modificabili, tool reali, skill corrette per harness e azioni ARchetipo nello stesso thread. Non richiede una memoria cross-provider, una replica dei terminali interattivi dei vendor, un nuovo agent loop o una piattaforma proprietaria per gestire tutti i processi di sviluppo.

La parte più impegnativa è separare correttamente sessione ed execution conservando il tracking del processo; la parte meno determinata è ARcipelago. Avviare lo spike e il lavoro sui provider locali consente di progredire mentre si definisce il contratto remoto, senza ridurre tutte le conversazioni alle capacità dell'adapter attuale.

Verifica di questa analisi: lettura del codice, documentazione ufficiale, versioni CLI e schemi generati da Codex. Non sono state avviate sessioni AI né eseguite prove live dei comportamenti proposti. Nessun codice applicativo o configurazione è stato modificato.
