# ARchetipo View: Conversazione e Board

Revisione del mockup del 14 ottobre 2026. Si usa la struttura e il vocabolario esistenti. Nessuna frase promozionale, nuovo oggetto di dominio o funzionalità di portfolio.

## Struttura

- Barra superiore: identità del workspace, Conversazione, Board, Nuova spec e menu delle altre funzioni.
- Conversazione: indice dei thread, messaggi, campo di scrittura e collegamento alla spec quando presente.
- Board: colonne del processo, card delle spec, accesso al dettaglio e alle azioni.
- Dettaglio: Storia, Piano e, quando ci sono dati di revisione, Revisione. I comandi ammessi rimangono raggiungibili qui.

La chat e la board sono due accessi allo stesso lavoro. Si può descrivere un bisogno nella chat oppure scegliere una storia dalla board e avviare un passo da lì. Nessun passaggio obbliga a formulare un nuovo messaggio.

## Azioni dalla board

Il viewer attuale apre il dettaglio dalla card e mostra lì le azioni ricevute dal server. La revisione grafica propone una scorciatoia alle stesse azioni sulla card, senza aggiungere un'operazione.

Nel Template software esistente:

| Stato | Azione |
| --- | --- |
| TODO | Pianifica |
| PLANNED | Implementa |
| IN PROGRESS | Implementa |
| REVIEW | Rivedi |
| DONE | Nessuna azione di processo |

Questa tabella descrive il Template attuale. Nell'implementazione la UI legge azioni, etichette e disponibilità dal server; non contiene questa tabella come regola. Le azioni dal dettaglio, dalla board e dalla chat usano gli stessi controlli e le stesse operazioni.

La board esistente consente anche il riordino e il cambio di stato. Questi vanno mantenuti con i controlli sulle transizioni già presenti. Spostare una card e avviare un'azione hanno effetti distinti: trascinare in PLANNED non produce un piano; trascinare in DONE non dimostra l'accettazione di un risultato.

## Dati e limiti del mockup

Il mockup contiene uno snapshot delle 39 spec ordinate nel backlog del repository: 2 TODO e 37 DONE. Titoli, codici, priorità, punti, epic, stati, corpi e piani sono copiati dagli artefatti presenti. Le colonne PLANNED, IN PROGRESS e REVIEW sono vuote perché non ci sono spec in quegli stati. Non si spostano le storie per popolare il disegno.

Non sono copiati transcript di conversazioni. La chat parte vuota e non genera risposte predefinite. I pulsanti di azione permettono di controllare la collocazione del comando ma non avviano esecuzioni; la disponibilità effettiva del provider non è verificata. Nuova spec apre un modulo senza salvare dati. Il mockup non riproduce tutti i campi del modulo esistente.

I pannelli del menu sono soltanto voci di navigazione esistenti. La Revisione non è mostrata nel dettaglio perché questo snapshot non include un dossier di revisione. Non vengono inventati demo, test superati, verdetti o aggiornamenti di esecuzione.

## Design

Un solo carattere, Nunito Sans, già usato dal viewer. Logo esistente, fondo chiaro, bordi sottili, un accento indaco per le azioni. I titoli descrivono la vista o l'oggetto. La gerarchia deriva da spaziatura e peso del testo, senza titoli editoriali, slogan o decorazioni.

Su desktop la chat conserva visibile il campo di scrittura. La board usa la larghezza della pagina. Su telefono le colonne scorrono orizzontalmente. Il dettaglio ha un comando esplicito per tornare alla board.

## Riferimenti verificati nel repository

- `cli/internal/web/assets/workspace-layout.js`: Conversazione, Board e dettaglio spec.
- `cli/internal/web/assets/app.js`: apertura card, cambio stato, azioni e avvio dell'esecuzione sul bersaglio.
- `cli/internal/web/handlers.go`: il dettaglio restituisce spec, piano, task e azioni.
- `cli/internal/web/execution_run.go`: disponibilità e condizioni di avvio.
- `cli/internal/template/template.go`: etichette e stati ammessi dal Template software.
- `.archetipo/backlog.yaml`, `.archetipo/specs/`, `.archetipo/plans/`: fonte dei dati dello snapshot.

Le precedenti proposte Dialogo, Studio e Regia sono sostituite da questo unico mockup. I vecchi URL locali portano alle viste aggiornate.

## Avvio dalla conversazione

La schermata iniziale espone quattro attività: «Definisci un prodotto», «Nuova storia», «Nuovo mockup» e «Conversazione libera». Ogni scheda descrive il lavoro e il risultato. Dopo la scelta il campo Messaggio contiene soltanto ciò che scrive la persona; i nomi tecnici non sono inseriti nel testo.

Il menu «Scegli attività» permette di cambiare attività dalla chat. Il collegamento «Apri la board» conserva l'alternativa di lavorare sulle storie esistenti. «Nuova spec» continua ad aprire il modulo manuale.

Le associazioni con le funzionalità esistenti sono interne: definizione del prodotto con la skill di inception; storia con intervista, proposta e conferma usando la skill di spec; mockup con la skill di design. Nel prodotto queste scelte devono essere offerte soltanto quando presenti nel catalogo della sessione e compatibili con le azioni di workspace. Il mockup non legge un catalogo nativo e non determina la disponibilità effettiva.

La prima inception guidata richiede un workspace senza PRD. Per un altro prodotto si apre o crea il suo workspace. Il design produce un mockup isolato, senza implementarlo nell'app. La creazione assistita della storia usa le domande e la conferma già previste dal flusso di estensione del backlog; non aggiunge un questionario rigido nel frontend.

Scegliere un'attività nel mockup prepara soltanto il contesto locale. Non avvia un agente, non genera domande o risposte simulate e non salva artefatti. I corpi delle storie del repository restano invariati: eventuali termini tecnici presenti nei documenti originali non vengono riscritti.

## Altre opzioni di presentazione da valutare

1. **Chat e contenuto affiancati.** Durante il lavoro si apre accanto ai messaggi l'artefatto pertinente: documento prodotto, proposta di storia, piano o mockup. Si riusano i contenuti che il processo produce, senza creare un nuovo oggetto di dominio. La vista su schermi piccoli può alternare messaggi e contenuto mantenendo il contesto.
2. **Creazione assistita anche dalla board.** Il comando per una nuova storia propone «Con intervista» e «Compila direttamente», come due accessi alla creazione assistita e al modulo esistenti. L'utente non deve aprire la chat e conoscere il nome tecnico della skill per iniziare l'intervista.
3. **Esito e prossimo comando nel punto del lavoro.** Quando una proposta è disponibile, il viewer mostra il contenuto e la conferma pertinente; quando esiste un piano, espone l'azione successiva ammessa dal server. Non vengono inventati progressi, durate, domande o risultati prima che il processo li produca.

Queste opzioni sono proposte di UX, non capacità implementate nel mockup. La versione corrente sviluppa il punto di ingresso con attività esplicite e mantiene invariata la board con i dati del repository.
