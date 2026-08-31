---
type: decision
title: Template di processo del workspace
description: Il processo di un workspace è un valore builtin nominato, risolto per ID all'inizializzazione e conservato nella configurazione
status: reviewed
decision_status: accepted
sources:
    - path: cli/internal/template/template.go
      role: implementation
    - path: cli/internal/template/template_test.go
      role: verification
    - path: cli/internal/config/config.go
      role: implementation
    - path: cli/internal/cli/init_project_cmd.go
      role: implementation
    - path: cli/internal/cli/config_cmd.go
      role: implementation
    - path: cli/internal/cli/spec_cmd.go
      role: implementation
    - path: cli/internal/cli/cli_test.go
      role: verification
    - path: .archetipo/config.yaml
      role: workspace-config-template
review:
    content_hash: sha256:5dbb9f02f4363c118bb3c96e0636e25de718d50c5ed9612a4365e26b2fd45b6f
    evidence_revision: c5ab7abcca10d18e5fe3aed38948e04689b95c6f
    evidence_hash: sha256:2b103b940fddcc16897c9ae0be4e3069385c5e01e9c35e471c16e51bee3ed527
    reviewed_at: "2026-08-31T15:02:21Z"
---
# Template di processo del workspace

<!-- archetipo:wiki section=context -->
## Contesto

Il processo installato da `archetipo init` era implicito. La lista delle skill viveva in una variabile del comando di inizializzazione e gli stati del workflow soltanto nel `config.yaml` impacchettato: un workspace riceveva un processo ma non poteva dichiarare quale, e nessun'altra parte del sistema poteva interrogarlo.

Questo diventa un limite nel momento in cui il processo smette di essere l'unico possibile. Serve un oggetto nominabile che possieda skill e stati, che il workspace conservi, e su cui una funzionalità successiva possa appoggiare le azioni ammesse per ciascuno stato di una spec.

<!-- archetipo:wiki section=decision -->
## Decisione

Il Template è un **valore builtin nominato**: porta identificativo, versione, etichetta, lista di skill, azioni offerte su una spec e stati del workflow. Il binario contiene una sola definizione, `fabbrica-del-software`: ID vuoto e ID di default la risolvono direttamente, ogni altro ID produce lo stesso errore tipizzato usato dal contratto precedente. Non esistono registry, ordine di registrazione o duplicate check per una variabilità che il prodotto non offre.

Il package `template` diventa l'unico punto in cui il processo è scritto: `init` installa `tpl.Skills`, e `uninstall` e `doctor` continuano a leggere la stessa lista attraverso una variabile derivata dal Template di default. La duplicazione precedente sparisce senza toccare quei due comandi.

Il Template dichiara anche le proprie **azioni**, cioè i passi che il processo offre su una spec. Ogni azione porta quattro cose: un identificativo stabile, che è ciò su cui un programma si appoggia; un'etichetta destinata a una persona; la skill che la realizza, così il chiamante può scegliere il risultato senza conoscere il nome interno della skill; e l'elenco degli stati in cui l'azione è ammessa. La Fabbrica del software dichiara `plan` / «Pianifica» / `archetipo-plan` per `TODO`, `implement` / «Implementa» / `archetipo-implement` per `PLANNED` e `IN PROGRESS`, `review` / «Rivedi» / `archetipo-review` per `REVIEW`. `DONE` non dichiara alcuna azione. Gli stati ammessi sono i valori canonici del dominio, gli stessi che il Template porta già in `Statuses`, e non le etichette configurabili del workflow: il confronto avviene sul vocabolario che il connector restituisce.

Il filtro per stato restituisce le azioni ammesse in ordine di dichiarazione e restituisce **sempre** un elenco, eventualmente vuoto. Uno stato senza azioni ammesse — `DONE`, ma anche uno stato sconosciuto o la stringa vuota — non è un errore e non è un'assenza: è un elenco vuoto. L'elenco è non-nil per costruzione, così nella rappresentazione JSON resta `[]` e non diventa `null`, e un client non deve distinguere due forme per lo stesso significato. La copia difensiva già applicata a `Skills` si estende alle azioni e agli stati che ciascuna dichiara: ogni risoluzione restituisce valori separati dalla definizione builtin.

La lettura è `archetipo spec actions US-XXX`. Legge dal connector lo stato realmente persistito della spec, risolve il Template selezionato dal workspace e restituisce un envelope `spec_actions` con il codice e lo stato della spec, l'identità del Template e le azioni ammesse in quello stato. La forma è per spec e non per stato perché è il ricalcolo al cambiare dello stato reale a dover essere osservabile, e perché una lettura parametrizzata sullo stato introdurrebbe nell'interfaccia una seconda sorgente di verità su di esso; la forma per stato resta ottenibile in futuro senza rompere questa. La versione riportata è quella del Template **risolto** in-process, cioè della definizione da cui le azioni provengono: riportare accanto a esse la versione persistita sarebbe una falsa attribuzione. La coppia identificativo/versione conservata in `.archetipo/config.yaml` resta esposta da `config show`, ed è lì che quel dato appartiene. Un identificativo di Template sconosciuto nella configurazione fallisce con `E_INVALID_INPUT` e l'elenco degli id validi, nella stessa forma già usata dall'inizializzazione; una spec inesistente resta l'`E_PRECONDITION` del connector, invariato.

Il workspace conserva la selezione in `.archetipo/config.yaml`, in un blocco di primo livello `template:` con `id` e `version`. L'assenza del blocco non è un errore: la risoluzione dei default riempie i due campi con il Template predefinito, quindi un workspace inizializzato prima dell'introduzione dei Template continua a valere come Fabbrica del software. Il valore della versione è una costante del package e non la versione della CLI, che vale `dev` nei build locali e identificherebbe il binario invece del processo.

La risoluzione del Template precede qualunque scrittura. È la prima istruzione dell'inizializzazione, prima della scoperta della data directory, della creazione delle directory dei tool e della copia delle skill: un identificativo sconosciuto fallisce con `E_INVALID_INPUT` quando sul filesystem non è ancora accaduto nulla, ed è questa proprietà — non il codice d'errore — a rendere vero «senza lasciare un'inizializzazione parziale».

La scrittura del blocco nel file di configurazione è testuale, come la riscrittura della riga `connector:`, e non un round-trip `yaml.Node` sull'intero documento. Il round-trip riformatterebbe indentazione e righe vuote del config generato, cioè cambierebbe in modo osservabile ciò che un'inizializzazione predefinita produce oggi.

La rilettura passa dall'envelope di `archetipo config show`, dove le skill leggono già ogni metadato di workspace. Il campo è assemblato nel comando, non in `domain.SetupInfo`: il Template non dipende dal connector, e i campi preesistenti dell'envelope restano invariati.

<!-- archetipo:wiki section=alternatives -->
## Alternative

- **Etichetta di sola configurazione**: scrivere `template.id` e `template.version` nel config lasciando la lista di skill nella CLI. Scartata perché il Template non definirebbe nulla — sarebbe un nome senza contenuto — e la funzionalità successiva sulle azioni per stato dovrebbe reintrodurre il concetto da capo.
- **Template come directory di asset su disco**: descrivere ogni Template con file distribuiti nel pacchetto. Scartata perché introduce un formato, un caricatore e una superficie d'errore che il perimetro corrente esclude esplicitamente: niente secondo Template, niente marketplace, niente ereditarietà.
- **Campo `Template` su `domain.SetupInfo`**: scartata perché obbligherebbe i quattro connector a trasportare un dato che non dipende dal connector, per un valore che il comando possiede già.
- **Validazione dell'ID anche al caricamento della configurazione**: scartata perché renderebbe illeggibile un workspace scritto da una CLI più recente. Il rifiuto appartiene al momento della scelta, non a ogni lettura.
- **Round-trip `yaml.Node` per scrivere il blocco**: scartata perché riformatterebbe il config generato, un cambiamento osservabile del comportamento predefinito.
- **Riusare `execution.ActionID` e `execution.Capability` per le azioni del Template**: tipizzare l'identificativo dell'azione con il vocabolario dell'esecuzione remota e associare a ogni azione la capacità richiesta. Tecnicamente possibile — `execution` non importa `template`, quindi non nasce alcun ciclo — e apparentemente un riuso gratuito. Scartata perché quel vocabolario conosce oggi soltanto `plan`: la risoluzione della capacità restituisce un errore tipizzato per qualunque altra azione. Dichiarare lì `implement` e `review` prometterebbe un confine di esecuzione remota che nessun provider implementa, cioè un contratto falso su un'integrazione, non una duplicazione risparmiata. Il costo accettato è la convivenza di due vocabolari di azione, uno descrittivo del processo e uno dell'esecuzione remota; il vincolo che li tiene allineati è che l'identificativo condiviso resta letteralmente `plan`, così `archetipo spec actions` e l'esecuzione remota nominano la stessa azione.
- **Lettura per stato (`template actions --status`) invece che per spec**: scartata perché il ricalcolo da dimostrare è quello che segue lo stato reale di una spec, e perché richiedere allo stato di essere dichiarato dal chiamante lo trasformerebbe in una seconda sorgente di verità accanto al connector.

<!-- archetipo:wiki section=consequences -->
## Conseguenze

Le skill del processo hanno un solo punto di verità, e i comandi che le leggono restano invariati. La configurazione del workspace guadagna un contratto persistito retrocompatibile, perché l'assenza del blocco risolve il default. L'envelope di `config show` rende la selezione interrogabile da un programma, che era il presupposto su cui poggiano le azioni offerte dal Template.

Quella promessa è ora mantenuta: il Template non possiede soltanto skill e stati, ma anche le azioni e il vincolo di stato che le ammette, e il workspace ha una sola superficie di lettura che combina lo stato reale di una spec con quella definizione. Un client può quindi proporre a una persona un risultato desiderato — «Pianifica», «Implementa», «Rivedi» — senza conoscere il nome interno della skill che lo produce, e senza scrivere da nessun'altra parte quale passo sia ammesso in quale stato: la CLI non acquisisce alcuna regola di processo propria. L'assenza di azioni ammesse è un elenco vuoto in un envelope di successo, non un errore, quindi un consumatore non ha alcun caso speciale da gestire per uno stato terminale.

Il costo è la convivenza di due vocabolari di azione: quello descrittivo del processo, che vive nel package `template`, e quello dell'esecuzione remota. Aggiungere un'azione al processo non la rende eseguibile da remoto, e i due insiemi vanno tenuti allineati a mano sull'identificativo condiviso `plan`. Restano inoltre fuori da questa decisione qualunque condizione di ammissibilità oltre lo stato della spec, qualunque esecuzione dell'azione a partire dall'elenco restituito, e qualunque azione di ri-pianificazione offerta su una spec già pianificata.

Il costo è anche che un Template vive dentro il binario: aggiungerne uno richiede una release della CLI. È coerente con il perimetro corrente ed è reversibile verso i Template su disco senza cambiare ciò che il workspace persiste, perché ciò che è persistito è soltanto la coppia identificativo/versione.

Restano fuori da questa decisione un secondo Template, l'ereditarietà fra Template, l'aggiornamento automatico di un workspace quando la versione del Template cambia, e qualunque riconciliazione fra la versione registrata e quella del binario che la legge: oggi una differenza fra le due non produce né avviso né migrazione.

<!-- archetipo:wiki section=verification -->
## Verifica

Il tipo, il Template builtin e la risoluzione diretta sono in [template.go](../../../cli/internal/template/template.go); i suoi test verificano la lista completa delle skill, gli stati canonici, l'ID vuoto che risolve il default, l'errore tipizzato su ID sconosciuto e l'assenza di aliasing degli slice in [template_test.go](../../../cli/internal/template/template_test.go).

Il tipo `Action`, le tre azioni della Fabbrica del software e il filtro `ActionsFor` sono nello stesso [template.go](../../../cli/internal/template/template.go), insieme alla copia difensiva estesa alle azioni e ai loro stati. [template_test.go](../../../cli/internal/template/template_test.go) scrive per esteso la lista attesa delle azioni e la confronta per intero, così che modificare il processo rompa un test in modo esplicito; verifica che identificativo, etichetta e skill siano presenti su ciascuna azione; percorre con una tabella tutti e cinque gli stati canonici, ottenendo `plan` in `TODO`, `implement` in `PLANNED` e `IN PROGRESS`, `review` in `REVIEW` e nessuna azione in `DONE`; asserisce che l'elenco vuoto sia lungo zero **e** non-nil per `DONE`, per uno stato sconosciuto e per lo stato vuoto; e verifica che modificare le azioni ottenute non raggiunga la definizione builtin.

La lettura `archetipo spec actions US-XXX` è la foglia registrata in [spec_cmd.go](../../../cli/internal/cli/spec_cmd.go), che emette l'envelope `spec_actions`. I test di integrazione in [cli_test.go](../../../cli/internal/cli/cli_test.go) eseguono la CLI reale su un workspace temporaneo con il connector su filesystem e portano una sola spec lungo l'intero ciclo di vita `TODO → PLANNED → IN PROGRESS → REVIEW → DONE`, rileggendo le azioni dopo ogni transizione: nessun doppio si interpone fra lo stato scritto su disco e l'elenco restituito. Il caso `DONE` è asserito con una type assertion su una lista JSON, che fallisce su `null` e rende quindi osservabile la scelta dell'elenco vuoto; lo stesso file verifica che l'envelope nomini il Template risolto, che un codice spec mancante sia `E_INVALID_INPUT` e che una spec inesistente resti `E_PRECONDITION`.

Il contratto persistito e la sua risoluzione per default sono in [config.go](../../../cli/internal/config/config.go), coperti da [config_test.go](../../../cli/internal/config/config_test.go): workspace senza blocco, selezione esplicita conservata e riempimento della sola versione mancante.

Il flag, la risoluzione anticipata e la scrittura testuale del blocco sono in [init_project_cmd.go](../../../cli/internal/cli/init_project_cmd.go); la rilettura nell'envelope in [config_cmd.go](../../../cli/internal/cli/config_cmd.go). I test di integrazione in [cli_test.go](../../../cli/internal/cli/cli_test.go) eseguono il binario reale: installano le skill del Template, confrontano l'inizializzazione implicita con quella esplicita byte per byte sul config prodotto, e verificano che un identificativo sconosciuto non lasci sul filesystem né le directory delle skill né i file di runtime.
