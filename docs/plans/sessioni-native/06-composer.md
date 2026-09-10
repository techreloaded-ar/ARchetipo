# 06 — Aggiornare View per sessioni e scelte per turno

Dipendenze: 03, 04, 05.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 06: Aggiornare View per sessioni e scelte per turno.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 03, 04, 05.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Rendere usabile nella UI il nuovo lifecycle con stessa rail entry, nuovo messaggio dopo giorni e modello/effort per il prossimo turno.

Punti di ingresso da verificare sul codice corrente:
cli/internal/web/assets/app.js, conversation.js, conversation-index.js, componenti del selettore modello, rotte web e test/web.

Passi:
1. Riutilizza il design e i componenti esistenti; elimina il percorso che cambiava conversation ID al resume. Il composer rimane utilizzabile a idle e durante una disconnessione recuperabile con stato spiegato.
2. Mostra provider e ambiente effettivi della conversazione. Popola il selettore dal catalogo del provider di quella sessione, non da quello divenuto default nel workspace.
3. Consenti modello/opzioni per il prossimo turno anche mentre il corrente lavora; distingui selezione futura da impostazioni del turno attivo. Gestisci modelli senza effort e scelte rifiutate senza perdere la bozza.
4. Definisci esplicitamente invio durante attività: steering dove supportato oppure messaggio per il turno successivo. Mostra quale comportamento avrà il comando; non trattare lo steering come nuovo turno con modello nuovo.
5. Separa stop, archiviazione e rimozione dall'elenco. Mostra errori di resume/sessione mancante senza creare automaticamente altri thread. Mantieni input e approvazioni utilizzabili dopo aggiornamenti della timeline.
6. Rendi paginabile la storia mantenendo scroll e ordine. Il frontend rappresenta stati/capacità del backend e non contiene nomi di provider per scegliere regole di processo.

Criteri di completamento:
Click reali provano cambio modello/effort, invio a idle, comportamento durante attività, interrupt, resume e caricamento storia. Nessuna perdita di bozza o doppia conversazione, nessuna eccezione JS; interazioni da tastiera funzionanti.

Verifica:
npm run test:web e browser smoke reale costruito sul supporto page-load già presente. Aggiorna gli smoke che prima richiedevano nuova conversazione al resume. Check Go comuni se tocchi il backend.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Non sviluppare ancora un nuovo menu skill: lo implementa il task 07.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/06.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
