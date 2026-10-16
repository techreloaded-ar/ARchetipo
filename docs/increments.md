# Incrementi di prodotto

Un incremento è un percorso guidato disponibile fra le **Attività** della
conversazione. Le conversazioni libere e le azioni sulle singole storie
continuano a funzionare come prima.

## Percorso

1. Scegli **Sviluppa un incremento** e racconta il risultato desiderato.
2. L'agente chiarisce l'intento e propone storie piccole e verificabili.
3. Leggi **Intento e storie** e premi **Approva intento e avvia sviluppo**.
4. Pianificazione, implementazione e verifica usano le skill esistenti dentro
   la sandbox. Il risultato comprende istruzioni di prova, verifiche eseguite
   e, quando il progetto la prevede, un'anteprima HTTP locale.
5. Scrivi in chat cosa cambiare. Una domanda sul risultato lascia valida la
   verifica; una richiesta esplicita di modifica avvia una breve esplorazione, seguita da
   una nuova proposta da approvare. Codici locali, conversazione nativa e
   worktree restano gli stessi.
6. **Approva risultato e apri PR** congela la versione provata, pubblica il
   branch e apre una sola PR verso `main`. Le storie finali entrano nel backlog ufficiale
   come proposte, in TODO e con il collegamento alla PR.
7. **Aggiorna stato PR** riconcilia lo stato effettivo di GitHub. Solo un merge
   confermato del codice approvato porta le storie a DONE e l'incremento a
   consegnato. ARchetipo non esegue il merge.

L'interfaccia riusa il pannello di conversazione e le sue approvazioni native.
Non introduce una seconda board, una chat diversa o nuovi stati delle storie.

## Isolamento e persistenza

L’indice sta in `.archetipo/increments/<id>.json`; lo stato corrente e il suo
lock restano nel worktree, sotto `.archetipo/increment-local/increment.json` e
`.archetipo/conversations/`. I record precedenti vengono letti senza modifiche
e migrano nello stesso worktree al primo aggiornamento. Il worktree dedicato sta in
`.archetipo/increments/worktrees/<id>`, sul branch
`archetipo/increments/<id>`. La base è il commit del branch locale `main`,
che deve essere disponibile prima della creazione. Il branch corrente del
workspace di origine e le sue modifiche non committate non vengono copiati.

Il worktree contiene il normale codice e la configurazione versionata.
Un piccolo riferimento runtime, `.archetipo/increment-workspace.json`,
seleziona un backlog file locale sotto `.archetipo/increment-local/` e
disabilita i worktree per singola storia. Il caricamento della configurazione
riconosce esplicitamente questa radice anche senza `config.yaml` locale.
Le impostazioni del padre vengono lette senza prove di scrittura; le verifiche
dei percorsi si eseguono solo dopo averle riportate nella sandbox. I comandi
delle skill non risalgono
al backlog principale. PRD, Wiki, mockup e risultati puntano al checkout
isolato; percorsi configurati esterni al progetto sono rifiutati.

I file runtime e le skill installate non entrano nei commit del prodotto.
L'installazione copia soltanto gli asset del workflow, senza file .env.
Il worktree isola codice e processo, non costituisce una barriera di sicurezza
del sistema operativo. L'agente conserva la politica di permessi del provider.
L'anteprima deve usare dati e servizi di prova separati, seguendo il comando di
avvio già documentato dal progetto. Le skill predispongono il necessario
ambiente di prova locale usando gli strumenti del progetto.

L'incremento mantiene tutte le revisioni dell'intento e le evidenze dei
risultati precedenti. All'approvazione di una revisione:

- le storie invariate conservano piano, stato e lavoro verificato;
- le storie cambiate mantengono il codice locale ma perdono piano e accettazione
  precedenti, così da essere ripianificate e verificate;
- le storie ritirate escono dal backlog interno; lo sviluppo deve riconciliare
  anche codice, test e documentazione con il risultato corrente.

Il record, la sessione nativa e le identità di pubblicazione sopravvivono al
riavvio. Interrompere un turno non elimina l'incremento o la sessione.
Una conversazione libera di origine resta distinta dalla nuova conversazione
creata direttamente nel worktree.

## Pubblicazione

La prima versione è locale, lavora sulle storie in sequenza e apre PR GitHub
tramite il comando `gh` già autenticato dall'utente. Serve un remote
`origin` scrivibile. Il backlog ufficiale continua a usare il connector
configurato nel workspace originale.

Prima della pubblicazione la CLI verifica nuovamente il commit, la pulizia
del checkout, il contenuto delle storie accettate e il completamento dei task.
Una modifica dopo la prova richiede una nuova verifica. Il mapping fra codici
locali e codici ufficiali viene salvato prima di pubblicare; una pubblicazione
parziale si riprende con **Approva risultato e apri PR**, senza duplicare la PR
o le storie già scritte. Un codice occupato da contenuti diversi viene
rifiutato, senza sovrascriverli.

La PR rimane soggetta alla normale revisione tecnica. Una PR chiusa senza merge
non consegna l'incremento. Se commit della PR o storie pubblicate cambiano dopo
l'approvazione, la riconciliazione si ferma e segnala il conflitto.
Lo stato della PR si aggiorna su richiesta; non vengono avviati monitor.

## Uso standalone e CLI

La skill `/archetipo-increment <id>` espone lo stesso metodo anche senza il
viewer. I comandi deterministici usano il normale envelope JSON
`archetipo/v1`, con `kind: increment`:

- `increment create --file -`: `{"title":"…","request":"…"}`
- `increment list` e `increment show <id>`: record persistenti.
- `increment propose <id> --file -`: `{"intent":"…","stories":[…]}`.
- `increment approve <id> --file -`: `{"revision":N}`.
- `increment ready <id> --file -`: `{"summary":"…","checks":["comando — risultato"],"preview_url":"http://127.0.0.1:3000"}`; anteprima facoltativa.
- `increment feedback <id> --file -`: `{"feedback":"…"}`.
- `increment publish <id> --file -`: `{"revision":N,"head_sha":"commit provato"}`.
- `increment refresh <id>`: stato effettivo della PR e riconciliazione.

Le approvazioni standalone devono essere richieste dalla persona sulla
versione effettivamente mostrata. Il testo dell'agente non sostituisce gli
effetti persistenti verificati dalla CLI.

## Verifica

`npm run test:view-increment-smoke` attraversa l'interfaccia reale in Chrome,
con il server, il connector file, Git, i worktree e l'adapter nativo reali.
Solo agente e GitHub sono simulati. Prova due giri, il riavvio della pagina,
la stessa sessione nativa, assenza di modifiche al prodotto principale,
pubblicazione ripetuta senza duplicati, consegna solo dopo merge e accesso
alla conversazione libera. Controlla anche il layout a 390 px e conserva
screenshot e report sotto `test/workspaces/`, ignorato da Git.

## Composizione e recupero

Increment compone spec e Autopilot; le skill di base e Autopilot non dipendono
dall’incremento. Checkpoint, recuperi della sessione e verifiche sono descritti
in [Esecuzioni riprendibili](execution-recovery.md).
