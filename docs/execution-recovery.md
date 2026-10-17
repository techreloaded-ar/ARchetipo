# Esecuzioni riprendibili

Le skill di base (`inception`, `design`, `spec`, `plan`, `implement`, `review`)
funzionano direttamente nel coding agent e in View. Non conoscono le skill che
le compongono. Autopilot compone `plan`, `implement` e `review`; Increment compone
`spec` e Autopilot. Autopilot non riconosce un incremento e non riceve policy
specifiche. Ambiente locale e contratto dei subagenti sono istruzioni comuni
in `.archetipo/shared-runtime.md`.

ARchetipo conserva i fatti dell'esecuzione; il coding agent gestisce i propri
subagenti. Non viene introdotto un motore di worker.

Per una singola modifica circoscritta, su un componente esistente e con una
ricetta di verifica già disponibile, Autopilot può eseguire Plan e Implement
nello stesso contesto. Invoca comunque i contratti delle due skill e verifica
gli stessi effetti CLI. L'accettazione resta sempre in una sessione indipendente.
La scelta viene conservata nel checkpoint; nuove dipendenze, persistenza,
autenticazione, concorrenza o complessità emersa richiedono fasi isolate.
Increment richiama Autopilot nel proprio contesto, mantenendo due checkpoint
distinti, e continua fino alla consegna verificata prima di chiudere il turno.

## Checkpoint CLI

I controller usano `archetipo execution workflow` per conservare stato e contatori
in `.archetipo/workflow-runs/<id>.json`. I record sono atomici, protetti dai lock
del journal ed esclusi dai diff e commit automatici.

```sh
archetipo execution workflow start --file - <<'EOF'
{"skill":"archetipo-autopilot","scope":"US-001","invocation":"/archetipo-autopilot US-001","state":{"selected_specs":["US-001"],"specs":{"US-001":{"outcome":"pending","review_iterations":0,"phase_recoveries":{}}}}}
EOF
archetipo execution workflow list
archetipo execution workflow show <id>
```

`start` restituisce il record incompleto già presente per quella skill, scope e
conversazione. Il campo `invocation` conserva il comando effettivo separato dallo
scope normalizzato (senza selettori, Autopilot non riceve l’argomento `ALL`).
Uno scope differente non può sostituirlo. In View il prompt fornisce
`Workflow conversation: <id>`: il controller lo conserva in `conversation_id`.
Standalone il campo può essere omesso e il protocollo resta identico.

`checkpoint <id> --file -` accetta `revision`, `status`, `phase`, `worker_id`,
`state` e `last_error`. `state` è un oggetto JSON interpretato soltanto dalla skill;
ogni aggiornamento lo passa integralmente. `revision` impedisce sovrascritture
da un writer obsoleto (`E_CONFLICT`). Gli stati scrivibili sono `running`, `blocked`,
`complete`; `recovering` viene riservato dal recupero del trasporto. I record
completi restano come cronologia immutabile.

Le risposte hanno envelope `archetipo/v1`, kind `workflow_run`, con il record in
`data` (un array per `list`). Un payload errato produce `E_INVALID_INPUT`, un ID
assente `E_NOT_FOUND`, uno stato non leggibile `E_INTERNAL`.

Il controller importa in sola lettura gli eventuali vecchi file
`autopilot-state-*.yaml`, conservando selezione e contatori. Dopo l'importazione
il checkpoint CLI è l'unico stato aggiornato; i file precedenti restano intatti.

## Recupero della connessione

La perdita del processo o dello stream Codex marca la connessione disconnessa e
il turno attivo fallito, conserva la sessione nativa e invalida le decisioni
pendenti della connessione. La diagnostica comprende errori di lettura dello
stream, codice di uscita ed estratto limitato di stderr. Il catalogo non riusa
il client terminato. Gli agganci alla stessa sessione vengono serializzati.

View può continuare un'esecuzione incompleta associata alla conversazione dopo
un errore di trasporto. Riserva al massimo tre recuperi per run, riaggancia la
stessa sessione nativa e avvia un nuovo turno di riconciliazione. Non reinvia il
comando precedente e non presume completati i task: il controller verifica
worker precedenti, file, task ed evidenze. Viene ripreso il controller più esterno,
senza interpretare i nomi delle skill.

La riconnessione viene tentata al massimo tre volte per recupero. Consegne incerte,
stop della persona e run bloccati o completi non vengono ripetuti automaticamente.
Il limite esaurito resta visibile come intervento necessario. La supervisione
opera mentre View è attivo; il checkpoint consente la ripresa standalone e dopo
un riavvio senza ricreare il lavoro da zero.

## Qualità e approvazioni

I recuperi non consumano né azzerano il budget di rework. Il controller registra
l'accettazione pendente prima del worker, così una transizione a TODO seguita
da un'interruzione non perde il conteggio.

Le verifiche registrano comando, esito, versione/diff verificato e report. Dopo
modifiche successive si ripetono i controlli interessati. Build e typecheck sono
distinti; le nuove diagnostiche si confrontano con una baseline esplicita senza
usare conteggi di righe o dichiarare verde un comando fallito.

Review mantiene il verdetto umano come default. L'accettazione tecnica autonoma
richiede `Acceptance mode: autonomous`, iterazione e autorizzazione al
request-changes. Una storia lasciata in review impedisce `increment ready`; un
nuovo run non può azzerare un budget esaurito. L'approvazione dell'intento
sopravvive alle riprese; la PR richiede l'approvazione del risultato verificato.

I test Go coprono caduta, riaggancio, assenza di replay, budget, checkpoint obsoleti,
stop e consegne incerte. `npm run test:web` verifica presentazione e confini fra
skill; lo smoke della pagina esegue i renderer nel browser. La valutazione
comportamentale delle skill resta distinta dai test deterministici del runtime.

## Ambiente e prove riproducibili

Plan indica il comando di setup dei test e i percorsi delle prove attese.
Implement prepara soltanto risorse fornite esplicitamente o riconoscibili come
proprie della directory e dello scope; conserva il setup idempotente con i test
del progetto. Nomi simili e porte locali non dimostrano proprietà. Un worker
successivo usa quella ricetta prima di cercare un altro ambiente; credenziali
ignote richiedono una nuova risorsa isolata, senza cambiare quelle di un servizio
preesistente. Queste regole sono comuni e non dipendono da un controller.

`archetipo e2e run --spec US-001` conserva output completo e `report.json` in una
nuova sottocartella di `paths.test_results/US-001/`, anche quando i test falliscono.
L'envelope resta `kind: e2e_run`: `data.passed` indica l'esito, `report_path` e
`output_path` indicano i file effettivi. `ok: true` non significa test verdi.
Il report registra comando, tempi, exit code e sorgente Git all'avvio. L'hash del
diff riguarda i file tracciati: il revisore deve controllare anche codice non
tracciato e modifiche successive. Il comando senza `--spec` mantiene il contratto
precedente e non crea questi file.

L'implementazione legge i report prima di consegnare la spec alla review. Output
solo in console o in `/tmp` non conclude il gate; disabilitare il video non
disabilita le prove funzionali. Review verifica i risultati conservati e la loro
pertinenza al codice corrente. La CLI persiste i fatti; le skill valutano la
copertura e la correttezza.
