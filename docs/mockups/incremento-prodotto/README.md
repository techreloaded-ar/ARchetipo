# Incremento di prodotto — mockup navigabile

Aprire `index.html` nel browser, anche direttamente dal filesystem. Le risorse sono locali e non sono richieste dipendenze.

Il mockup riprende Nunito Sans e la palette carta, indaco e albicocca del viewer. La schermata principale mette il risultato da provare accanto alla conversazione, mentre intento, storie e cronologia appartengono allo stesso incremento.

## Percorso da provare

1. Nell’anteprima scegliere un’area, cercare un cliente e premere **Esporta Excel**. L’esportazione mostra una conferma simulata e non produce un file.
2. Premere **Aggiungi un dettaglio**, inviare il feedback e scegliere **In entrambi**. Confermare il giro 4: compare anche il fatturato dei clienti.
3. Aprire **Intento**, **Storie** e **Cronologia**: il quarto giro rimane collegato all’incremento e lo stato si conserva durante la navigazione nella scheda.
4. Premere **Approva e apri PR**, confermare la prova e aprire la PR simulata. Il backlog mostra le storie come proposte.
5. Premere **Vedi la PR #128** e **Simula il merge**: l’incremento risulta integrato e le storie consegnate.
6. Il pulsante **+** nella barra laterale apre l’inizio dell’intervista per un nuovo incremento.

La guida **?** permette di ricominciare la demo. Tutti i dati e i controlli tecnici sono dimostrativi; nessun agente, connector, database o servizio Git viene chiamato. Questo è un prototipo visivo, non un’implementazione del processo.

Schermate: `index.html`, `intento.html`, `storie.html`, `cronologia.html`. Il solo stato della demo è mantenuto nel `sessionStorage` della scheda, quando disponibile.
