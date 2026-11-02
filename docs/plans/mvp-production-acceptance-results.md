# Collaudo MVP: risultati del 29 ottobre 2026

## Giudizio

Il percorso chat → implementazione → app provabile → feedback → candidato
verificato è stato eseguito con Codex reale su due stack applicativi. Ha prodotto
funzionalità utilizzabili e una seconda revisione in entrambi i progetti.
Il primo passaggio non era sufficientemente fluido: sono emersi dieci gruppi di
difetti o ambiguità di ARchetipo, corretti e coperti da regressioni.

La versione locale corretta supera i controlli eseguiti. È un candidato per un
pilot controllato su macOS con Codex e backlog file; questo collaudo non basta a
dichiarare disponibilità generale su tutte le piattaforme e i connector.
RideAtlas dimostra anche che il prodotto rifiuta correttamente la consegna
quando i controlli dell'applicazione falliscono.

Il [piano](mvp-production-acceptance.md) descrive gli oracoli. Le prove locali
restano sotto `test/workspaces/mvp-acceptance/2026-10-29/`, ignorato da Git;
specifiche, osservazioni, report originali e conversazioni restano nei progetti.

## Ambiente e metodo

- Branch `codex/chat-first-workspace`, base ARchetipo `f6605d6` più le modifiche
  locali di questo collaudo; installazione globale tramite `npm run install:dev`.
  La versione dev `0.0.0-dev.gf6605d6.dirty` non identifica da sola ciascuna
  reinstallazione: sorgenti e regressioni di questa modifica sono il riferimento.
- Installazioni ARchetipo precedenti rimosse prima dell'init, incluse conversazioni,
  helper e skill gestite. Sorgenti applicativi e dati originali conservati.
- Entrambi inizializzati con `--tool codex --connector file`; provider nativo
  `/opt/homebrew/bin/codex`, modello osservato `gpt-6.1-sol`, permessi del provider.
- Richieste e feedback inseriti nel viewer; funzionamento provato nel browser.
  API, filesystem e log usati come oracoli e per diagnosticare i difetti.
- Il Codex eseguito dal prodotto non aveva browser abilitato. La verifica UI è
  stata fatta separatamente dal collaudatore e registrata come osservazione della
  persona, senza attribuire all'agente prove che non aveva eseguito.
- Nessuna PR reale pubblicata: manca l'approvazione esplicita del commit mostrato.
  Nessun merge o deploy. Le applicazioni usano dati locali di prova.

## Percorsi reali

| Scenario | Esito | Evidenza |
| --- | --- | --- |
| M01: apri l'app Digital Twin dalla home | PASS dopo correzioni | Python/Streamlit scoperto, ricetta qualificata, installazione pulita e riuso offline; sessione tecnica distinta dal lavoro prodotto |
| M02: esporta l'ultima simulazione | PASS | Prima dell'analisi spiegazione senza download; dopo 30 simulazioni CSV reale di 24 righe, etichette italiane, parametri e valori coerenti con UI |
| M03: feedback sul nome CSV | PASS | `risultati_azienda-demo.csv`; stessi dati dopo riavvio e seconda revisione verificata nella medesima sessione |
| M04: checklist bici pubblica | PASS funzionale | Pagina `/checklist-bici`, cinque controlli iniziali, nessun login, spunte e progresso persistenti |
| M05: aggiungi pressione pneumatici | PASS funzionale | Selezioni Casco/Acqua conservate: 2/6; completamento 6/6 e messaggio che non certifica la sicurezza; viewport 390 px senza overflow |
| M06: stop, reload, ripresa, dossier | PASS dopo correzioni | Porte chiuse dopo stop; identità native conservate; dossier esportati; morte del gruppo proprietario provata su Next e Streamlit reali |
| M07: errori e recupero | PASS per i casi eseguiti | Ricette con glob respinte, ripresa dopo permessi/interruzioni, prove riutilizzabili tra agente e viewer; check falliti non producono un falso ready |
| M08: consegna e regressioni | PASS locale; PR reale NOT_RUN | Candidato Digital Twin in REVIEW; RideAtlas in DEVELOPING; flusso PR/commit approvato/idempotenza coperto dallo smoke con hosting simulato |

### Digital Twin

Incremento `inc-a3ea5d1ac042b5e5`, conversazione
`conv-ab5811335c75021c2e1dc115196876bd`, sessione nativa
`01a18789-c74a-7b60-a7fb-648580b95faa`.

La richiesta originale era portare in riunione un CSV leggibile dell'ultima
simulazione, senza AI o servizi esterni e senza un file vuoto prima dell'analisi.
Il feedback ha richiesto un nome riconoscibile dall'azienda, chiarito anche con
un secondo messaggio durante il turno attivo. Entrambi i messaggi restano nella
stessa conversazione.

Revisione 1: 97 test superati. Revisione 2: **98 test superati**, installazione
pulita verificata, stato **REVIEW**, commit
`9c7f3dab389bc4b630834833592b92af2c0aa5a5`.
Report originali della revisione finale: `verification-1323455743` e
`verification-4089830387`. Approvazione e PR assenti.

Il CSV finale conserva le stesse righe e gli stessi valori del primo download:
attesa media 6,734 h e pratiche concluse 21,733, coerenti con l'arrotondamento a
schermo. I byte differiscono perché il caricamento dello snapshot persistito
riordina i reparti; nessun risultato è stato ricalcolato per questa verifica.
`csv-final-check.json` documenta separatamente uguaglianza dei dati e dei byte.

La prova di morte dell'intero gruppo proprietario ha lasciato la porta chiusa,
lo stato `stopped`, preparazione riusata e dati identici: hash del payload azienda
prima/dopo `5816df6fa5669a07961c904bc215f75aa540530a6e898faa6ba5c5de1772e24f`.
Evidenze: `streamlit-process-check.json`, CSV iniziale/finale, screenshot e dossier.

### RideAtlas

Incremento `inc-0624cc8a36087dba`, conversazione
`conv-fb70d97790b0fbde6db5d77aeda1c66d`, sessione nativa
`01a1877f-bfd4-7642-85e2-36c80ef5ebef`.

La checklist è stata provata e rivista realmente. I controlli dell'incremento
sono stati eseguiti tutti a preview ferma, senza rimuovere quelli falliti:

| Controllo | Esito |
| --- | --- |
| Checklist | PASS, 8/8 |
| Lint | PASS, sette warning |
| Build Next | PASS, pagina prerenderizzata |
| TypeScript | FAIL, 986 errori in test esistenti |
| Jest completo | FAIL, 1916 passati, cinque falliti, due saltati; tre suite admin-users fallite |

I cinque fallimenti confrontano il messaggio atteso «Ruolo utente aggiornato con
successo» con «Dati utente aggiornati con successo». AppShell e RideChecklist
passano. I report conservati sono `verification-4083302452`,
`verification-761709865`, `verification-2176716716`, `verification-276858969` e
`verification-1053325295`. Il gate restituisce `E_PRECONDITION` per `tests`.

Il candidato resta **DEVELOPING**, senza ready SHA, approvazione o PR. Questi
difetti applicativi preesistenti non sono stati nascosti per ottenere un esito
verde e restano da risolvere prima della consegna di RideAtlas.

## Correzioni ARchetipo e prove di regressione

| Problema osservato | Correzione | Verifica |
| --- | --- | --- |
| Configurazione ancora «in attesa» dopo risposta/permesso | Persistita la ripresa in `configuring`, rimosso errore superato | Test web dell'ambiente |
| Prove valide nell'agente ma stale nel viewer | Fingerprint sugli eseguibili risolti; percorsi relativi dal progetto; PATH esplicito se dichiarato | Test unitari e Service con cwd/PATH differenti; ricette reali riutilizzate |
| Glob trattati come nomi di file inesistenti | Validazione rifiuta glob e spiega file/directory espliciti | Test recipe e riparazione reale della ricetta |
| Risposte e tabelle spezzate dai metadati streaming | Ricomposta la risposta dello stesso messaggio conservando i metadati | Test del renderer puro |
| Prompt interno mostrato come messaggio dell'utente durante steering | Trasporto separato di istruzione e testo visualizzato in Codex/Claude | Test adapter/server e nuovo feedback reale durante il turno |
| Stop annullato automaticamente al turno successivo | Scelta conservata per checkout nella sessione browser fino a Start esplicito | Smoke browser conta zero POST di riavvio dopo nuovo turno e reload |
| Preview CLI viva mostrata come ferma | Stato esplicito del proprietario esterno, link apribile e stop disponibile | Test journal e smoke con secondo viewer |
| Agente View avvia un secondo server o trattiene il turno | Istruzioni View affidano avvio al viewer; skill standalone rimane autonoma | Seconda revisione reale conclusa senza server concorrente |
| Morte del gruppo harness uccide anche la pulizia | Supervisori POSIX separati con controllo via pipe e cleanup verificato | Benchmark, smoke con dipendenze, Next reale in cinque modalità e Streamlit reale |
| Viewer esaurisce i descrittori osservando virtualenv e runtime | Esclusi alberi generati sia alla scansione sia alla creazione, anche con watch root `.archetipo` | Test iniziali/dinamici/root, nuovi viewer e controllo HTTP finale |

Il blocco dei descrittori era concreto: circa 245 mila record `lsof`, per lo più
virtualenv, e letture delle conversazioni fallite con EMFILE. Dopo la correzione,
100 richieste HTTP sequenziali a conversazione/preview in entrambi i workspace
hanno prodotto zero errori in 24,745 s; Digital Twin aveva 44 record nominati,
RideAtlas 28, nessuno nella virtualenv. È un controllo di stabilità HTTP breve,
non un soak test prolungato o una misura della velocità dell'LLM.

Un vecchio processo Streamlit rimasto orfano prima della correzione è stato
rimosso dal collaudatore solo dopo verifica di identità, comando e gruppo del
processo di test. Questo intervento è attrito, non una guarigione automatica del
prodotto. Non è stato aggiunto alcun kill generico di listener sconosciuti.

## Verifiche della branch

- `gofmt -l .`: nessun output; `go vet ./...`, `go build ./...`, `go test ./...`:
  superati; `golangci-lint run --timeout 5m ./...`: zero issue.
- `npm run test:web`: **412 test superati**.
- Smoke pagina, chat, incremento e preview: superati nel browser reale locale.
  Includono stop persistente, ripresa, owner esterno, storie e PR simulata.
- `npm run test:harness-benchmark`: superato, incluse invalidazione, fallimenti,
  concorrenza, timeout e morte dell'intero gruppo proprietario.
- Smoke processi con **Next 15.5.28 reale**: cinque modalità superate, inclusa
  `owner-group-sigkill`. Prova Streamlit reale: superata e dati conservati.
- Race detector mirato su watcher, steering e configurazione nativa: superato.
- `git diff --check`: superato.

Report riproducibili dei principali run:

- `test/workspaces/preview-view-smoke/2026-10-29T12-38-34-533Z/`
- `test/workspaces/enterprise-product-view-smoke/2026-10-29T12-45-26-041Z/`
- `test/workspaces/next-preview-process-smoke/2026-10-29T12-31-02-213Z/`
- `test/workspaces/harness-benchmark/runs/2026-10-29T12-31-39-658Z/report.html`

La CI ora richiede frontend, quattro smoke browser e benchmark ambiente su
Ubuntu, con Chrome esplicito e conservazione dei report. La nuova pipeline
remota **non è ancora stata eseguita**: gli esiti sopra sono locali.

## Attrito residuo e gate per il rilascio

Il ciclo reale ha richiesto diagnosi, reinstallazioni, risposte ai permessi e
riavvii manuali. I tempi dei turni mescolano modello, attese, verifiche e debug:
non sono una misura pulita del tempo alla prima app. Non è stato ancora misurato
il completamento autonomo da parte di utenti nuovi senza assistenza tecnica.

Prima di dichiarare disponibilità generale:

1. Eseguire la nuova CI da un commit identificabile e un pacchetto immutabile.
2. Ripetere il percorso su macchine pulite Linux/Windows. La separazione dei
   supervisori aggiunta è POSIX; Windows conserva il contratto taskkill esistente
   e non è stato validato dal collaudo reale di questa sessione.
3. Provare con persone esterne un primo lavoro e una revisione senza aiuto,
   registrando completamento, abbandono, richieste tecniche e comprensione del
   candidato. Questo valuta la fluidità; questi incrementi non validano il mercato.
4. Fare un soak prolungato con più worktree e turni. Il breve controllo finale
   non dimostra stabilità su giorni di utilizzo.
5. Se il rilascio comprende GitHub/Jira e provider diversi da Codex, eseguire i
   relativi percorsi reali. Qui hosting della PR era simulato e backlog era file.
6. Per consegnare RideAtlas risolvere la baseline rossa; per pubblicare il Digital
   Twin approvare esplicitamente il commit verificato mostrato nel viewer.

I due viewer rimangono consultabili localmente: Digital Twin sulla porta 43821,
RideAtlas sulla 43822. Conversazioni e dossier consentono di leggere cosa è stato
chiesto, osservato e deciso, compresi i fallimenti e le limitazioni.
