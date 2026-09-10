# 08 — Eseguire le azioni ARchetipo nello stesso thread

Dipendenze: 03, 04, 05, 06, 07.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 08: Eseguire le azioni ARchetipo nello stesso thread.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 03, 04, 05, 06, 07.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Mantenere il valore dei controlli di processo ARchetipo facendo lavorare la sessione già aperta, senza imporre il processo alle conversazioni libere.

Punti di ingresso da verificare sul codice corrente:
cli/internal/execution/service.go, actionprompt.go e ricevute, cli/internal/web/conversation_action.go, conversation_adopt.go, execution.go, Template e skill coinvolte.

Passi:
1. Separa creazione/finalizzazione del record execution dal possesso del processo dell'agente. Un'azione dei controlli View entra nella sessione esistente con identità execution distinta dalla conversazione.
2. Riutilizza le verifiche di ammissibilità del Template per i controlli gestiti e la verifica degli effetti persistiti. Non applicare tali controlli globalmente ai messaggi liberi o a ogni skill nativa.
3. Unisci i percorsi dei pulsanti/proposte opzionali in un solo avvio nella sessione. Quando non esiste ancora una conversazione, creala una volta e usa quella. Non spawnare un secondo agente per la stessa azione.
4. Gestisci azioni che attraversano più turni e risposte umane; correla ricevute all'azione corrente. Una ricevuta vecchia, citata o prodotta in un turno fallito non può chiudere una nuova execution.
5. Alla fine dell'azione chiudi il record, lascia viva la sessione e aggiorna board/timeline. Permetti successive azioni nello stesso thread e un nuovo messaggio libero; non duplicare rendering di run e conversazione.
6. Aggiorna skill/prompt soltanto dove servono davvero per la sessione persistente; rimuovi il divieto di continuare dopo la ricevuta. Se il lavoro passa a un worktree, conserva cwd effettivo e rientro dopo integrazione.
7. Mantieni utilizzabile il percorso CLI batch senza farne derivare la durata della sessione View. Per skill native prive di execution non fabbricare un record di successo retroattivo.

Criteri di completamento:
Da una sola conversazione: azione plan con domanda, piano verificato, cambio modello, implement e messaggio finale libero; stesso ID nativo e una sola sessione agente. Record execution distinti e chiusi correttamente. Una skill nativa lavora anche senza execution.

Verifica:
Test service e ricevute, smoke conversation-action/conversation-run/plan aggiornati e scenario E2E mirato con worktree. Copri invii duplicati, annullamento e risposta umana tra turni.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Le vecchie equivalenze «execution = conversazione» e «ricevuta = chiusura thread» devono sparire dal percorso nuovo e dalla documentazione corrente.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/08.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
