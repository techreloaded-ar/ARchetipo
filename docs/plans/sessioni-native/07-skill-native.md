# 07 — Scoprire e invocare le skill dell'harness

Dipendenze: 01, 04, 05, 06.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 07: Scoprire e invocare le skill dell'harness.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 01, 04, 05, 06.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Mostrare e invocare nella stessa conversazione esclusivamente le skill effettivamente disponibili al runtime selezionato.

Punti di ingresso da verificare sul codice corrente:
Discovery provider Claude/Codex, contratto di sessione, composer View, fixture e test web/protocollo.

Passi:
1. Usa skills/list per Codex e la fonte del catalogo Claude provata in 01. Verifica la versione reale; non inventare un'API simmetrica se il protocollo non la offre.
2. Conserva nome, descrizione, namespace/origine e modalità di invocazione necessaria; rispetta disabilitazioni e invocabilità esplicita. Gestisci duplicati/precedenze come il runtime.
3. Esponi un catalogo associato a sessione, harness e cwd. Aggiornalo quando cambia l'ambiente o il runtime segnala modifiche; un errore di discovery non significa elenco vuoto certo.
4. Implementa suggerimenti/selezione nel composer e invio strutturato al provider: skill input per Codex dove supportato, invocazione nativa Claude. Mantieni la skill nello stesso turno/sessione e non leggerne il corpo nel frontend.
5. Non registrare obbligatoriamente execution per skill native, comprese quelle ARchetipo invocate direttamente. Il percorso nativo non passa da una nuova proposta obbligatoria o da un secondo agente.
6. Prova skill di fixture solo Claude, solo Codex, condivisa, disabilitata e con namespace plugin. Dopo un effetto sul workspace la board si aggiorna attraverso lo stato reale.

Criteri di completamento:
La skill solo Claude non appare in Codex e viceversa; la condivisa funziona in entrambi; il risultato del tool dimostra l'invocazione nella stessa sessione. Aggiungere un file nell'altra directory non cambia il catalogo sbagliato.

Verifica:
Test provider con cataloghi reali/fake osservati, test web e click browser sul menu. Probe live con skill fixture che produce un output riconoscibile; nessun test limitato al testo del prompt.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Se la versione Claude non espone un catalogo completo, risolvi con il minimo percorso nativo supportato e documenta la copertura; non dichiarare il task completo mostrando skill ipotetiche.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/07.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
