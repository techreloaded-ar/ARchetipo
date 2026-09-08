# Piano operativo — View come client delle sessioni native

Data: 2026-09-08. Stato: piano e prompt pronti; implementazione non iniziata.

## Risultato atteso

Conversazioni che continuano nello stesso thread dopo giorni e restart, modello/effort per il prossimo turno, filesystem e tool dell'harness, skill disponibili per il provider reale. Anche le azioni di processo lavorano nella sessione già aperta.

Questa direzione sostituisce le vecchie regole di business. Nessun obbligo generale di mantenere conversazioni read-only, passare da una proposta, creare execution per qualsiasi skill o chiudere il thread al termine del lavoro. La nuova semantica dettagliata è in [contesto.md](contesto.md), da leggere in ogni sessione. [L'analisi iniziale](../conversazioni-sessioni-native.md) conserva le evidenze del punto di partenza, con un aggiornamento che rimanda a questo piano.

## Come usare i prompt

1. Rendi disponibili questa directory e l'analisi iniziale nel checkout/worktree dove eseguirai i task. Sono file locali: una nuova sessione su un altro checkout non li eredita automaticamente.
2. Avvia una sessione AI coding per ciascun task. Copia il blocco di testo del relativo file, oppure chiedi alla sessione di leggere quel file ed eseguire il prompt contenuto.
3. Procedi nell'ordine 01 → 12. È l'ordine consigliato per ridurre conflitti sui contratti comuni e permettere di provare ogni passaggio.
4. Prima del task successivo, rendi disponibili le modifiche effettive e il relativo file esiti/NN.md. Lo stesso checkout può essere riusato in sequenza; se usi worktree diversi, integra il lavoro prima di continuare. Un handoff senza il codice non soddisfa la dipendenza.
5. Se un task resta parziale, risolvi il suo criterio essenziale prima dei task che ne dipendono. Le capacità remote non verificate non impediscono una consegna locale, ma l'obiettivo sui tre provider resta incompleto.

Ogni prompt contiene passi concreti, file da cui partire, criteri di accettazione e verifiche. Non è una richiesta di generare nuovamente un piano. Dimensione indicativa: un task è una unità di consegna, non la promessa che basti un singolo turno AI.

Esempio di messaggio minimo per iniziare, dalla root di ARchetipo:

```text
Leggi docs/plans/sessioni-native/01-prove-protocolli.md ed esegui il prompt contenuto. Leggi anche il contesto condiviso indicato nel file. Svolgi soltanto questo task e lascia l'esito richiesto.
```

## Task e ordine

| Task | Consegna | Dipende da | Prompt |
| --- | --- | --- | --- |
| 01 | Provare i protocolli nativi | Nessuna. | [Apri 01](01-prove-protocolli.md) |
| 02 | Definire e implementare il contratto delle sessioni | 01, per le capacità locali che il contratto dichiara. | [Apri 02](02-contratto-sessioni.md) |
| 03 | Rendere durevoli conversazioni e timeline | 02. | [Apri 03](03-persistenza-lifecycle.md) |
| 04 | Completare le sessioni native Claude | 01, 02, 03. | [Apri 04](04-claude.md) |
| 05 | Completare le sessioni native Codex | 01, 02, 03; in sequenza consigliata dopo 04 per recepire gli aggiustamenti comuni. | [Apri 05](05-codex.md) |
| 06 | Aggiornare View per sessioni e scelte per turno | 03, 04, 05. | [Apri 06](06-composer.md) |
| 07 | Scoprire e invocare le skill dell'harness | 01, 04, 05, 06. | [Apri 07](07-skill-native.md) |
| 08 | Eseguire le azioni ARchetipo nello stesso thread | 03, 04, 05, 06, 07. | [Apri 08](08-azioni-nella-sessione.md) |
| 09 | Verificare recovery, filesystem e processi di sviluppo | 03–08. | [Apri 09](09-recovery-processi.md) |
| 10 | Evolvere il contratto ARcipelago e il runner | 01 e 02 per il contratto richiesto; 09 per allinearsi al comportamento locale verificato. | [Apri 10](10-arcipelago-hub-runner.md) |
| 11 | Collegare View alle sessioni ARcipelago | 02, 03, 06, 07, 08, 09 e implementazione/contratto effettivi del task 10. | [Apri 11](11-arcipelago-adapter.md) |
| 12 | Accettazione finale, rimozione del vecchio modello e documentazione | 01–11; per una consegna solo locale, dichiarare esplicitamente 10/11 incompleti. | [Apri 12](12-accettazione-pulizia.md) |

## Milestone

### A — Protocolli e fondamenta: 01–03

Prima si provano le capacità native, poi si implementa il contratto minimo e lo storage/lifecycle. Uscita osservabile: il fake persistente attraversa più turni e un restart mantenendo ID e storia, senza dipendere dal polling della UI. Non è ancora una sessione di sviluppo completa.

### B — Sessioni locali utilizzabili: 04–07

Claude e Codex passano al nuovo contratto; il composer modifica il prossimo turno e propone le skill corrette. Uscita osservabile: conversazione libera, tool che modificano un file, cambio modello, resume nativo e invocazione di una skill nello stesso thread.

### C — Nuovo prodotto locale: 08–09

Le azioni gestite lavorano nella stessa sessione, con record separati; vengono verificati server di sviluppo, worktree e recovery. Questa è la prima milestone completa sui provider locali. Non dipende da un hub remoto.

### D — Estensione remota: 10–11

Il task 10 verifica e modifica l'hub/runner, il task 11 integra il contratto effettivo in ARchetipo. Se manca il repository ARcipelago, il task 10 produce il gap report e identifica l'accesso mancante: non finge di avere realizzato il supporto. Un'alternativa consentita è consegnare prima il prodotto locale dichiarando queste due attività ancora aperte.

### E — Accettazione e rimozione del vecchio modello: 12

Prove del percorso intero, migrazione, pulizia e documentazione coerente. La documentazione direttamente contraddetta viene aggiornata già nei singoli task; questa fase verifica che non siano rimasti percorsi e regole obsolete.

## Decisioni da non lasciare ambigue tra sessioni

| Tema | Scelta del piano |
| --- | --- |
| Spec obbligatoria | No: la conversazione può lavorare liberamente |
| Execution per ogni skill | No: tracking per le azioni gestite; skill native utilizzabili direttamente |
| Azione conclusa | Chiude la sua execution, lascia aperta la sessione |
| Cambio modello durante il lavoro | Vale dal prossimo turno; steering distinto |
| Provider default modificato | Non modifica provider/ambiente delle sessioni esistenti |
| Inattività | Nessuna scadenza della conversazione |
| Restart | Ripresa del contesto nativo; lavoro attivo e job hanno garanzie separate e osservate |
| Skill | Catalogo dell'harness effettivo nel cwd effettivo |
| Processi di sviluppo | Tool/job del runtime, con disponibilità HTTP e cleanup verificati |
| ARcipelago | Filesystem e capacità del runner, non quelli del browser |
| Dati legacy | Consultabili; nessun resume completo inventato |
| Vecchie decisioni Wiki/test | Sostituibili; conservare integrità dei dati e accuratezza degli esiti |
| CLI pubblica | Eventuali incompatibilità dichiarate e versionate secondo AGENTS.md |

## Verifica finale richiesta

La consegna completa deve dimostrare, per ciascun provider supportato:

- stesso conversation ID e riferimento nativo attraverso messaggi e resume;
- storia oltre la vecchia finestra, salvata anche senza browser collegato;
- modello ed effort applicati al turno corretto e default workspace invariato;
- skill specifiche del provider disponibili, skill dell'altro harness non proposte;
- modifica di un file e avvio/controllo/stop di un server realmente raggiungibile;
- plan e implement gestiti nella stessa sessione, con effetti verificati e conversazione ancora utilizzabile;
- stop e crash senza replay cieco di effetti o perdita di storia;
- interazioni verificate in un browser reale e protocolli verificati con i runtime reali.

Il dettaglio di prove eseguite, non eseguite e limiti va in accettazione.md, creato dal task 12. Nessun requisito può essere considerato provato soltanto dalla presenza di un campo nell'API o da un fake che restituisce il risultato desiderato.

## Nota per il task esterno ARcipelago

Se apri direttamente una sessione nel repository ARcipelago, passa il blocco del prompt 10, contesto.md e contratto.md prodotti in ARchetipo. Gli esiti dell'hub/runner devono riportare repository e revisione. La sessione 11 va avviata di nuovo in ARchetipo con quell'esito disponibile. Questo passaggio è esplicito perché non conosciamo ancora il percorso del repository remoto.

## Stato dei file preparati

Creati: questo indice, contesto condiviso e dodici prompt. Aggiornato l'indirizzo dell'analisi iniziale. Nessuna sessione di implementazione avviata, nessuna modifica al codice applicativo, nessuna prova live dichiarata eseguita in questa preparazione.
