# 05 — Completare le sessioni native Codex

Dipendenze: 01, 02, 03; in sequenza consigliata dopo 04 per recepire gli aggiustamenti comuni.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 05: Completare le sessioni native Codex.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 01, 02, 03; in sequenza consigliata dopo 04 per recepire gli aggiustamenti comuni.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Aggiungere conversazioni libere Codex persistenti e multi-turn attraverso app-server.

Punti di ingresso da verificare sul codice corrente:
cli/internal/execution/codex, discovery del provider e test/e2e/support/fake-codex.mjs.

Passi:
1. Implementa il nuovo contratto di sessione per Codex: thread persistente, salvataggio dell'ID, thread/resume e più turn/start sullo stesso thread. Elimina ephemeral per le conversazioni durevoli.
2. Usa turn/steer solo per un turno attivo e turn/start per un nuovo turno. Interrompere un turno deve lasciare la sessione riprendibile.
3. Applica modello ed effort a turn/start e registra le scelte accettate. Non mutare i default del workspace; preserva la stessa identità al resume.
4. Sostituisci il rifiuto indiscriminato delle richieste server con il ponte verso le approvazioni/input di View, usando gli schemi della versione supportata. Conserva sandbox e policy dell'harness.
5. Gestisci correttamente stati terminali del turno, errori, strumenti ed eventi dopo reconnect, senza dedurre il successo dal semplice arrivo di turn/completed.
6. Verifica esecuzione sul cwd effettivo e uso dei tool senza spec obbligatoria; pubblica solo le capacità dimostrate. Elimina i percorsi ormai inutili che assumevano un solo turno.

Criteri di completamento:
La stessa suite comportamentale di Claude passa anche per Codex: due turni, scelta per turno, resume nativo, file scritto, approvazione/input e interrupt senza perdita di sessione. Nessun thread effimero né nuova rail entry al resume.

Verifica:
Fake app-server controllato, test adapter/core e smoke HTTP su View reale. Probe live con binary supportato; check Go comuni.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Non ricreare la UI per Codex: usa lo stesso contratto introdotto per tutti i provider.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/05.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
