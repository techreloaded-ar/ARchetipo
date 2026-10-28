# Ambienti e servizi locali

ARchetipo gestisce i servizi dichiarati dal progetto, senza riconoscere framework
né richiedere Docker. La CLI possiede registro, porte, serializzazione ed evidenze;
le skill scelgono i comandi del progetto e la view usa lo stesso contratto.

## Ricetta del progetto

La ricetta versionata `.archetipo/environment.json` versione 1 accetta i campi
opzionali `ports` e `resources`. Le ricette precedenti restano valide; gli avvii
nascosti nei vecchi script non vengono automaticamente trasformati in risorse.

Esempio di adattatore del progetto (lo script `tools/test-service.py` deve essere
implementato dal progetto, usando i propri strumenti):

```json
{
  "version": 1,
  "inputs": ["requirements.txt", "tools/test-service.py"],
  "outputs": [],
  "ports": ["DATABASE", "API"],
  "resources": [
    {
      "name": "database",
      "start": {"name": "db-start", "argv": ["python3", "tools/test-service.py", "start"], "timeout_seconds": 30},
      "probe": {"name": "db-ready", "argv": ["python3", "tools/test-service.py", "probe"], "timeout_seconds": 10},
      "stop": {"name": "db-stop", "argv": ["python3", "tools/test-service.py", "stop"], "timeout_seconds": 30},
      "stopped": {"name": "db-stopped", "argv": ["python3", "tools/test-service.py", "stopped"], "timeout_seconds": 10}
    }
  ],
  "prepare": [],
  "probe": [{"name": "project-ready", "argv": ["python3", "tools/test-service.py", "probe"], "timeout_seconds": 10}],
  "checks": {}
}
```

Gli adattatori sono comandi finiti: avviano e identificano la risorsa esterna con
il relativo strumento, attendono readiness o arresto entro il timeout e restituiscono
zero soltanto dopo aver verificato l'effetto. Un servizio può essere un processo
locale, un container o un insieme di risorse gestito da uno strumento del progetto.
L'applicazione dell'anteprima resta invece un comando in foreground supervisionato.

Ogni comando riceve:

- `ARCHETIPO_ENVIRONMENT_ID`, `ARCHETIPO_ENVIRONMENT_ROOT`,
  `ARCHETIPO_ENVIRONMENT_STATE`: identità, checkout e directory runtime.
- `ARCHETIPO_RESOURCE_OWNER`: token casuale della generazione di risorse.
- `ARCHETIPO_RESOURCE_NAME`: nome della risorsa durante le sue quattro operazioni.
- `ARCHETIPO_PORT_DATABASE`, `ARCHETIPO_PORT_API`: porte locali assegnate.

Usare proprietario e nome per identificare le risorse, verificandoli prima di
riusarle o fermarle. Per un container registrare e verificare ID/label; per un
processo usare un'identità verificabile dal servizio, non un PID potenzialmente
riciclato. `start` e `stop` devono essere idempotenti; `stop` deve funzionare anche
se `start` è fallito a metà. `stopped` verifica che il servizio posseduto sia davvero
terminato. Fermare non deve cancellare dati persistenti o volumi.

L'ordine dell'array è quello delle dipendenze. ARchetipo registra ogni tentativo
prima dell'avvio, verifica readiness, compensa un avvio/preparazione falliti e
ferma in ordine inverso tutte le risorse registrate, anche se uno stop fallisce.
Conserva i fallimenti, i log completi e gli adattatori originali per poter riprovare
anche dopo modifiche o rimozione della ricetta. Nessun segreto deve essere scritto
in argv, ricetta o registro.

## Porte e incrementi concorrenti

Ogni checkout ha proprietario, registro e porte distinti. Non versionare né copiare
`.archetipo/environment/`. Per l'applicazione usare `{port}` nell'URL e negli
argomenti di `.archetipo/preview.json`, con `prepare_environment: true`.
Le anteprime con risorse gestite richiedono questo contratto dinamico.
Gli URL derivati (API, database, callback di autenticazione) devono essere costruiti
dai valori assegnati, non da porte scritte nel codice o ereditate da un altro worktree.

L'assegnazione non può trasferire atomicamente un socket a qualunque strumento:
tra scelta e bind un programma esterno potrebbe occupare la porta. Ogni adattatore
deve quindi usare il bind rigoroso e verificare la propria identità nella readiness.
In caso di collisione l'avvio fallisce e viene compensato; un nuovo tentativo
assegna nuove porte. Non riutilizzare il server sconosciuto che risponde alla porta.

Due incrementi possono girare insieme; due anteprime che vogliono possedere i
servizi dello stesso checkout ricevono invece un conflitto. Uno stop esplicito
può raggiungere il supervisore dello stesso checkout anche da un altro terminale
o viewer, attraverso una richiesta indirizzata all'identità della singola esecuzione.

## Uso e arresto

- `archetipo execution environment configure --file recipe.json`: salva la ricetta.
- `archetipo execution environment prepare`: prepara e conserva i servizi registrati
  per il riuso fra task. Anche `execution preview prepare` conserva l'ambiente;
  non avvia l'applicazione dell'anteprima.
- `archetipo execution environment status`: legge il registro, gli stati osservati
  e le porte (non sostituisce una nuova readiness).
- `archetipo execution environment run`: prepara e mantiene l'ambiente finché il
  terminale resta attivo. Ctrl+C ferma e verifica i servizi.
- `archetipo execution environment cleanup`: ferma le risorse registrate e invalida
  le verifiche. Rifiuta l'operazione se una preview/run ne possiede ancora la vita. Anche una
  prepare/check esterna riceve un conflitto finché quel proprietario è attivo.
- `archetipo execution preview run`: prepara l'ambiente e supervisiona applicazione
  e dipendenze finché il terminale resta attivo.
- `archetipo execution preview status`: restituisce `kind: preview`, con l'ultimo
  endpoint applicativo, l'esito dello stop e la presenza del supervisore proprietario.
- `archetipo execution preview stop`: richiede lo stop al supervisore registrato
  di questo checkout e attende il rilascio e il cleanup; riconcilia poi i registri.
  Un timeout conserva la richiesta e restituisce `E_CONFLICT`, senza dichiarare
  il server fermo. Se il proprietario è scomparso ma la porta resta occupata,
  conserva l'errore e richiede intervento: non termina un listener sconosciuto.

Su macOS e Linux il comando in primo piano possiede un gruppo di processi:
riceve SIGTERM, poi SIGKILL dopo una breve attesa se necessario. Anche l'uscita
anticipata del launcher ferma i figli rimasti. I wrapper devono restare in primo
piano, senza `nohup`, detach o nuove sessioni. Su Windows resta il percorso
di cancellazione tramite `taskkill /T`; il recupero dei figli di un launcher già
terminato non è garantito da questa modifica.

Il registro `.archetipo/environment/preview-process.json` conserva endpoint,
identità dell'esecuzione ed esito verificato dell'arresto. La verifica usa una
connessione TCP, non il successo di una risposta HTTP. `preview-stop.json`
indirizza le richieste alla sola esecuzione indicata: una richiesta vecchia non
ferma quella successiva. Non sono registri di PID da terminare al riavvio.
La rimozione di un incremento attende anche il supervisore applicativo e rifiuta
di rimuovere il worktree se l'ultimo arresto risulta ancora da verificare.

Il test opzionale `npm run test:next-preview-process-smoke -- --node-modules
/percorso/assoluto/node_modules` usa una copia di Next già installata, senza
installare dipendenze o modificare quel progetto. Crea una piccola app separata
e prova la catena npm → wrapper → Next con stop normale, stop da un altro
terminale, morte forzata del proprietario e uscita anticipata del wrapper.
Report e log restano sotto `test/workspaces/next-preview-process-smoke/`.

I comandi environment usano `archetipo/v1`, `kind: verification_result`.
`status` restituisce il registro; `run` annuncia il risultato di preparazione e
attende l'interruzione. Per prepare/check/cleanup verificare `data.passed`, non
soltanto `ok`; per l'accettazione delle verifiche richiedere anche `data.stable`.
Errori di contratto sono `E_INVALID_INPUT`, ownership concorrente `E_CONFLICT`,
ricetta/registro mancanti o corrotti `E_PRECONDITION`.

Nella view **Ferma ambiente** attende l'arresto dell'applicazione e dei servizi;
il dettaglio Ambiente elenca servizi, porte e gestione, distinguendo i server
esterni dagli ultimi stati registrati delle dipendenze. Output mostra i log.
Un fallimento di cleanup resta visibile e
riprovabile. Il pulsante funziona anche dopo un riavvio, leggendo il registro.
Un supervisore separato riceve la chiusura del canale privato anche se il viewer
viene terminato con SIGKILL ed esegue il cleanup. Lo stesso vale per il run CLI.
La preparazione esplicita, invece, conserva intenzionalmente i servizi; se viene
interrotta bruscamente, il registro dei tentativi permette la riconciliazione alla
prossima preparazione o cleanup. Spegnimento della macchina o morte del supervisore
stesso richiedono ugualmente questa riconciliazione: non si promette l'esecuzione
di cleanup mentre nessun processo può eseguirlo.

Avviare o riprendere lo sviluppo dalla view sospende prima l’applicazione e il
proxy della preview che lo stesso viewer possiede per quel checkout, poi prepara
l’ambiente con il contratto CLI. Il supervisore riceve un rilascio esplicito:
servizi registrati, porte e dati dimostrativi rimangono disponibili e vengono
verificati prima del riuso. Lo stesso passaggio avviene per le richieste di
modifica inviate dallo strumento di commento della preview di un incremento.
Questo evita che una preview aperta durante la definizione dell’intento blocchi
lo sviluppo con un conflitto di ownership o che una revisione ricrei un database
di prova vuoto. Ferma ambiente, rimozione e perdita improvvisa del proprietario
di una preview attiva continuano a eseguire il cleanup. Preview di altri checkout e ambienti
posseduti da altri viewer o terminali restano protetti dai rispettivi lock.

Chiudere una scheda, interrompere un turno agente e cancellare un ambiente restano
operazioni distinte. La rimozione dell'incremento verifica i lock e il cleanup anche
quando l'avvio non ha ancora scritto un checkpoint di preparazione.

## Migrazione e RideAtlas

Aggiornare il binario e le skill installate. Spostare gli avvii dei servizi dagli
script preview alle risorse dichiarate, sostituire le porte fisse con i valori
runtime e verificare gli adattatori di stop. Le ricette e gli helper devono arrivare
in `main` prima di creare nuovi incrementi che li ereditino.

Cancellare `.archetipo` non ferma processi o container e perde il registro dei
proprietari; non è una procedura di arresto o migrazione. Prima riconciliare i vecchi
servizi identificati con certezza e conservare i dati, poi aggiornare le ricette.
La nuova gestione non adotta automaticamente container precedenti privi di owner.

## Verifica

`npm run test:view-preview-smoke` usa due incrementi reali e una dipendenza HTTP
esterna reale, senza Docker o credenziali. Verifica porte distinte per applicazione
e dipendenza, stop isolato, riuso della preparazione, stop dalla view e cleanup di
applicazione e dipendenza dopo SIGKILL del viewer. Le unit test verificano rollback,
ordine inverso, stop fallito e ripresa, cambio ricetta, invalidazione delle verifiche
e recupero usando gli adattatori salvati dopo la scomparsa della ricetta. Lo smoke
verifica inoltre un arresto fallito e riprovato e il run standalone con SIGKILL.
