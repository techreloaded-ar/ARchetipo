# 02 — Definire e implementare il contratto delle sessioni

Dipendenze: 01, per le capacità locali che il contratto dichiara.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 02: Definire e implementare il contratto delle sessioni.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 01, per le capacità locali che il contratto dichiara.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Introdurre nel backend un contratto minimo, coerente con i protocolli provati, che separi sessione nativa, turno, collegamento ed execution.

Punti di ingresso da verificare sul codice corrente:
cli/internal/execution/conversation.go, run.go, provider.go, discovery.go, modelchoice.go, localrun e i tipi consumati da web.

Passi:
1. Leggi protocolli.md e disegna le operazioni necessarie: creazione/ripresa con handle durevole, nuovo turno con modello/opzioni/skill, eventi, interruzione del turno, rilascio e discovery delle capacità.
2. Definisci metadati della conversazione, configurazione non segreta che ne fissa provider/ambiente e riferimenti nativi. Il riferimento del provider non è il conversation ID di View e non coincide necessariamente con un execution ID.
3. Definisci stati e transizioni essenziali: inattività, turno attivo, attesa input/approvazione, disconnessione recuperabile e sessione non recuperabile. Evita un unico enum che confonda archivio, rete e lavoro.
4. Riutilizza RunEvent e il ponte di dialogo quando il significato coincide. Sostituisci le parti del vecchio contratto che impediscono il risultato; non mantenere un doppio framework indefinitamente.
5. Dichiara capacità in base al provider/ambiente, inclusi resume, modello/opzioni per turno, skill, input/approvazioni e steering. Non promettere funzioni per ARcipelago non ancora provate.
6. Definisci come il core correla turni e invii, registra modello richiesto/applicato e rappresenta una consegna incerta senza reinviare effetti. Mantieni compilabili gli adapter con capacità esplicitamente non ancora supportate.

Criteri di completamento:
Contratto compilabile, con un fake comportamentale che dimostra creazione, due turni, interrupt e resume. Nessuna regola di processo vive nel frontend o nel contratto nativo. Gli adapter incompleti non dichiarano capacità operative.

Verifica:
Test mirati delle capability e transizioni, test execution/localrun e check Go comuni.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Documenta il contratto in docs/plans/sessioni-native/contratto.md, con esempi di payload e mapping verso i protocolli provati; è un documento di implementazione, non una nuova fonte delle regole delle skill.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/02.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
