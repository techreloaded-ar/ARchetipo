# Checklist import/setup - ARchetipo Inception Copilot Studio

## Preparazione

- [ ] Hai accesso all'ambiente Microsoft Copilot Studio corretto.
- [ ] Hai permessi per creare o modificare agenti.
- [ ] Hai accesso alla cartella OneDrive personale dedicata ai PRD ARchetipo.
- [ ] Hai identificato `ARCHETIPO_ONEDRIVE_FOLDER_PATH`.
- [ ] Hai scelto `ARCHETIPO_PRD_FILE_PREFIX` o accetti il default `PRD`.
- [ ] Hai scelto `ARCHETIPO_MAX_REFERENCE_PRDS` o accetti il default `5`.

## Agente

- [ ] Crea un agente chiamato `ARchetipo Inception`.
- [ ] Incolla `agent-instructions.md` nelle istruzioni dell'agente.
- [ ] Configura le variabili OneDrive dell'agente o della solution.
- [ ] Crea il topic principale seguendo `topic-inception.md`.
- [ ] Aggiungi trigger phrase per PRD, discovery prodotto, MVP e idea prodotto.

## Power Automate

- [ ] Crea il flow `ListReferencePrdsFromOneDrive`.
- [ ] Configura il trigger richiamabile da Copilot Studio.
- [ ] Definisci input: `folderPath`, `maxFiles`, `query`.
- [ ] Implementa lettura elenco file dalla cartella OneDrive.
- [ ] Implementa lettura contenuto Markdown dei PRD selezionati.
- [ ] Restituisci output: `ok`, `items`, `errorCode`, `message`.
- [ ] Crea il flow `SavePrdToOneDrive`.
- [ ] Configura il trigger richiamabile da Copilot Studio.
- [ ] Definisci input: `folderPath`, `filePrefix`, `productName`, `contentMarkdown`.
- [ ] Implementa generazione nome file univoco.
- [ ] Implementa creazione file senza overwrite.
- [ ] Restituisci output: `ok`, `webUrl`, `filePath`, `fileName`, `errorCode`, `message`.
- [ ] Aggiungi entrambi i flow all'agente come tool/action.

## Test

- [ ] Avvia una conversazione completa con una nuova idea prodotto.
- [ ] Verifica che l'agente presenti il team ARchetipo.
- [ ] Verifica che provi a leggere i PRD storici dalla cartella OneDrive.
- [ ] Verifica che segnali eventuali progetti simili senza copiare decisioni passate in modo automatico.
- [ ] Verifica che usi almeno due tecniche di challenge durante la discovery.
- [ ] Verifica che non generi il PRD prima dei criteri minimi.
- [ ] Verifica che il PRD abbia almeno 10 requisiti funzionali.
- [ ] Verifica che il PRD venga salvato in OneDrive.
- [ ] Verifica che il link OneDrive, il path e il nome file vengano restituiti in chat.
- [ ] Verifica che due inception consecutive producano due nomi file diversi.
- [ ] Verifica il caso di cartella OneDrive vuota.
- [ ] Verifica il caso di path OneDrive non valido.

## Pubblicazione

- [ ] Pubblica l'agente nel canale desiderato.
- [ ] Se serve trasportarlo tra ambienti, aggiungilo a una Power Platform Solution.
- [ ] Esporta la solution dall'ambiente sorgente.
- [ ] Importa la solution nell'ambiente target.
- [ ] Ricontrolla variabili OneDrive e connessioni Power Automate nell'ambiente target.

## Controllo finale

- [ ] Nessuna istruzione dell'agente richiede la CLI `archetipo`.
- [ ] Nessuna istruzione richiede accesso al file system locale.
- [ ] La cartella OneDrive e configurabile senza modificare le istruzioni agente.
- [ ] L'agente legge i PRD esistenti prima della discovery sostanziale.
- [ ] Ogni nuovo PRD viene salvato con un nome univoco.
- [ ] In caso di errore di salvataggio, l'agente conserva il PRD nella chat.
