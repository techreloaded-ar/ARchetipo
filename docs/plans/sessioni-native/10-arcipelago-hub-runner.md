# 10 — Evolvere il contratto ARcipelago e il runner

Dipendenze: 01 e 02 per il contratto richiesto; 09 per allinearsi al comportamento locale verificato.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 10: Evolvere il contratto ARcipelago e il runner.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 01 e 02 per il contratto richiesto; 09 per allinearsi al comportamento locale verificato.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Verificare e implementare nell'hub/runner ARcipelago le capacità necessarie alle sessioni durevoli. Questo task può richiedere un repository distinto.

Punti di ingresso da verificare sul codice corrente:
Repository ARcipelago che contiene hub e runner; in ARchetipo leggere cli/internal/execution/arcipelago e docs/plans/sessioni-native/contratto.md.

Passi:
1. Prima di modificare, individua l'implementazione effettiva di hub e runner nei repository esplicitamente disponibili. Se non è accessibile, completa il gap report dell'adapter ARchetipo e chiedi soltanto il percorso/accesso mancante; non inventare endpoint o dichiarare il lavoro completato.
2. Mappa il contratto effettivo: task, run, sessione nativa, runner, ambiente, storia, stream e persistenza. Distingui reconnect del trasporto da resume dopo morte del runtime.
3. Implementa il minimo supporto necessario a creare/riprendere per riferimento durevole, avviare turni con modello/opzioni, interrompere il turno, conservare il contesto e recuperare l'assegnazione dopo crash senza duplicare task.
4. Esponi le capacità dell'harness effettivo e il catalogo skill dal runner con cwd e disponibilità, non dal filesystem dell'hub o del client. Inoltra approvazioni/input e conserva la corretta distinzione tra task concluso e sessione ancora utilizzabile.
5. Definisci scope dei riferimenti, retention e cursor/replay/idempotenza effettivamente supportati. Una risposta incerta non deve causare retry cieco di effetti. Riusa autenticazione e isolamento esistenti.
6. Verifica ambiente e URL dei processi avviati: filesystem remoto dichiarato, nessuna promessa di accesso al computer del browser. Non implementare port forwarding se esiste già un meccanismo nativo sufficiente.
7. Scrivi il contratto osservabile con esempi reali, errori stabili e versione minima che l'adapter ARchetipo potrà usare.

Criteri di completamento:
Hub/runner reali con harness di prova: sessione dopo restart, più turni e impostazioni, skill corrette, ritorno runner offline e nessun task duplicato. API e capability corrispondono all'implementazione, non soltanto a mock.

Verifica:
Suite previste dal repository ARcipelago e test contrattuali/integrati hub-runner. Riporta gli scenari non provati con runtime reali.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Handoff obbligatorio in docs/plans/sessioni-native/esiti/10.md nel repository su cui lavori, contenente repository/revisione e contratto API. Se è esterno ad ARchetipo, consegna il percorso assoluto del file: va reso disponibile alla sessione 11. Non lanciare una seconda sessione al posto dell'utente.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/10.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
