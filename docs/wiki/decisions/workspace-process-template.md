---
type: decision
title: Template di processo del workspace
description: Il processo di un workspace è una definizione registrata in-process, selezionata per ID all'inizializzazione e conservata nella configurazione
status: generated
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
    - path: cli/internal/cli/cli_test.go
      role: verification
    - path: .archetipo/config.yaml
      role: workspace-config-template
---
# Template di processo del workspace

<!-- archetipo:wiki section=context -->
## Contesto

Il processo installato da `archetipo init` era implicito. La lista delle skill viveva in una variabile del comando di inizializzazione e gli stati del workflow soltanto nel `config.yaml` impacchettato: un workspace riceveva un processo ma non poteva dichiarare quale, e nessun'altra parte del sistema poteva interrogarlo.

Questo diventa un limite nel momento in cui il processo smette di essere l'unico possibile. Serve un oggetto nominabile che possieda skill e stati, che il workspace conservi, e su cui una funzionalità successiva possa appoggiare le azioni ammesse per ciascuno stato di una spec.

<!-- archetipo:wiki section=decision -->
## Decisione

Il Template è una **definizione registrata in-process**: un valore con identificativo, versione, etichetta, lista di skill e stati del workflow, risolto per ID da un registry costruito dal binario. Il registry riproduce il pattern già accettato per i provider di esecuzione — stessa forma dell'errore tipizzato, stessa risoluzione per ID — così il repository ha un solo modo di modellare una selezione per identificativo.

Il package `template` diventa l'unico punto in cui il processo è scritto: `init` installa `tpl.Skills`, e `uninstall` e `doctor` continuano a leggere la stessa lista attraverso una variabile derivata dal Template di default. La duplicazione precedente sparisce senza toccare quei due comandi.

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

<!-- archetipo:wiki section=consequences -->
## Conseguenze

Le skill del processo hanno un solo punto di verità, e i comandi che le leggono restano invariati. La configurazione del workspace guadagna un contratto persistito retrocompatibile, perché l'assenza del blocco risolve il default. L'envelope di `config show` rende la selezione interrogabile da un programma, che è il presupposto su cui poggeranno le azioni offerte dal Template.

Il costo è che un Template vive dentro il binario: aggiungerne uno richiede una release della CLI. È coerente con il perimetro corrente ed è reversibile verso i Template su disco senza cambiare ciò che il workspace persiste, perché ciò che è persistito è soltanto la coppia identificativo/versione.

Restano fuori da questa decisione un secondo Template, l'ereditarietà fra Template, l'aggiornamento automatico di un workspace quando la versione del Template cambia, e qualunque riconciliazione fra la versione registrata e quella del binario che la legge: oggi una differenza fra le due non produce né avviso né migrazione.

<!-- archetipo:wiki section=verification -->
## Verifica

Il tipo, il registry e il Template builtin sono in [template.go](../../../cli/internal/template/template.go); i suoi test verificano la lista completa delle skill, gli stati canonici, l'ID vuoto che risolve il default, l'errore tipizzato su ID sconosciuto e l'assenza di aliasing dello slice in [template_test.go](../../../cli/internal/template/template_test.go).

Il contratto persistito e la sua risoluzione per default sono in [config.go](../../../cli/internal/config/config.go), coperti da [config_test.go](../../../cli/internal/config/config_test.go): workspace senza blocco, selezione esplicita conservata e riempimento della sola versione mancante.

Il flag, la risoluzione anticipata e la scrittura testuale del blocco sono in [init_project_cmd.go](../../../cli/internal/cli/init_project_cmd.go); la rilettura nell'envelope in [config_cmd.go](../../../cli/internal/cli/config_cmd.go). I test di integrazione in [cli_test.go](../../../cli/internal/cli/cli_test.go) eseguono il binario reale: installano le skill del Template, confrontano l'inizializzazione implicita con quella esplicita byte per byte sul config prodotto, e verificano che un identificativo sconosciuto non lasci sul filesystem né le directory delle skill né i file di runtime.
