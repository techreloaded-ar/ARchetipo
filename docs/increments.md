# Lavoro sul prodotto

Il termine tecnico `increment` identifica un lavoro durevole sul prodotto. Il suo
risultato può essere una soluzione funzionante da consegnare ai developer oppure
una decisione motivata che non richiede altro codice.

## Capire l’intento in chat

Un nuovo lavoro parte dalla richiesta e approfondisce problema, persone, risultato
atteso, esempi concreti e vincoli prima di costruire. La conversazione segue una
domanda alla volta e riusa le risposte già date. Le prospettive ARchetipo rendono
esplicito chi interviene: Andrea sul valore, Costanza sulle ipotesi e alternative,
Livia sull’esperienza, Leonardo sulla fattibilità ed Emanuele sui requisiti.
Sono voci dello stesso assistente; intervengono quando aggiungono qualcosa.
Nella prima parte Andrea, Costanza e Livia fanno brainstorming nello stile di
inception: brevi scambi, alternative concrete, ipotesi messe alla prova, tecniche
«E se…», cambio di pubblico e anti-problema. La conversazione converge sulle
scelte della persona senza imporre un numero fisso di turni. Durante la
costruzione intervengono anche Ugo (sviluppo), Mina (test e prove) e Cesare
(revisione e rischi). Le voci riferiscono solo attività effettive: una diversa
personalità non costituisce una revisione indipendente.

Quando il runtime offre input strutturato, le domande mostrano risposte suggerite
selezionabili e, quando consentito, una risposta libera. Si sceglie e si invia;
nessuna opzione viene scelta automaticamente. Le richieste native Codex e
`AskUserQuestion` di Claude usano lo stesso modulo del viewer; le domande di Claude
con `multiSelect` permettono più scelte. Una consegna fallita conserva la selezione
per riprovare. Se lo strumento strutturato non è disponibile, la domanda continua
nella chat. Una richiesta già precisa o l’invito a procedere abbreviano
l’esplorazione; le modifiche successive non ricominciano un questionario.

## Contratto

Un lavoro conserva richiesta originale, specifica Markdown versionata, riferimenti
alle storie originali, feedback, osservazioni, decisioni e risultati delle verifiche.
Ogni risultato è legato a un commit pulito e alla specifica provata. Una modifica
esplicita revoca il candidato corrente, conservando prove e versione precedenti.
Una domanda o un’osservazione non riscrivono i requisiti e non approvano codice.
`ready` richiede i report originali di **tutti** i check dichiarati nella ricetta:
comandi conclusi con successo, input correnti, preparazione valida e log integri.
`checks` è una spiegazione leggibile, non una prova. Report, impronte degli input,
exit code e hash dei log vengono congelati con il commit; la pubblicazione li
ricontrolla. Una verifica fallita successiva revoca un precedente successo.

La sessione nativa e il worktree rimangono gli stessi nelle revisioni e nei recuperi.
L’indice vive in `.archetipo/increments/`; il record e il lock del lavoro nel suo
worktree, sotto `.archetipo/increment-local/`. Gli artefatti del runtime non entrano
nei commit dell’app. Il workspace principale e i suoi file non committati non sono
copiati nel worktree.

La base è `main` quando esiste, altrimenti il branch corrente committato. In un
workspace vuoto può essere inizializzato Git con un commit vuoto; sorgenti di app
esistenti non vengono importati automaticamente. L’agente lavora soltanto nel
worktree assegnato, con la politica di permessi del provider.

## Operazioni CLI

Tutte restituiscono `archetipo/v1`, `kind: increment`, record in `data`, salvo il
dossier che restituisce `data.id` e `data.markdown`.

| Operazione | Payload / effetto |
| --- | --- |
| `increment create --file -` | `{"title":"Titolo","request":"Problema"}`; crea ambiente di lavoro e branch |
| `increment show <id>` / `list` | Legge stato o lavori attivi, senza mutazioni |
| `increment link <id> --file -` | `{"story_codes":["US-010"]}` prima della prima specifica |
| `increment propose <id> --file -` | `{"intent":"Markdown","story_codes":["US-010"]}`; versiona specifica e riferimenti |
| `increment ready <id> --file -` | `{"summary":"Come provarlo e limiti","checks":["Controllo reale e riferimento alla prova"]}`; congela candidato pulito e committato |
| `increment feedback <id> --file -` | `{"feedback":"Comportamento da cambiare"}`; richiede una nuova specifica/verifica |
| `increment learn <id> --file -` | `{"observation":"Osservazione reale","decision":"Decisione facoltativa"}` |
| `increment conclude <id> --file -` | Stesso payload, decisione obbligatoria; conclude senza pubblicare codice |
| `increment publish <id> --file -` | `{"revision":2,"head_sha":"commit visualizzato"}`; approvazione esplicita del candidato e PR idempotente |
| `increment refresh <id>` | Rilegge PR; distingue SHA approvato e successivo head integrato dai developer |
| `increment dossier <id>` | Esporta richiesta, versioni, storie, prove, decisioni e consegna; funziona anche dopo rimozione dell’ambiente |

Da dentro il worktree, `-C <workdir>` seleziona il lavoro; il suo riferimento
runtime permette ai comandi increment di risalire al record proprietario senza
scrivere nel backlog principale. `story_codes` assente conserva i collegamenti;
un array esplicito li aggiorna nella nuova revisione. Titolo e corpo originali
restano nella copia versionata. I riferimenti non richiedono né piani né transizioni
sintetiche delle storie.

## Esperienza e gestione aziendale

Il primo messaggio della home avvia direttamente il lavoro. In alternativa,
**Backlog → storia → Lavora su questa storia** porta gli stessi requisiti in un
nuovo lavoro. Nel dettaglio della storia si trovano lavori e PR collegati;
nel lavoro si torna alle storie originali. Se titolo o criteri della storia sono
cambiati nel backlog, il lavoro segnala lo scostamento e conserva la specifica
originalmente usata: aggiornare i requisiti richiede una nuova revisione esplicita.

**Specifica e decisioni** apre il pannello del dossier. Il pannello conserva
revisioni, criteri, feedback e prove; **Esporta dossier** scarica il Markdown.
**Documentazione** conserva la Wiki consultabile e le sue operazioni dedicate.
PRD, epiche, priorità, stati, stime e piani sono strumenti di gestione facoltativi.
Zero punti indica una storia non stimata. Gli stati del backlog non approvano
codice, non fanno merge e non attestano valore business.

La PR contiene il dossier e il commit approvato dal business. I developer possono
modificare la soluzione senza attribuire alle loro modifiche la verifica precedente.
`approved_sha` e `approved_at` conservano la decisione originaria;
`integration_sha` è l’head della PR osservata come integrata, non il commit di deploy.
Una PR chiusa senza merge non è una consegna.

## Ambiente locale

Le ricette `execution environment` e `execution preview` gestiscono risorse,
porte, preparazione, verifiche e cleanup. Lavori distinti hanno risorse e dati
separati; le revisioni riusano quelli del lavoro corrente. Le configurazioni
portabili si propagano dal progetto; checkpoint e possesso dei processi no.

La preview si apre quando l’app è configurata e il turno termina, anche durante
lo sviluppo. Rimane aperta durante domande, feedback e modifiche; i server con
ricaricamento automatico mostrano gli aggiornamenti senza creare un altro ambiente.
**In lavorazione** distingue l’app corrente dalla **Versione verificata**: quest’ultima
richiede commit pulito e prove valide. Solo il server abilita l’approvazione.
**Ferma** resta esplicito anche dopo reload. Il browser conserva la scelta per
il checkout: una risposta successiva della chat non riavvia l’ambiente; occorre
premere di nuovo Preview.

Un check può dichiarare `during_preview: true` se non reinstalla dipendenze,
non riscrive gli output del server e non azzera dati condivisi. Il check usa la
preparazione esistente; se cambia, richiede di fermare la preview e preparare di
nuovo. Quando il proprietario dell’ambiente termina, il check viene annullato e
il supervisore procede al cleanup. Build incompatibili, setup e pulizia restano
esclusivi. `execution environment evidence` legge le prove senza eseguire comandi.

La ricetta comune include gli helper espliciti e si riusa nei worktree; installazioni,
porte, dati e report rimangono locali a ciascuno. Gli installer devono rispettare il
lockfile: un lock incoerente si corregge esplicitamente, senza fallback silenziosi
a versioni risolte nuovamente. Questo rende ripetibile l’ambiente nei futuri worker.

Worktree e porte separate non costituiscono isolamento di sicurezza del sistema
operativo. Runtime remoto e sandbox di sicurezza sono fasi successive. Anche il
ciclo di modifica e riapprovazione dopo l’apertura della PR resta a una fase
successiva: la consegna corrente rimane congelata per i developer.

## Rimozione e storico

La cancellazione richiede il piano di rimozione, conferma e SHA osservato. Ferma
solo risorse possedute, esegue il cleanup dichiarato e rimuove branch e worktree.
Resta un riferimento Git di recupero per i commit e una ricevuta con il dossier
business. Il dossier e i collegamenti alle storie rimangono consultabili anche
quando i file dell’ambiente non esistono più. La conservazione della conversazione
è una scelta distinta; il transcript nativo del provider non viene cancellato.

Il contratto del vecchio incremento — copie delle storie, classificazioni
correzione/refinement/scope e approvazione preventiva dell’intento — è rimosso.
I record storici sono leggibili senza ripristinare quella macchina a stati.
Le skill di implementazione, review, autopilot e mockup non sono distribuite;
`init` elimina soltanto quelle directory gestite, conservando skill personali.

## Verifiche

`test:view-chat-smoke` usa la pagina, CLI e adattatore Codex reali con agente finto.
`test:view-increment-smoke` aggiunge l’ingresso dal backlog, il recupero dopo morte
del processo, riferimenti bidirezionali, dossier, approvazione umana, PR unica e
riconciliazione di modifiche dei developer. GitHub è simulato. Le prove su browser
non attestano la qualità del codice prodotto da un modello reale.


### Prima configurazione e riparazione della ricetta

La skill `archetipo-environment` ricostruisce requisiti, strumenti e copertura dai
file effettivi del progetto, anche in un clone appena inizializzato. Non esiste
un catalogo di stack scelto dal viewer. Il contesto passato dal viewer indica
provider, directory e profilo richiesto; le capacità reali si osservano nella
sessione con `execution environment capabilities`, che funziona senza ricetta.
Tool e probe aggiuntivi sono espliciti; rete e servizi non provati restano ignoti.
I report non espongono valori delle variabili d'ambiente.

`environment configure --candidate` e `preview configure --candidate` preparano
file locali senza aggiornare il bundle comune. `environment qualify --asset ...`
esegue due preparazioni effettive e una prova di riuso con readiness; conserva
report e log originali e fallisce su input instabili. `publish --qualified` adotta
atomicamente solo lo stesso bundle con evidenze ancora valide. Una nuova
qualificazione revoca il risultato precedente prima di iniziare. I comandi legacy
restano disponibili, ma la sessione tecnica del viewer richiede la pubblicazione
qualificata: una vecchia ricetta comune non dimostra che una riparazione sia finita.

La qualificazione riguarda la ripetibilità del setup: non certifica da sola
installazione da zero, integrità semantica dei dati o correttezza del prodotto.
La skill sceglie prove specifiche per installazione pulita in risorse dedicate,
conservazione delle fixture e stop/ripresa. I check prodotto restano distinti e
obbligatori per la readiness del candidato; non si indeboliscono per pubblicare
una ricetta. I permessi restano quelli dell'harness, senza escalation automatica.


### Preview prima di un nuovo lavoro

La schermata iniziale offre «Apri l’app attuale» senza creare una conversazione
prodotto o un incremento. Le route `/api/workspace/preview` usano il checkout
servito dal viewer e gli stessi contratti standalone di configurazione,
preparazione e preview delle conversazioni. Se manca la ricetta viene avviata
soltanto la sessione tecnica. Un'app già configurata può essere esplorata anche
senza un agente disponibile. I commenti diretti richiedono una conversazione
aperta; la preview iniziale consente navigazione e gestione dell'ambiente.

Il server legge da Git branch o commit effettivo e distingue un worktree dal
checkout principale. Il contesto, sempre visibile anche a preview ferma, espone
la directory e, se presenti, lavoro e conversazione aperta. `main` non è imposto:
la preview esegue i file del checkout selezionato, incluse modifiche locali. Più
conversazioni sulla stessa directory condividono il processo; il nome della chat
indica il contesto aperto, non un secondo proprietario del servizio.


### Richieste e cronologia dell’ambiente

La Preview mostra una sezione permanente Ambiente: stato di preparazione,
esiti delle verifiche e collegamento alla chat tecnica. Domande strutturate,
richieste di autorizzazione e messaggi da completare restano rispondibili nel
pannello, usando gli stessi endpoint e la stessa sessione nativa della chat.
Le opzioni vengono dall’harness; il viewer non decide autorizzazioni. Il polling
non ricrea un modulo invariato, così mantiene testo, scelta e focus. Una risposta
fallita conserva il modulo; dopo una consegna riuscita il controllo resta disabilitato
finché il runtime non annuncia la richiesta successiva.

Il browser conserva per workspace l’ultima chat selezionata, incluse quelle
tecniche, e lo stato del pannello nella sessione del browser. Ricaricare legge
la stessa cronologia senza avviare un turno. La chat tecnica resta separata
dall’elenco prodotto, ma è sempre raggiungibile dall’Ambiente.

`execution environment checks` legge gli ultimi report originali, compresi quelli
falliti. Una nuova preparazione non li elimina: li presenta da riconfermare quando
non corrispondono più agli input e alla preparazione correnti. Report/log mancanti
o alterati non diventano successi. I log sono consultabili come testo inerte dal
pannello. Gli errori di tentativi precedenti restano storici quando la preview
attuale ha una preparazione verificata; non descrivono più lo stato corrente.
