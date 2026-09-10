# 12 — Accettazione finale, rimozione del vecchio modello e documentazione

Dipendenze: 01–11; per una consegna solo locale, dichiarare esplicitamente 10/11 incompleti.

Copia tutto il blocco seguente in una nuova sessione AI coding con accesso al repository. Non occorre incollare l'intera conversazione originale.

```text
Lavora al task 12: Accettazione finale, rimozione del vecchio modello e documentazione.

Il prodotto ARchetipo cambia direzione: le conversazioni di View devono diventare sessioni native degli harness, persistenti e operative, con modello/effort per turno e skill/tool reali. Le skill di processo devono poter lavorare nella stessa conversazione. Le scelte di business precedenti in codice, test e Wiki possono essere sostituite: non fermarti a chiedere conferma per conservare una regola superata.

Leggi AGENTS.md, docs/plans/sessioni-native/contesto.md e gli esiti delle dipendenze disponibili. Dipendenze: 01–11; per una consegna solo locale, dichiarare esplicitamente 10/11 incompleti.
Se questo prompt viene eseguito nel repository ARcipelago, il contesto condiviso deve essere fornito insieme al prompt; segui anche le sue istruzioni locali.

Obiettivo:
Verificare l'intera nuova direzione del prodotto, eliminare le semantiche superate e preparare una consegna coerente di codice, test, documentazione e migrazione.

Punti di ingresso da verificare sul codice corrente:
Intero percorso View/provider/skill interessato, test/e2e, test/web, docs/wiki, documentazione utente, AGENTS.md e note di release.

Passi:
1. Leggi gli esiti dei task e verifica sul codice che tutti i criteri siano soddisfatti. Nessun task è completo perché ha un file di handoff o test fake verdi.
2. Esegui scenari end-to-end da View: conversazione libera, file e server, cambio modello/effort, skill giuste, azione plan con domanda, implement nello stesso thread, stop, restart e ripresa. Verifica runtime effettivo e filesystem con oracoli osservabili.
3. Consolida i test credential-free condivisi tra provider e mantieni un insieme piccolo di probe live. Aggiorna i test legacy affinché asseriscano la nuova semantica, preservando isolamento, errori e integrità dei dati.
4. Cerca ed elimina codice, flag, fallback, UI e commenti che impongono il vecchio timeout, transcript-resume automatico, conversazioni read-only o equivalenza tra fine execution e fine sessione. Conserva il reader dei dati legacy necessario a proteggerli.
5. Aggiorna Wiki e documentazione utente: nuova direzione, decisioni sostituite, sessione/turno/execution, scelte per turno, skill native, ambiente locale/remoto, stop/archive/delete, dati legacy e capacità non disponibili. Documenta il cambiamento come implementato soltanto dove provato.
6. Allinea AGENTS.md e descrizioni degli smoke al comportamento reale. Non marcare pagine Wiki reviewed senza seguire la verifica prevista. Aggiorna note di release e migrazione; se sono cambiate API CLI incompatibilmente, applica la policy di versionamento e prepara i relativi file senza eseguire publish/deploy.
7. Controlla resource cleanup, compatibilità dei record precedenti e assenza di modifiche accidentali al config dell'utente. Rimuovi soltanto fixture e processi creati dai test.

Criteri di completamento:
Report finale con matrice provider × capacità, prove osservate, migrazione e limiti. Tutto il percorso locale è usabile da browser; remoto completo solo se la prova integrata passa. La documentazione corrente non presenta più le vecchie restrizioni come regole valide.

Verifica:
Check Go comuni e conformance di tutti i connector; npm run test:web; npm run test:e2e:unit; browser page-load e smoke conversazione/execution pertinenti disponibili in package.json; E2E live mirati. Riporta fallimenti preesistenti provati e ogni skip.
Applica inoltre le verifiche e la consegna comune di contesto.md quando pertinenti.

Output specifico:
Salva docs/plans/sessioni-native/accettazione.md oltre a esiti/12.md. La relazione deve distinguere pronto al rilascio locale da obiettivo completo sui tre provider.

Implementa il task, non limitarti a proporre un altro piano. Mantieni il lavoro circoscritto e completa i test appropriati. Non svolgere i task successivi. Consegna docs/plans/sessioni-native/esiti/12.md con risultato, contratti modificati, verifiche, limiti e handoff. Conserva le modifiche preesistenti dell'utente.
```
