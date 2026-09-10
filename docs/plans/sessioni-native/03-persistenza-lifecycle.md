# 03 — Rendere durevoli conversazioni e timeline

Dipendenze: 02.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 03: Rendere durevoli conversazioni e timeline.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 02.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Implementare storage e orchestrazione backend affinché una conversazione conservi identità e storia anche senza browser e dopo restart di View.

Punti di ingresso da verificare sul codice corrente:
cli/internal/conversationlog, cli/internal/web/conversation*.go, session.go, run_follow.go e lo storage/runtime introdotto nel task 02.

Passi:
1. Versiona il record e conserva riferimento nativo, provider/ambiente, scelte del prossimo turno e correlazioni. Migra o leggi in modo compatibile i record precedenti, preservandone tutti i dati disponibili.
2. Scrivi eventi alla ricezione e indipendentemente dalle GET. Separa la finestra RAM dalla storia durevole e aggiungi lettura paginata. Preferisci l'attuale storage su file con append robusto e metadati atomici a un nuovo database.
3. Implementa apertura e resume dello stesso conversation ID, rilascio senza distruzione del contesto, stop del solo turno e archiviazione distinta. Non aprire automaticamente una nuova conversazione con un estratto.
4. Serializza comandi e possesso della stessa sessione tra tab e istanze View concorrenti. Il lock non deve bloccare le altre conversazioni e deve permettere recupero dopo crash.
5. Riconcilia reference nativo e stato osservabile al restart. Persisti invii/correlazioni in modo da distinguere messaggio non consegnato da consegna incerta, senza retry automatico di comandi con effetti.
6. Aggiorna le rotte e la proiezione backend affinché le sessioni persistenti possano essere usate dai prossimi task. Dichiara i limiti dei record legacy; rendi esplicita la semantica di delete.

Criteri di completamento:
Con un provider fake persistente: stesso ID dopo restart, oltre 2.000 eventi conservati senza letture HTTP, paginazione senza buchi, niente doppio turno su doppio invio e niente falso successo dopo crash. Cambiare provider default non sposta una sessione preesistente.

Verifica:
Test store/journal e crash recovery; smoke HTTP attraverso View reale con browser assente. Aggiorna conversation-history e conversation-multi quando il vecchio comportamento è superato, mantenendo isolamento e integrità.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Il task può essere verificato con fake del nuovo contratto: non deve implementare in parallelo i due adapter locali.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/03.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
