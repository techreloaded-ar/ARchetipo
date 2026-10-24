# Lavoro sul prodotto

Il termine tecnico `increment` identifica un lavoro durevole sul prodotto. Il suo
risultato può essere una soluzione funzionante da consegnare ai developer oppure
una decisione motivata che non richiede altro codice.

## Contratto

Un lavoro conserva richiesta originale, specifica Markdown versionata, riferimenti
alle storie originali, feedback, osservazioni, decisioni e risultati delle verifiche.
Ogni risultato è legato a un commit pulito e alla specifica provata. Una modifica
esplicita revoca il candidato corrente, conservando prove e versione precedenti.
Una domanda o un’osservazione non riscrivono i requisiti e non approvano codice.

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
nel lavoro si torna alle storie originali.

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

La preview si apre automaticamente quando il candidato è verificato e il turno è
terminato. Ferma resta esplicito anche dopo reload. Durante un nuovo turno può
essere sospesa per permettere l’uso esclusivo della ricetta; servizi e dati vengono
conservati. Non è ancora una preview continua durante ogni modifica. Il worktree
non è una sandbox di sicurezza: esecuzione remota e isolamento OS sono livelli
runtime ulteriori, non capacità già offerte da questa versione.

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
