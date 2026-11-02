# Merge di v30 in v3.1 — 2 novembre 2026

## Riferimenti

- Destinazione iniziale: `v3.1`, `a0cd6a530bd8e50fafdafc5c1aa7153e24033035`.
- Sorgente: `origin/v30`, `0931320446929ccb6ad2ae0c3e1929dcae754cac`.
- Base comune: `f4209b3b85ab9598a56eaeaaa94bd83f9d4bbc57`.
- Divergenza: 23 commit esclusivi della sorgente, 9 della destinazione.
- La simulazione iniziale ha rilevato conflitti in 21 file, inclusi due conflitti modifica/eliminazione.

Il merge è stato preparato nel worktree `ARchetipo-merge-v30-v31`, sul branch
`codex/merge-v30-into-v3.1`, conservando il checkout originale e i suoi file non
tracciati. Il merge commit mantiene entrambi i parent; non è uno squash dei branch.

## Decisioni concordate

| Aspetto | Scelta |
| --- | --- |
| Processo | Adottare v30: lavoro direttamente dalla chat, specifica evolutiva, sviluppo e verifiche nello stesso incremento. |
| Backlog | Collegare le storie business originali; nessun backlog interno e nessun avanzamento automatico delle storie al merge. |
| Approvazione iniziale | Nessuna approvazione formale obbligatoria dell'intento prima dello sviluppo. Le domande di prodotto restano decisioni della persona. |
| Approvazione della PR | Approvare il candidato business prima della pubblicazione; conservare SHA e momento dell'approvazione separati dalla head successivamente integrata dai developer. |
| Head modificata dai developer | Non richiedere la riapprovazione introdotta in v3.1; il merge dei developer non viene presentato come approvazione business del codice successivo. |
| Ambiente dopo il merge | Conservarlo. Archiviazione e rimozione richiedono la scelta esplicita prevista da v30. |
| Vocabolario | “Integrato”, senza equiparare il merge al deploy. |

## Cosa arriva da v30

- Processo `fabbrica-del-software` revisionato, specifica e decisioni versionate,
  collegamenti bidirezionali con le storie business e dossier esportabile.
- Ritiro delle skill `archetipo-autopilot`, `archetipo-implement`,
  `archetipo-review` e `archetipo-design`, con aggiornamento dei metodi rimasti,
  del runtime condiviso, della documentazione e delle azioni offerte dal viewer.
- Gestione delle ricette di ambiente candidate: qualifica, pubblicazione comune,
  verifica della configurazione effettiva di ciascun checkout, evidenze originali
  e controlli dichiarati compatibili con la preview attiva.
- Preview persistente, recupero, dettagli dell'ambiente, richieste tecniche e
  report consultabili; arresto dei processi gestiti e protezione dell'ownership.
- Correzioni dei lock e delle scritture su Windows, dei permessi dei worker,
  del focus del dossier, delle bozze e dell'attivazione del primo provider.
- Controllo automatico della PR senza rimozione dell'ambiente e CI con smoke
  del prodotto e benchmark dell'ambiente.

Questionario, feedback dalla preview, file locali dichiarati e varie interazioni
elencate nel report precedente di Stefano erano già nella base comune. Le loro
varianti sono state conciliate, senza considerarli tutte nuove aggiunte di v30.

## Cosa viene conservato da v3.1

- Allegati immagine nei messaggi e negli avvii delle attività; persistenza e
  distribuzione agli adapter Claude e Codex, compreso il primo turno dell'incremento.
- Immagini lette e prodotte dall'agente, percorsi del workspace, striscia delle
  immagini lette e gestione dei contenuti immagine nei protocolli nativi.
- Composer compatto che dispone modello e Invia sotto il testo quando va a capo.
- Archiviazione dalla rail con conferma inline e scelta dell'ambiente quando dovuta.
- Mockup come immagini, quando la modifica alla UI li richiede: la direzione scelta
  viene registrata nella specifica evolutiva, senza ricreare le storie interne.
- `squash_commit.title` obbligatorio, con una sola riga non vuota, e corretta lettura
  dei nomi dei campi del risultato nei payload strutturati della CLI.

## Compromessi e risoluzioni

- La PR usa titolo e descrizione suggeriti per lo squash, seguiti dal dossier
  completo di v30. Il requisito sul titolo non esegue né impone un merge squash.
- La conversione automatica dei codici delle storie interne nei commit non viene
  conservata: quel modello non esiste più. La skill usa gli originali codici delle
  storie collegate quando la convenzione del progetto lo richiede.
- Le immagini sono state combinate con il trattamento degli eventi dei worker:
  i loro eventi tecnici non spezzano una risposta del parent in streaming.
- Lo steering dei due adapter conserva sia gli allegati sia il testo destinato
  allo storico, anziché perdere uno dei due lati del conflitto.
- Il nuovo avvio dalla chat conserva il parametro `development_mode` e può
  trasportare gli allegati senza confondere i due argomenti.
- Le bozze persistenti di v30 e gli allegati di v3.1 si conservano insieme quando
  l'invio fallisce; il retry raggiunge la stessa sessione nativa.
- I test del workflow ritirato sono sostituiti dalle verifiche del processo v30;
  restano i test delle immagini e sono stati estesi gli smoke del flusso combinato.

## Verifiche

Prima del merge, `go test ./...` e `npm run test:web` passavano su v3.1.
Sul risultato tutti i check seguenti sono passati:

- `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`.
- `golangci-lint run --timeout 5m ./...`: 0 issue (binario già installato in `~/go/bin`).
- `npm run test:web`: 434 test, nessun fallimento o skip.
- `npm run test:e2e:unit`: tutti i test passati.
- `npm run test:view-page-load-smoke`.
- `npm run test:view-chat-smoke`.
- `npm run test:view-increment-smoke`.
- `npm run test:view-preview-smoke`.
- `npm run test:view-increment-deletion-smoke`.
- `npm run test:harness-benchmark`.

Gli smoke usano CLI, viewer, Git, servizi locali e Chrome reali, con agente e
hosting simulati: non costituiscono un collaudo con un LLM o GitHub reali. I check
locali sono eseguiti su macOS; la matrice Windows/Linux rimane compito della CI.
I timing del benchmark misurano il contratto dell'ambiente, non la velocità end-to-end
dell'agente. Report e screenshot restano nelle directory ignorate del worktree di
verifica, sotto `test/workspaces/`.

Gli incrementi del vecchio modello restano consultabili; v30 rifiuta di modificarli
attraverso il nuovo workflow continuo. Il merge non migra dati di workspace esterni.
Per proseguire quel lavoro si avvia un nuovo incremento dalla richiesta originale.

I log dei check sono raccolti anche in
`test/workspaces/merge-v30-v31/2026-11-02/` nel worktree di verifica.
Il checkout originale conserva i mockup e lo script locale non tracciati presenti
all'inizio. Non è stato eseguito alcun push.
