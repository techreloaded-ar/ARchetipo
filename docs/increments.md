# Incrementi di prodotto

Un incremento è un percorso guidato disponibile fra le **Attività** della
conversazione. Le conversazioni libere e le azioni sulle singole storie
continuano a funzionare come prima.

## Percorso

1. Scegli **Sviluppa un incremento** e racconta il risultato desiderato.
2. L'agente chiarisce l'intento e propone storie piccole e verificabili.
3. Apri **Dettagli incremento**, leggi **Intento e storie** e premi **Approva intento e avvia sviluppo**.
   Le storie approvate entrano subito nel backlog ufficiale, con i codici
   definitivi, in IN PROGRESS. Una revisione approvata le aggiorna, ne aggiunge
   o ritira; eliminare l'incremento prima della PR le rimuove.
4. Pianificazione, implementazione e verifica usano le skill esistenti dentro
   la sandbox. Il risultato comprende istruzioni di prova, verifiche eseguite
   e, quando il progetto la prevede, un'anteprima HTTP locale. Se la ricetta
   dell'anteprima dichiara integrazioni simulate (`simulated`), il risultato e
   il pulsante di approvazione lo dicono: l'approvazione è consapevole e la
   prova con i servizi reali avviene dopo il merge.
5. Scrivi in chat cosa cambiare. Una domanda sul risultato lascia valida la
   verifica; una richiesta esplicita di modifica avvia una breve esplorazione, seguita da
   una nuova proposta da approvare. Codici locali, conversazione nativa e
   worktree restano gli stessi.
6. **Approva risultato e apri PR** congela la versione provata, pubblica il
   branch e apre una sola PR verso `main`. Le storie ufficiali passano a REVIEW
   con il collegamento alla PR.
7. **Aggiorna stato PR** riconcilia lo stato effettivo di GitHub. Solo un merge
   confermato del codice approvato porta le storie a DONE e l'incremento a
   consegnato. ARchetipo non esegue il merge. A quel punto il viewer rimuove da
   solo la sandbox (worktree, branch e servizi), conserva il ref Git di
   recupero e archivia la conversazione, che resta leggibile. Se la rimozione
   fallisce, l'incremento resta consegnato e la nota ne spiega il motivo.

L'interfaccia riusa il pannello di conversazione e le sue approvazioni native.
Non introduce una seconda board, una chat diversa o nuovi stati delle storie.

## Seguire il lavoro nella chat

Una barra compatta mostra titolo, fase dell’incremento, attività della sessione
ed eventuale conteggio delle storie completate. L’avviso generico «Lavoro in
corso» non viene duplicato sopra la chat dell’incremento; restano visibili
gli avvisi di recupero, blocco o interruzione. Le storie segnate in lavorazione
nel backlog della sandbox sono apribili direttamente dalla barra; non si presume
che la prima storia incompleta sia quella attiva.

**Dettagli incremento** apre un pannello laterale con tutte le storie, i loro
stati e criteri espandibili, intento, risultato da provare, verifiche e comandi.
La chat conserva lo spazio quando il pannello è chiuso. Su schermi piccoli il
pannello occupa la vista; **Chiudi** o Escape riportano alla chat. Apertura,
sezioni espanse e posizione di lettura si conservano durante gli aggiornamenti.

Lo stesso pannello serve gli incrementi già esistenti, senza migrare i dati.
Se la sandbox non è leggibile, intento, storie e risultati salvati restano
consultabili, mentre gli stati non verificabili sono dichiarati non disponibili.

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
I file locali non versionati che la persona dichiara in `local_files` della
ricetta (per esempio `.env.local`) arrivano nel checkout con la preparazione.
Il worktree isola codice e processo, non costituisce una barriera di sicurezza
del sistema operativo. L'agente conserva la politica di permessi del provider.
L'anteprima deve usare dati e servizi di prova separati, seguendo il comando di
avvio già documentato dal progetto. Le skill predispongono il necessario
ambiente di prova locale usando gli strumenti del progetto.

L'incremento mantiene tutte le revisioni dell'intento e le evidenze dei
risultati precedenti. All'approvazione di una revisione:

- le storie invariate conservano piano, stato e lavoro verificato;
- le storie cambiate mantengono il codice locale e lo storico degli stati ma perdono piano e accettazione
  precedenti, così da essere ripianificate e verificate;
- le storie ritirate escono dal backlog interno; lo sviluppo deve riconciliare
  anche codice, test e documentazione con il risultato corrente.

I commenti sugli elementi della preview restano nella stessa conversazione e
nello stesso incremento. La skill registra richiesta e riferimento all’elemento,
collegandoli ai codici delle storie interessate:

- `correction`: corregge un criterio già approvato, conserva i requisiti e riapre
  soltanto le storie collegate. Una nuova revisione di lavorazione conserva
  l’approvazione dell’intento e lo storico dei risultati; la correzione viene
  sviluppata e verificata senza richiedere un’altra approvazione dell’intento.
- `refinement`: cambia i criteri della stessa capacità, aggiorna le storie
  esistenti e richiede l’approvazione della nuova revisione. Non basta annotare
  il cambiamento nell’intento lasciando invariate le storie.
- `scope`: aggiunge o cambia l’ambito; può aggiungere storie nello stesso
  incremento, prima dello sviluppo e previa approvazione. Un commento non crea
  automaticamente un nuovo incremento. Dopo la consegna si avvia un nuovo
  incremento con una decisione esplicita.

La classificazione spetta alla skill; il CLI valida codici e transizioni.
Le domande mantengono valido il risultato. Ogni nuovo risultato verificato,
comprese le correzioni, richiede l’approvazione finale prima della pubblicazione.
Le revisioni riusano i servizi, le porte e i dati sintetici dell’ambiente corrente;
la sospensione della preview per tornare allo sviluppo non equivale a Ferma ambiente.

Il record, la sessione nativa e le identità di pubblicazione sopravvivono al
riavvio. Interrompere un turno non elimina l'incremento o la sessione.
Una conversazione libera di origine resta distinta dalla nuova conversazione
creata direttamente nel worktree.

La gestione di servizi, porte concorrenti e arresto è descritta in
[Ambienti e servizi locali](local-services.md). Le dipendenze dichiarate
condividono lo stesso ciclo di vita da CLI e view; i vecchi script con servizi
avviati in background richiedono una migrazione esplicita.

## Pubblicazione

La prima versione è locale, lavora sulle storie in sequenza e apre PR GitHub
tramite il comando `gh` già autenticato dall'utente. Serve un remote
`origin` scrivibile. Il backlog ufficiale continua a usare il connector
configurato nel workspace originale.

Prima della pubblicazione la CLI verifica nuovamente il commit, la pulizia
del checkout, il contenuto delle storie accettate e il completamento dei task.
Una modifica dopo la prova richiede una nuova verifica. Il mapping fra codici
locali e codici ufficiali viene salvato all'approvazione dell'intento, prima di
scrivere; una pubblicazione parziale si riprende con **Approva risultato e apri PR**,
senza duplicare la PR o le storie già scritte. Fino alla consegna l'incremento è
l'autorità sul contenuto delle sue storie ufficiali. Un codice occupato da una
storia che non appartiene all'incremento viene rifiutato, senza sovrascriverla.
Eliminare un incremento la cui PR non è stata mergiata ritira le sue storie
ufficiali. Con connector che non sanno cancellare (github, jira) una storia
ritirata torna a TODO e va chiusa a mano.

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
  Sono opzionali `kind` (`correction`, `refinement`, `scope`) e `story_codes`;
  i codici sono obbligatori per correction/refinement. Il payload storico senza
  classificazione mantiene il passaggio attraverso definizione e approvazione.
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

## Eliminazione degli ambienti

La rimozione confermata ferma il lavoro e i servizi posseduti dall’incremento,
esegue il cleanup e rimuove worktree, branch e file locali, incluse dipendenze,
file ignorati e modifiche non committate. Non crea archivi dell’ambiente.
Rimangono una ricevuta di eliminazione per i tentativi successivi e un riferimento
Git `refs/archetipo/deleted/<id>` per recuperare esclusivamente i commit salvati.
Per recuperarli si può creare un nuovo branch dal riferimento con
`git branch recupero-incremento refs/archetipo/deleted/<id>`.
La ricevuta permette anche di leggere una conversazione archiviata dopo averne
rimosso l’ambiente; non contiene una copia dei file del worktree.
