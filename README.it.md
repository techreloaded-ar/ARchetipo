# ARchetipo

[English](README.md) · Italiano

Uno spazio di lavoro per comprendere un problema, costruire e provare una soluzione,
imparare dall’esperienza e consegnare ai developer una versione verificata.

La chat e l’app sono al centro. Backlog, storie, specifiche, piani facoltativi e
Wiki mantengono la tracciabilità necessaria in azienda, senza imporre un ciclo di
pianificazione, implementazione e review per ogni modifica.

## Provare questo branch

Da questo repository servono Go per compilare e Node/npm per la distribuzione di
sviluppo. Non è necessario un servizio cloud per gli ambienti locali.

```sh
npm run install:dev
cd /percorso/del/progetto
archetipo init --tool codex --connector file
archetipo view
```

La build di sviluppo installa il contenuto di questo branch. Il pacchetto npm
pubblico può contenere una versione precedente. Per la chat occorre configurare
un provider con sessioni native, Codex o Claude, con il proprio accesso al modello.
Gli altri tool supportano l’uso standalone delle skill installate.

Un’app esistente deve essere versionata in Git. Per una nuova app basta un workspace
contenente gli asset di ARchetipo: viene creata una base Git vuota, senza includere
file non versionati. Il lavoro parte da `main`, oppure dal branch corrente
committato se `main` non esiste.

## Esperienza

1. Descrivi il problema in chat, oppure apri una storia dal **Backlog** e scegli
   **Lavora su questa storia**.
2. ARchetipo crea un worktree, un branch e una sessione nativa dedicati. L’agente
   chiarisce quanto serve e mantiene una specifica Markdown evolutiva.
3. Il codice viene sviluppato e verificato nell’ambiente del lavoro. Porte,
   servizi e dati di prova sono gestiti attraverso ricette riutilizzabili.
4. Prova l’app nell’anteprima. Scrivi in chat o seleziona elementi e commentali.
   Le revisioni conservano lo stesso ambiente e la stessa conversazione.
5. **Specifica e decisioni** raccoglie requisiti, versioni, verifiche e osservazioni.
   **Esporta dossier** scarica un documento Markdown consultabile fuori dal prodotto.
6. **Approva risultato e apri PR** consegna la versione provata ai developer.
   I developer possono modificarla, riscriverla, integrarla e portarla in produzione.

ARchetipo non esegue merge o deploy dalla UI. Una conclusione utile può anche non
richiedere codice: osservazione e decisione vengono registrate nel lavoro.

## Gestione e memoria del prodotto

- Le storie restano nel backlog originale, con epiche, priorità, criteri e storico.
  Zero punti indica una storia non stimata.
- I collegamenti storia → lavoro → PR sono navigabili in entrambe le direzioni.
  Ogni revisione conserva la copia dei requisiti originali usati come contesto.
- Specifica, feedback, risultati precedenti, osservazioni e decisioni restano
  consultabili. Il dossier della PR identifica anche il commit approvato.
- La Wiki documenta il progetto; PRD e piani sono strumenti facoltativi.
- Il merge non modifica automaticamente lo stato delle storie. La versione
  approvata dal business resta distinta dalle successive modifiche dei developer.
- La rimozione di un ambiente conserva il dossier e i riferimenti alle storie.
  I commit sono recuperabili tramite un riferimento Git; i dati locali dell’ambiente
  vengono rimossi secondo il contratto di cleanup.

## Architettura e limiti

La UI orchestra e presenta. La CLI gestisce deterministicamente persistenza,
connector, Git e risorse locali. Le skill interpretano richieste e producono contenuti.
Le sessioni sono native e durevoli: cambiare schermata non chiude il lavoro.

I worktree separano codice e dati di lavoro; **non sono sandbox di sicurezza del
sistema operativo**. La base locale non richiede Kubernetes, Docker o servizi a
pagamento. La ricetta del progetto può usare gli strumenti che servono all’app.
La preview viene sospesa quando il turno necessita dell’ambiente e riparte dal
candidato verificato; non è ancora una preview continua durante ogni compilazione.
L’esecuzione remota e l’isolamento di sicurezza richiedono un ulteriore livello runtime.

Questo branch rimuove le skill `archetipo-autopilot`, `archetipo-design`,
`archetipo-implement` e `archetipo-review`, insieme al vecchio percorso degli
incrementi con backlog interno, approvazione dell’intento e review che integra branch
nel viewer. `init` elimina quelle quattro skill gestite durante l’aggiornamento;
conserva le skill personali. I lavori storici restano leggibili: il loro workflow
non viene ricreato. I comandi CLI per gestire gli artefatti aziendali restano disponibili.

Vedi [contratto del lavoro](docs/increments.md) e [istruzioni del repository](AGENTS.md).
