# Seconda prova indipendente del percorso prodotto

30 ottobre 2026. Nessun push, PR, tag o pubblicazione su ARchetipo. Non sono stati
modificati RideAtlas, digital_twin o il repository privato della prova Prestiti.

## Scenario e risultato

Installazione npm locale da due pacchetti costruiti dai sorgenti, in un prefisso
nuovo, e progetto Git con soli README e gitignore. Provider Codex nativo,
connettore file, nessun backlog iniziale e nessun sorgente applicativo preparato
dal collaudatore. Tutte le richieste prodotto e i permessi passano dal viewer.
Il collaudatore verifica il browser; non scrive il codice dell'applicazione.

La prima installazione da `b2b3eb9` si blocca: una richiesta di permesso del
worker scompare quando termina il turno del parent. La prova fallita è conservata.
Dopo le correzioni, nuova installazione in un altro prefisso e nuovo progetto:

1. Richiesta ordinaria: segnalare problema, luogo e urgenza della sede di una
   associazione; risolvere e conservare i dati nel browser. Codex produce
   specifica, app, ricetta qualificata, nove test e commit locale `b1569bc`.
2. Prova reale: Rubinetto/Bagno urgente e Lampadina/Ingresso normale; reload,
   risoluzione, riapertura. Porta/Sala riunioni lasciata risolta. Nessun overflow
   orizzontale a 390 px.
3. Feedback nella stessa chat: filtro Solo urgenza alta, disattivato al reload,
   dati precedenti preservati. Specifica v2, dieci test e candidato
   `71d231dca562e8334ceb72e4d66cb865d7daeada`.
4. Browser: il filtro mostra un aperto su due; la risolta resta consultabile;
   reload ripristina tutti gli aperti. Tre record conservati, console senza
   errori nella prova. Anche v2 non supera i 390 px disponibili.
5. Chiusura e riavvio del viewer: stessa conversazione e sessione nativa, risposta
   corretta sull'ultima modifica e il commit, worktree pulito. Riapertura Preview:
   stesso indirizzo pubblico e tre record precedenti. Il servizio interno può
   cambiare porta, senza cambiare l'origine pubblica del browser.
6. Dossier esportato dalla UI: richiesta iniziale, specifiche v1/v2, candidati,
   verifiche e osservazioni conservati. Nessuna approvazione business o PR reale.

La prova è sintetica e condotta da un agente: dimostra il percorso tecnico, non
la facilità d'uso per persone nuove senza assistenza.

## Difetti corretti

- Provider scelto nelle impostazioni: ora il compositore si abilita nello stesso
  viewer senza riavvio e senza mantenere il vecchio avviso di indisponibilità.
  Le impostazioni che richiedono un riavvio continuano a dichiararlo.
- Fine turno parent: non cancella più richieste pendenti dei worker Codex.
  Dopo reload si può rispondere o fermare il worker esatto, conservando concluso
  il turno parent. La conclusione di un worker non consuma richieste dei fratelli.
- Skill: una qualifica divenuta obsoleta è una precondizione da ristabilire,
  non un permesso da chiedere nuovamente. Coordinare gli scrittori prima della
  qualifica finale; leggere gli esiti, anche quando il comando termina con zero.

Regressioni riprodotte prima della correzione e passate dopo; smoke browser
specifici per onboarding e permessi worker dopo fine parent, reload e stop.

## Attriti e limiti rimasti

La ricetta generata include `src` e `index.html` negli input di preparazione.
Durante la revisione li modifica: benché i check dichiarino `during_preview`,
la preparazione non è più valida e il CLI richiede correttamente stop/prepare.
L'agente lo esegue e View riavvia automaticamente al risultato pronto. Il monitor
che pretendeva preview ininterrotta durante il feedback fallisce e resta tale in
`stability.json`: non è un soak superato. Migliorare la scelta proporzionata degli
input di setup e rendere più chiara la pausa, mantenendo completa la copertura
dei check e della readiness. Non togliere input necessari per ottenere un verde.

Le richieste di permesso su ricetta comune e commit locali interrompono ancora
il percorso. Sono state risposte una volta, senza regole persistenti o modifiche
alla politica del runtime. La gestione dell'ambiente resta il passaggio più
costoso di questa piccola app. Il browser del Codex nel prodotto era disabilitato
nelle impostazioni Work: ha dichiarato il limite e non simulato verifiche visive.

Nessuna PR reale in questo giro: il percorso fino alla pubblicazione e agli
aggiornamenti developer è verificato dallo smoke con GitHub simulato. Non è una
prova dell'hosting reale. Nessuna nuova CI remota o esecuzione Windows; è stata
verificata solo la compilazione dei test Windows. Restano utenti nuovi,
sessioni di giorni e progetti con dipendenze/servizi più complessi.

## Controlli locali ed evidenze

Superati: gofmt, vet, build, tutti i test Go, golangci-lint (zero issue),
412 test frontend, smoke pagina (onboarding incluso), chat (worker incluso),
incremento e preview, benchmark ambiente. Race detector su web e Codex e
30 ripetizioni delle regressioni worker superati. Le misure del benchmark
descrivono il CLI, non la velocità end-to-end dell'agente.

Run ignorato: `test/workspaces/release-rehearsal/2026-10-30T08-01-29Z/`.
`retest.json` identifica versione, pacchetti e SHA-256; `source-fixes.patch`
conserva il sorgente installato; `go-checks.json` e i log documentano i controlli;
`final-acceptance.json`, `dossier-v2.md` e screenshot documentano il percorso.
L'installazione corretta è nel prefisso `installation-fixed`, versione
`0.0.0-dev.gb2b3eb9.dirty`; non è una nuova release npm e non sostituisce la
precedente installazione globale. Gli hash identificano il contenuto effettivo.

Conversazione `conv-9daf29f8509df9cc1db70d40ba96de41`, sessione Codex
`01a18bf9-31ef-7771-9633-c04264013a77`, incremento `inc-8f8fb56d34c38ec4`.
Viewer locale <http://127.0.0.1:43841/>, app <http://127.0.0.1:59689/>.
