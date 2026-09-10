# 09 — Verificare recovery, filesystem e processi di sviluppo

Dipendenze: 03–08.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 09: Verificare recovery, filesystem e processi di sviluppo.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 03–08.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Chiudere i casi operativi che distinguono una demo da una sessione di sviluppo utilizzabile per giorni.

Punti di ingresso da verificare sul codice corrente:
Lifecycle/store/provider già implementati, supporto processi, cwd/worktree, integrazione View e test/e2e.

Passi:
1. Avvia un server di fixture tramite il vero harness, verifica la risposta HTTP, completa il turno e invia un nuovo messaggio per controllarlo e fermarlo. Registra il comportamento dei job nativi su interrupt e shutdown.
2. Risolvi eventuali errori di gestione risorse senza introdurre un process manager generico: usa il runtime nativo e un ownership chiaro. Non rilasciare automaticamente un runtime che possiede un job necessario.
3. Prova giorni di inattività simulando il tempo, restart reale di View/runtime e sessione ripresa con contesto. Un crash può interrompere lavoro attivo senza eliminare la conversazione: UI e record devono rappresentarlo correttamente.
4. Inietta crash prima dell'invio, dopo l'invio e prima della conferma, durante append e durante finalizzazione execution. Riconcilia con lo stato nativo dove possibile; altrimenti mostra consegna/esito incerti senza rilanciare effetti.
5. Prova due tab e due processi View sullo stesso workspace, provider default cambiato, due conversazioni indipendenti e perdita del collegamento. Niente doppio proprietario della stessa sessione, cursori azzerati o storia persa.
6. Prova worktree creato e rimosso, cwd non più raggiungibile e sessione nativa assente. Non puntare silenziosamente a un'altra directory; mantieni recuperabili i dati e spiega l'azione necessaria.
7. Verifica che credenziali e dati di sessione sensibili non entrino nei nuovi metadati/API mentre restano visibili gli eventi di lavoro utili.

Criteri di completamento:
Matrice per Claude/Codex con risultati misurati per resume, crash e ciclo server. Nessun processo di fixture lasciato vivo, nessun messaggio duplicato nascosto, nessuna execution eternamente RUNNING senza stato di recupero. Limiti di shutdown espliciti.

Verifica:
Test di fault injection deterministici, race test sui package con concorrenza modificati, smoke HTTP/browser e prove live mirate. Attese condizionate, non sleep arbitrari.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Non promettere la sopravvivenza del server a un restart se l'harness lo termina; dimostra invece resume della sessione e possibilità di verificare/riavviare il servizio su richiesta.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/09.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
