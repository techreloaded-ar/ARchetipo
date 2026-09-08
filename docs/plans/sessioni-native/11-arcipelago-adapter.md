# 11 — Collegare View alle sessioni ARcipelago

Dipendenze: 02, 03, 06, 07, 08, 09 e implementazione/contratto effettivi del task 10.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 11: Collegare View alle sessioni ARcipelago.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 02, 03, 06, 07, 08, 09 e implementazione/contratto effettivi del task 10.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Adattare il provider ARcipelago al nuovo contratto senza introdurre differenze di lifecycle nella UI.

Punti di ingresso da verificare sul codice corrente:
cli/internal/execution/arcipelago, discovery, metadati sessioni remote, fake hub e smoke run/conversation.

Passi:
1. Leggi l'esito 10 e verifica che gli endpoint descritti esistano nella versione accessibile. Se il file è esterno, usa il percorso consegnato dall'utente o chiedilo; non ricostruire a memoria il contratto.
2. Persisti e riusa riferimenti a sessione/task/runner/ambiente; riaggancia sessioni e stream dopo restart di View senza creare nuovi task per messaggi successivi.
3. Invia scelte di modello/opzioni per turno e usa il catalogo skill del runner. Pubblica capability veritiere per quel runner: controlli non supportati restano indisponibili con un motivo.
4. Preserva cursori, deduplica, approvazioni e richieste d'input su reconnect. Distingui sessione inattiva, runner offline, turno terminato e sessione irrecuperabile.
5. Instrada anche le azioni gestite nella stessa sessione, mantenendo la verifica degli effetti dove il connector può osservarli e dichiarando eventuali problemi di sincronizzazione del checkout.
6. Verifica errori dopo creazione remota e prima della risposta locale, cambio default endpoint e disconnessioni. Niente segreti persistiti o reinvio implicito di operazioni.
7. Rimuovi il codice del vecchio percorso task-per-conversazione dove sostituito. La UI comune deve funzionare tramite capability e ambiente, senza regole hardcoded per ARcipelago.

Criteri di completamento:
La stessa conversazione remota continua dopo più turni e restart, con modello/skill del runner e un'identità stabile. Un run chiuso non viene finto attivo; un runner offline non distrugge la conversazione. Il conteggio dei task dimostra assenza di duplicazioni.

Verifica:
Test adapter contro fake del contratto reale, smoke View remoto esistenti aggiornati e una prova integrata hub-runner. Usa il set di accettazione condiviso dei provider, con limiti espliciti.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Se ARcipelago non è disponibile, consegna l'adapter verificato col fake come parziale; il task 12 non deve chiamare completa la parità remota.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/11.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
