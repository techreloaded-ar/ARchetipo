# Direzione del nuovo prodotto

Il percorso principale è ora un lavoro sul problema: specifica evolutiva,
soluzione funzionante, osservazioni e decisioni. Non occorre creare un backlog
interno. In modalità `continuous`, `increment propose` accetta il solo `intent`
Markdown e rende la specifica lavorabile senza registrare un’approvazione umana.
Il documento conserva problema e persone, esempi verificabili, ipotesi, decisioni,
esclusioni e domande. Le storie rimangono leggibili per i lavori precedenti.

`increment learn <id> --file -` registra `observation` e un’eventuale `decision`.
L’osservazione non invalida le prove tecniche del candidato; modifiche a specifica
e codice richiedono invece un nuovo risultato verificato. `increment conclude`
usa lo stesso payload con decisione obbligatoria e permette di concludere senza
codice né PR. Non va dichiarata una validazione business senza osservazioni reali.

La consegna di un lavoro senza storie apre una PR con specifica e verifiche e non
crea epiche o storie nel backlog ufficiale. Le successive modifiche dei developer
sono ammesse: `approved_sha` conserva la versione provata dal business e
`integration_sha` registra il commit effettivo della PR quando risulta integrata.
Le prove precedenti non certificano le modifiche dei developer.

La skill di ingresso è stata ridotta a un metodo diretto. Le istruzioni storiche
sono caricate soltanto per record senza modalità continua. Nei nuovi worktree
continui si installano le sole skill del lavoro e dell’ambiente; pianificazione,
implementazione, review e autopilot non sono prerequisiti. I vecchi moduli del
viewer e del CLI sono ancora presenti: questa è la separazione del nuovo nucleo,
non la conclusione della rimozione del vecchio prodotto. Rimangono da affrontare
la disponibilità dell’anteprima durante il lavoro, la navigazione completamente
centrata sul prodotto e prove con persone e agenti reali.

---

# Lavorare su un’app dalla chat

La schermata iniziale offre direttamente il campo «Cosa vuoi costruire?».
Il primo invio crea un lavoro dedicato (incremento), il worktree e una sessione
nativa persistente. «Nuovo lavoro» torna a questa schermata; conversazione libera
e attività del processo rimangono nel menu secondario.

Il nuovo percorso crea l’incremento con `development_mode: continuous`. La richiesta
autorizza lo sviluppo locale: l’agente chiarisce le sole ambiguità rilevanti,
persiste intento e criteri con `increment propose` e prosegue senza richiedere
una seconda conferma. La revisione registra `approval_source: request`, distinta
da un’approvazione umana. Le revisioni dei lavori precedenti mantengono il percorso
con approvazione esplicita. L’opzione è disponibile anche standalone nel JSON di
`increment create`; valori diversi da `continuous` sono rifiutati.

I messaggi successivi e i commenti sugli elementi conservano lavoro, worktree e
sessione. Le richieste di modifica aggiornano la specifica e invalidano le vecchie
verifiche; le domande non cambiano il risultato accettabile. La chat mostra brevi
aggiornamenti sul risultato; log e dettagli restano consultabili separatamente.
La pubblicazione conserva il gate esplicito sulla versione verificata e non viene
autorizzata dalla modalità continua.

L’anteprima è affiancata alla chat fin dall’apertura del lavoro. Alla disponibilità
di un risultato verificato, con ricetta configurata e turno concluso, il browser
avvia automaticamente la preview gestita. Non si dichiara funzionante un’app in
assenza di una ricetta o readiness. Un errore non genera retry infiniti e il pulsante
Ferma resta operativo. Durante un nuovo turno continuo la preview viene sospesa,
conservando servizi e dati; dopo il turno può ripartire. Questo primo percorso non
promette una preview ininterrotta durante compilazione e verifiche.

Ogni lavoro usa la gestione esistente di porte, servizi posseduti, dati separati e
cleanup. Il worktree non è una sandbox di sicurezza del sistema operativo.
Una nuova app in un workspace contenente soltanto asset del workflow riceve una
base Git vuota, senza committare file runtime. Un’app esistente deve essere già
versionata; il sistema non importa né committa automaticamente sorgenti o segreti.
Il percorso continuo usa `main` se disponibile, altrimenti il branch corrente
committato. La creazione storica continua a richiedere `main`.

Verifica: `npm run test:view-chat-smoke` attraversa la pagina reale, il primo invio,
la specifica riconciliata dal CLI, l’avvio automatico dell’anteprima, il feedback,
il reload e il rifiuto di una pubblicazione con commit errato. Solo l’agente e
GitHub sono simulati; screenshot e report restano sotto `test/workspaces/`.

---

# Incrementi di prodotto

Un incremento è un percorso guidato disponibile fra le **Attività** della
conversazione. Le conversazioni libere e le azioni sulle singole storie
continuano a funzionare come prima.

## Percorso

1. Scegli **Sviluppa un incremento** e racconta il risultato desiderato.
2. L'agente chiarisce l'intento e propone storie piccole e verificabili.
3. Apri **Dettagli incremento**, leggi **Intento e storie** e premi **Approva intento e avvia sviluppo**.
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
