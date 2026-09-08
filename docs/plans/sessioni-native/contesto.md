# Direzione di prodotto e istruzioni comuni

Questo documento accompagna i dodici prompt di implementazione. Leggerlo a ogni nuova sessione. Stato: piano proposto; nessun task risulta eseguito per il solo fatto che questi file esistano.

## Direzione confermata

L'utente vuole che le conversazioni di ARchetipo View siano sessioni dei veri harness di sviluppo: contesto nativo persistente, nessuna scadenza della conversazione, modello ed effort modificabili, tool reali sul filesystem dell'ambiente di esecuzione e skill effettivamente disponibili al provider. Anche le skill di processo devono lavorare nello stesso thread da cui sono invocate.

È un cambio di strada del prodotto. Le precedenti scelte documentate nel codice, nei test, nelle spec o nella Wiki possono e devono essere sostituite quando contraddicono questa direzione. Non chiedere conferma soltanto perché trovi la vecchia regola. Conserva le istruzioni operative del repository e distingui una protezione dei dati da una restrizione di prodotto superata.

## Scelte proposte da questo piano

- Una conversazione può lavorare senza spec, senza execution e senza un passaggio obbligatorio da proposta/conferma. L'harness conserva le sue policy e i suoi meccanismi di approvazione.
- Una skill nativa si invoca nativamente nella sessione. La disponibilità viene dal runtime effettivo, non da una scansione cumulativa delle directory di tutti i provider.
- Un'azione lanciata dai controlli di processo ARchetipo mantiene tracking e verifica degli effetti, ma usa la sessione esistente. Il successo dell'azione non chiude la conversazione.
- Non imporre che ogni skill scelta autonomamente dal modello produca un record execution. Non introdurre un bridge o un'intercettazione universale soltanto per conservare quel vincolo storico. La board rilegge lo stato reale anche dopo modifiche prodotte da skill native.
- Conversazione, collegamento al runtime, turno ed execution hanno identità e lifecycle distinti. Una conversazione può contenere molte execution; una execution può attraversare più turni.
- Modello/effort scelti durante un turno si applicano al prossimo. Lo steering, dove supportato, resta nel turno corrente. Il provider e l'ambiente della conversazione non cambiano perché cambia il default del workspace.
- L'harness possiede contesto e compaction. ARchetipo conserva riferimenti nativi e timeline; nessun riassunto automatico mascherato da resume.
- Nessun timeout globale o di inattività chiude la conversazione. Conservare timeout tecnici e distinguere i limiti del batch. Chiudere una tab non interrompe il lavoro; interrompere un turno non elimina la sessione.
- Il primo rilascio garantisce ripresa del contesto dopo restart di View. La continuità dell'esecuzione attiva durante lo shutdown dell'host è una capacità distinta da provare: niente daemon universale introdotto implicitamente.
- Tool e server di sviluppo agiscono sul filesystem dell'host del runtime. Un runner remoto non ha implicitamente i file o il localhost del browser.
- Più conversazioni possono modificare il medesimo workspace, come negli harness. Non introdurre per questo un blocco globale o worktree obbligatori. Serializzare invece il possesso e i comandi della stessa sessione.
- Archiviare non cancella il contesto. Per la prima consegna, eliminare da View riguarda i dati ARchetipo: non cancellare automaticamente transcript nativi o file di lavoro. Dichiarare questa semantica nell'interfaccia/documentazione.
- I dati precedenti restano leggibili e non vengono distrutti. Quando manca una sessione nativa, dichiarare il limite; un'eventuale nuova sessione da estratto è un gesto esplicito e non il resume normale.

## Come eseguire un task

Lavora soltanto sul task richiesto e sulle dipendenze tecniche indispensabili. Leggi AGENTS.md e osserva git status prima degli edit; conserva le modifiche dell'utente. I percorsi nei prompt sono relativi alla root del repository per permettere l'uso in worktree.

Leggi il codice attuale, non soltanto l'analisi iniziale: i task precedenti possono avere spostato file e cambiato contratti. Gli esiti documentano il lavoro, ma codice e test devono confermare che la dipendenza esista davvero. Se manca una dipendenza essenziale, identifica il task mancante senza improvvisarne un secondo design.

L'analisi iniziale è docs/plans/conversazioni-sessioni-native.md. Questo documento e il task operativo prevalgono sulle sue proposte superate. Preferisci gli adapter, il journal e i componenti esistenti. Cambia o elimina le vecchie interfacce quando semplifica il risultato; non aggiungere livelli di compatibilità soltanto per conservare semantiche abbandonate.

Modifiche incompatibili alle operazioni CLI pubbliche richiedono la versione prevista da AGENTS.md. Non cambiare arbitrariamente versione, tag o release durante un singolo task: registra il cambiamento necessario per il task 12. Conservare un comando batch funzionante è utile; non deve imporre il suo lifecycle alla conversazione interattiva.

Aggiorna nel task i test, i commenti e i documenti direttamente contraddetti dalla modifica. Non indebolire le verifiche rimaste valide per far diventare verde la suite. Le pagine Wiki vanno aggiornate seguendo il workflow del repository senza attribuire automaticamente uno stato reviewed non verificato.

Consulta documentazione ufficiale aggiornata per i protocolli e confrontala con la versione installata. Prove AI reali in sandbox dedicate, con richieste brevi e cleanup mirato. Non usare un'altra conversazione dell'utente per le prove. Una credenziale mancante o un runtime non disponibile lasciano la relativa prova non eseguita, mai passata.

## Verifica e consegna comune

Per i task che cambiano codice esegui dalla directory cli:
- gofmt -l . — nessun output;
- go vet ./...;
- go build ./...;
- go test ./...;
- golangci-lint run --timeout 5m ./....

Se golangci-lint manca, segui l'installazione prescritta da AGENTS.md. Esegui inoltre i test mirati e gli smoke indicati nel task. Verifica prima che gli script esistano nel package.json corrente. I test con fake provano l'adapter; quelli live provano il protocollo effettivo; quelli browser provano l'interazione. Non sono intercambiabili.

Non eseguire ripetutamente suite già verdi senza nuove modifiche o problemi. Per task esclusivamente documentali basta verificare contenuti, collegamenti e coerenza. Un fallimento preesistente va provato e riportato: non sovrascrivere il config dell'utente per ottenere un verde apparente.

Ogni task deve produrre docs/plans/sessioni-native/esiti/NN.md con:
1. stato: completato, parziale o bloccato; revisione/branch di partenza;
2. risultato concreto e file/contratti modificati;
3. regole precedenti eliminate e decisioni adottate;
4. comandi di verifica ed esito, runtime/versioni e prove non eseguite;
5. limiti residui e istruzioni essenziali per il task successivo.

Crea la directory esiti quando serve, senza file vuoti per i task futuri. Se il task opera su un altro repository, usa il percorso di handoff indicato dal suo prompt. Non dichiarare completato un task quando uno dei suoi criteri essenziali rimane non dimostrato.
