# 04 — Completare le sessioni native Claude

Dipendenze: 01, 02, 03.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 04: Completare le sessioni native Claude.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 01, 02, 03.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Portare Claude al nuovo contratto, con sessioni persistenti, più turni, modello/effort variabili e tool reali.

Punti di ingresso da verificare sul codice corrente:
cli/internal/execution/claude, execution/conversationprompt.go, supporto fake-claude e integrazione backend delle conversazioni.

Passi:
1. Abilita la persistenza delle conversazioni, acquisisci e salva il session_id, implementa resume per ID e verifica che venga ripresa la sessione attesa. Non usare --continue generico dove ci sono più conversazioni.
2. Elimina il timeout totale/inattività della conversazione. Mantieni timeout tecnici e i limiti batch separati. Un result termina il turno, un interrupt non distrugge la conversazione.
3. Implementa modello/effort al confine dei turni usando il percorso provato in 01. Se riapri il processo con --resume conserva session ID, timeline e correlazioni e rendi veritiero lo stato delle impostazioni applicate.
4. Rimuovi dal prompt condiviso il divieto di agire, invocare skill e scrivere nel workspace, compreso l'obbligo di proporre ogni azione. Mantieni soltanto istruzioni utili per il contesto ARchetipo, senza sostituire il comportamento nativo dell'harness.
5. Conserva e completa il ponte di approvazioni e richieste d'input; carica le impostazioni/istruzioni/tool del runtime secondo la configurazione effettiva. Non aggirare le policy con bypass globale.
6. Verifica filesystem, cwd e ritorno a idle dopo tool. Aggiorna commenti e test che imponevano sessioni non persistenti o timeout globale.

Criteri di completamento:
Da View reale: due turni, cambio modello/effort, resume dopo restart con memoria nativa, scrittura di un file, approvazione, interrupt e messaggio successivo nella stessa sessione. Nessuna dipendenza dalla presenza di spec o skill ARchetipo.

Verifica:
Fake Claude controllato per i nuovi scambi, test Go e smoke conversazione/local-run pertinenti; ripeti il probe live attraverso l'adapter reale, senza sostituirlo con il solo fake.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
La UI completa arriva in 06: qui l'accettazione può usare le rotte HTTP reali.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/04.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
