# 01 — Provare i protocolli nativi

Dipendenze: Nessuna.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 01: Provare i protocolli nativi.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: Nessuna.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Produrre prove eseguibili dei protocolli Claude e Codex necessarie a scegliere il contratto, senza implementare ancora il refactoring di View.

Punti di ingresso da verificare sul codice corrente:
cli/internal/execution/claude, cli/internal/execution/codex, test/e2e/support e i live_probe_test.go esistenti.

Passi:
1. Registra versioni installate e consulta le fonti ufficiali. Riutilizza il supporto liveprobe o un piccolo probe indipendente; nessuna nuova infrastruttura di test senza necessità.
2. Per ciascun harness prova due turni, chiusura/riapertura del runtime e resume dello stesso ID. Usa un'informazione casuale detta nel primo turno e richiesta dopo il resume, senza reinserirla nel nuovo prompt.
3. Prova cambio modello ed effort tra turni con riscontro dal protocollo, interrupt seguito da un nuovo turno, approvazione e richiesta d'input. Distingui opzione accettata, effettivamente riportata e non osservabile.
4. Prova discovery delle skill e invocazione di una skill di fixture. Per Claude individua come ottenere il catalogo effettivo e se il cambio modello/effort funziona sul nostro stream-json; confronta il fallback con resume dello stesso ID.
5. Prova scrittura di un file e avvio di un server HTTP locale di fixture, verifica con una richiesta HTTP e ferma il processo. Osserva cosa succede a fine turno e al rilascio del runtime.
6. Documenta limiti, messaggi/scambi minimi riproducibili, versioni e fallback ammessi. Non salvare credenziali né transcript personali. Non trasformare la discovery ARcipelago non eseguita in una capability presunta.

Criteri di completamento:
Per ogni capacità la matrice distingue provata, assente e non verificata. I probe devono essere rilanciabili e avere timeout tecnici espliciti. Se una prova live è bloccata, consegna il probe e identifica esattamente la prova necessaria prima di implementare la capacità.

Verifica:
Esegui i probe realmente disponibili e i test mirati del supporto modificato. Gli esiti dei fake non valgono come conferma dei vendor.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Salva anche docs/plans/sessioni-native/protocolli.md: sarà la fonte tecnica per i task 02, 04, 05, 07 e 10.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/01.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
