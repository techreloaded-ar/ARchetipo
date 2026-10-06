# Apertura Contratto Cassette di Sicurezza - Documento dei Requisiti di Prodotto

**Autore:** ARchetipo  
**Data:** 05/10/2026  
**Versione:** 1.0

---

## Elevator Pitch

> Un processo digitale guidato in FrEE che consente al gestore di filiale di aprire un contratto di cassetta di sicurezza in modo semplice, controllato e tracciabile, riutilizzando i dati già disponibili e coordinando in un unico percorso deleghe, condizioni economiche, adeguata verifica, documentazione e firma.
>
> Per **i gestori delle Filiali Crédit Agricole Italia che collocano il servizio a clienti maggiorenni titolari di conto corrente**, che hanno il problema di **operare su procedure e documenti separati, con inserimenti ripetuti e rischi di errore o incompletezza**, **Apertura Contratto Cassette di Sicurezza** è un **processo bancario orchestrato su FrEE** che **guida l’apertura end-to-end e rende espliciti controlli, dipendenze ed esiti**. A differenza del **flusso 3270/B000 con compilazione manuale dei documenti in DynDoc**, il prodotto **precompila i dati disponibili, integra i processi specialistici e concentra la firma in un solo passaggio**.

---

## Visione

Trasformare l’apertura del contratto di cassetta di sicurezza da una sequenza frammentata di attività operative in un’esperienza unica, coerente e verificabile su FrEE, riducendo rilavorazioni e rischi di non conformità senza alterare le regole sostanziali del servizio.

Il processo parte dalla Scheda Cliente FrEE e comprende inserimento dati, eventuale gestione dei delegati, condizioni economiche, questionario KYC, finalizzazione e documentale con firma unica. I passaggi sono coordinati dall’orchestratore FrEE; gli step applicativi nuovi da requisire sono principalmente inserimento dati, condizioni economiche e finalizzazione.[doc:turn1doc1]

### Differenziatore del Prodotto

- Un solo percorso guidato, con stato della pratica sempre visibile.
- Precompilazione dei dati del cliente e dei cointestatari dalla Scheda Cliente FrEE.
- Recupero e verifica della disponibilità della cassetta prima della stipula.
- Calcolo automatico e trasparente delle condizioni economiche.
- Gestione delle deroghe esclusivamente tramite DOL, senza scontistica manuale nel nuovo processo.
- Integrazione nativa con gestione delegati, KYC, documentale e firma unica.
- Controlli bloccanti applicati prima della stipula, non dopo.
- Riepilogo finale leggibile sia dal gestore sia dal cliente prima della firma.

---

## Personas Utente

### Persona 1: Giulia, Gestore di Filiale

**Ruolo:** Gestore retail responsabile della vendita e dell’apertura del contratto  
**Età:** 38 anni | **Contesto:** Lavora in una filiale dotata di impianto cassette; gestisce relazioni commerciali, adeguata verifica e operatività amministrativa.

**Obiettivi:**
- Verificare rapidamente che il cliente e la cassetta siano idonei.
- Completare l’apertura senza passaggi ridondanti tra applicazioni.
- Presentare al cliente condizioni e coperture in modo comprensibile.
- Evitare errori che impediscano la stipula o richiedano rilavorazioni.
- Disporre di una prova chiara delle attività completate.

**Punti Dolenti:**
- Dati distribuiti tra FrEE, 3270/B000, GIANOS 3D, ZC00/DOL e DynDoc.
- Inserimenti manuali ripetuti e uso di codici operativi.
- Difficoltà nel capire quale controllo o sotto-processo blocchi la pratica.
- Rischio di arrivare alla firma con dati o documenti incompleti.

**Comportamenti e Strumenti:**
- Parte dalla Scheda Cliente FrEE.
- Consulta l’impianto della filiale e le condizioni applicabili.
- Coinvolge il cliente per intestazione, operatività, deleghe, assicurazione e firma.
- Sospende una pratica quando un sotto-processo non può essere completato subito.

**Motivazioni:** Servire il cliente in un unico appuntamento, operando correttamente al primo tentativo.  
**Competenza Tecnologica:** Media-alta sui sistemi bancari, ma condizionata dalla complessità e dalla frammentazione delle procedure.

#### Customer Journey - Giulia, Gestore di Filiale

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Consapevolezza | Raccoglie l’esigenza del cliente e verifica che la filiale offra il servizio | “Posso proporgli una cassetta adatta e disponibile?” | Attenta | Rendere visibili subito disponibilità e prerequisiti |
| Valutazione | Verifica target market, conto di addebito, soggetti e bisogno assicurativo | “Ho tutti gli elementi per iniziare senza rischiare blocchi più avanti?” | Prudente | Eseguire controlli preliminari e spiegare i blocchi |
| Primo Utilizzo | Avvia il processo dalla Scheda Cliente e completa i dati | “Il sistema deve riusare ciò che già conosce” | Fiduciosa | Precompilazione, validazioni contestuali e salvataggio |
| Uso Ricorrente | Gestisce deleghe, KYC, condizioni, riepilogo e firma | “Voglio capire stato, dipendenze e prossima azione” | Concentrata | Timeline, sotto-processi ed esiti uniformi |
| Promozione | Riutilizza il processo e lo suggerisce ai colleghi | “È più sicuro e rapido del vecchio percorso” | Soddisfatta | Evidenze di qualità, tracciabilità e riduzione rilavorazioni |

---

### Persona 2: Carlo, Cliente

**Ruolo:** Cliente maggiorenne, titolare o cointestatario della cassetta  
**Età:** 62 anni | **Contesto:** Cliente retail con conto corrente presso la Banca; desidera custodire valori e documenti con riservatezza e copertura assicurativa.

**Obiettivi:**
- Comprendere costi, durata, modalità operative e copertura assicurativa.
- Scegliere una cassetta adatta alle proprie esigenze.
- Eventualmente cointestare il rapporto o nominare delegati.
- Sottoscrivere i documenti senza passaggi inutili.

**Punti Dolenti:**
- Linguaggio tecnico e condizioni economiche difficili da confrontare.
- Timore di costi non previsti o copertura insufficiente.
- Tempi di attesa dovuti a controlli o applicazioni separate.
- Possibili difficoltà visive, motorie o di comprensione del processo digitale.

**Comportamenti e Strumenti:**
- Si affida al gestore e richiede spiegazioni sui costi.
- Può preferire firma cartacea oppure digitale, se disponibile.
- Può coinvolgere familiari come cointestatari o delegati.

**Motivazioni:** Sicurezza, riservatezza, chiarezza contrattuale e fiducia nella Banca.  
**Competenza Tecnologica:** Variabile; il processo non deve presupporre autonomia digitale del cliente.

#### Customer Journey - Carlo, Cliente

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Consapevolezza | Esprime il bisogno di proteggere beni o documenti | “È il servizio giusto per ciò che voglio custodire?” | Interessato | Spiegazione chiara di benefici, limiti e alternative |
| Valutazione | Confronta dimensione, costo, intestazione e assicurazione | “Quanto pago e fino a quale valore sono coperto?” | Cauto | Riepilogo trasparente, inclusa copertura base di € 52.000,00[doc:turn1doc7] |
| Primo Utilizzo | Fornisce informazioni e partecipa ai controlli KYC | “Perché servono questi dati?” | Impaziente | Linguaggio semplice e visibilità dell’avanzamento |
| Uso Ricorrente | Accede alla cassetta secondo le regole contrattuali | “Il rapporto è stato configurato correttamente?” | Rassicurato | Documentazione coerente e dati verificabili |
| Promozione | Valuta l’esperienza complessiva | “La banca ha gestito tutto senza errori?” | Soddisfatto | Processo breve, inclusivo e affidabile |

---

## Risultati del Brainstorming

> Principali scoperte e direzioni alternative esplorate durante l’inception.

### Assunzioni Sfidate

1. **Assumption challenging — “La digitalizzazione coincide con la trasposizione della mappa 3270.”** Se fosse falso, il valore non starebbe nel replicare tutti i campi, ma nel chiedere al gestore solo quelli necessari e derivare gli altri. Direzione scelta: mantenere automatismi e regole del 3270, eliminando dal nuovo percorso i campi non necessari all’apertura.
2. **Anti-problem — “Come rendere il processo inutilizzabile?”** Separare le attività in molte applicazioni senza stato condiviso, consentire la firma prima dei controlli, nascondere i motivi dei blocchi, perdere i dati tornando indietro, lasciare modificare manualmente condizioni calcolate. Contromisure: orchestrazione, punti di consistenza, messaggi azionabili, persistenza della bozza e condizioni governate.
3. **What if — “E se KYC, deleghe o deroga non terminassero nello stesso appuntamento?”** Il processo deve poter essere sospeso e ripreso senza perdere dati, mantenendo visibili i sotto-processi e impedendo la finalizzazione finché gli esiti obbligatori non sono positivi.
4. **Audience flip — “E se il gestore fosse neoassunto o il cliente avesse bassa familiarità digitale?”** Il percorso deve spiegare termini, conseguenze e prossima azione senza affidarsi alla conoscenza dei codici 3270. La firma deve supportare le modalità consentite dal Documentale e non creare barriere per il cliente.

### Nuove Direzioni Individuate

- Separare nettamente dati contrattuali, condizioni economiche e conferma finale.
- Rendere la ricerca e prenotazione logica della cassetta parte del percorso, con ricontrollo alla finalizzazione.
- Usare una checklist di stipulabilità sempre visibile: cliente idoneo, conto valido, cassetta disponibile, soggetti completi, KYC valido, condizioni definitive, documenti pronti.
- Mostrare l’impatto economico di ogni scelta: cointestatari, delegati, tipologia/dimensione, copertura integrativa e spese di apertura.
- Prevedere un percorso lineare per il caso standard e sotto-processi solo quando necessari.

### Assunzioni da Validare

- L’MVP riguarda esclusivamente le cassette di sicurezza; i colli chiusi sono fuori scope.
- L’utente primario è il gestore di una filiale CAI abilitata e titolare dell’impianto.
- Il cliente deve essere maggiorenne, residente o non residente in Italia, titolare di un conto corrente CAI e coerente con il target market; la clientela non al dettaglio è esclusa.[doc:turn1doc2]
- Il conto di addebito può avere intestazione diversa dalla cassetta, ma deve appartenere ad almeno uno degli intestatari.[doc:turn1doc2]
- Le eccezioni che utilizzano il conto interno 111111 non rientrano nel percorso ordinario MVP.
- Il questionario KYC deve essere completato per ogni intestatario prima della stipula; quello specifico per l’apertura ha validità di 15 giorni di calendario.[doc:turn1doc2]
- La disponibilità fisica della cassetta può essere interrogata in tempo reale e riservata logicamente per la durata della pratica.
- Le condizioni definitive e gli esiti dei sotto-processi possono essere scambiati con l’orchestratore mediante contratti di integrazione disponibili.
- **Domande aperte:** durata massima della riserva logica; regole per persone giuridiche e rappresentanti legali; modalità di firma abilitate tra cartacea, OTP e VAD; SLA e stati esatti della deroga DOL; catalogo completo dei messaggi e mapping degli esiti tecnici.

### Rischi Chiave

- **Adozione:** ritorno alle procedure legacy se il nuovo flusso è più lento o non copre le eccezioni.
- **Qualità dati:** precompilazioni anagrafiche obsolete o incoerenti.
- **Concorrenza operativa:** cassetta non più disponibile al momento della stipula.
- **Integrazione:** indisponibilità o esiti asincroni di deleghe, KYC, DOL o Documentale.
- **Conformità:** controlli KYC, POG o conto di addebito incompleti.
- **Trasparenza:** condizioni economiche non aggiornate o non spiegate.
- **Accessibilità:** interfaccia densa, basata solo sul colore o con testi tecnici.
- **Migrazione:** differenze tra dati mostrati in FrEE e dati registrati nel sistema contratti.

---

## Ambito del Prodotto

### MVP - Prodotto Minimo Funzionante

- Avvio dalla Scheda Cliente FrEE e controlli preliminari su operatore, cliente, target market, conto e disponibilità del servizio.
- Creazione, salvataggio, sospensione, ripresa e annullo della pratica.
- Selezione di una cassetta libera e disponibile nell’impianto della filiale.
- Inserimento/precompilazione dei dati della cassetta, del contratto, degli intestatari e dell’eventuale assicurazione integrativa.
- Gestione di monointestazione/cointestazione e operatività congiunta/disgiunta.
- Avvio opzionale del sotto-processo delegati già presente in FrEE.
- Calcolo e visualizzazione delle condizioni economiche standard.
- Avvio opzionale della deroga in DOL e ricezione del relativo esito.
- Avvio o recupero del questionario KYC per ogni intestatario e verifica della validità.
- Riepilogo finale e controllo di stipulabilità.
- Produzione automatica della documentazione contrattuale pertinente.
- Selezione dei firmatari e firma unica tramite Documentale.
- Registrazione definitiva del contratto solo a firma completata.
- Esito complessivo e tracciatura.
- Accessibilità secondo FREE UX Guidelines e requisiti adottati dalla Banca.

### Funzionalità di Crescita (Post-MVP)

- Modifica contratto, modifica/revoca delegati ed estinzione.
- Supporto al processo “Colli chiusi”.
- Prenotazione cliente della cassetta e gestione appuntamento.
- Dashboard su pratiche sospese, tempi, errori e conversione.
- Gestione guidata delle eccezioni sul conto interno 111111.
- Notifiche per KYC o deroga in scadenza.
- Firma remota, se consentita dalle policy.
- Confronto guidato tra cassette disponibili per volume e costo.

### Visione (Futuro)

- Piattaforma unica per l’intero ciclo di vita delle cassette: apertura, variazioni, accessi, blocchi/sblocchi, canoni, assicurazione ed estinzione.
- Esperienza omnicanale con preparazione della richiesta da parte del cliente e finalizzazione assistita in filiale.
- Automazione dei controlli documentali e di coerenza, mantenendo decisione e responsabilità nei ruoli autorizzati.
- Insight di rete su disponibilità degli impianti e domanda territoriale, senza esporre informazioni riservate.

---
## Requisiti Funzionali

### A. Avvio, controlli preliminari e gestione pratica

**FR-01 — Avvio contestuale.** Il sistema deve consentire al gestore abilitato di avviare il processo dal menu laterale della Scheda Cliente FrEE, ereditando il cliente selezionato.[doc:turn1doc1]

**FR-02 — Autorizzazione operatore.** Prima dell’avvio, il sistema deve verificare che il gestore sia autorizzato a operare per la filiale e per il servizio; in caso negativo deve impedire l’avvio e mostrare il motivo e l’azione correttiva.

**FR-03 — Verifica cliente e target market.** Il sistema deve verificare almeno: maggiore età, anagrafica valida, appartenenza al target market ammesso e disponibilità di almeno un conto corrente utilizzabile. Il servizio è previsto per clienti CAI maggiorenni, residenti e non residenti in Italia; la verifica del target market è richiesta in sede di vendita.[doc:turn1doc2]

**FR-04 — Creazione pratica.** Il sistema deve creare una pratica con identificativo univoco, data/ora, filiale, gestore, cliente di partenza, stato corrente e storico degli eventi.

**FR-05 — Sospensione, ripresa e annullo.** Il gestore deve poter salvare e sospendere il processo, riprenderlo dalla gestione processi e annullarlo con motivazione. L’annullo deve chiudere anche i sotto-processi collegati ove previsto.

**FR-06 — Timeline.** Il sistema deve mostrare gli step e i sotto-step, distinguendo quelli da iniziare, in corso, completati, completati automaticamente e bloccati; gli step conclusi devono essere consultabili in sola lettura.

### B. Selezione cassetta e dati del contratto

**FR-07 — Ricerca disponibilità.** Il sistema deve mostrare le cassette della filiale filtrabili almeno per fila, codice/numero, classe, tipologia o dimensione e stato di disponibilità.

**FR-08 — Selezione valida.** In apertura deve essere selezionabile solo una cassetta libera, non associata a un contratto e non occupata dalla filiale; filiale e classe devono essere valorizzate automaticamente.[doc:turn1doc3]

**FR-09 — Gestione concorrenza.** Dopo la selezione, il sistema deve riservare logicamente la cassetta e verificarne nuovamente la disponibilità prima della stipula; in caso di conflitto deve impedire la prosecuzione e richiedere una nuova selezione.

**FR-10 — Precompilazione.** Il sistema deve precompilare i dati disponibili della cassetta, del contratto, del cliente e degli eventuali cointestatari, mantenendo gli automatismi pertinenti della procedura B000.[doc:turn1doc1][doc:turn1doc3]

**FR-11 — Conto di addebito.** Il gestore deve selezionare un conto valido. Il conto può essere diversamente intestato rispetto alla cassetta, ma deve appartenere ad almeno uno degli intestatari; le eccezioni sul conto 111111 devono essere escluse dal flusso ordinario e indirizzate al percorso autorizzativo previsto.[doc:turn1doc2]

**FR-12 — Dati economici derivati.** Diritto di apertura, bollo ove dovuto, canone base, diritti per cointestatari/delegati e totale devono essere calcolati dal sistema e non inseriti liberamente dal gestore.

**FR-13 — Dati assicurativi.** Il sistema deve permettere di indicare l’eventuale massimale aggiuntivo rispetto alla copertura base, il conto di addebito del premio, la decorrenza e se la copertura è a revoca o fino a una data. Se non è a revoca, la data di scadenza deve essere obbligatoria; premio e scadenza devono essere calcolati automaticamente.[doc:turn1doc3]

### C. Intestatari, cointestatari e delegati

**FR-14 — Intestatari.** Il sistema deve presentare gli intestatari recuperati dalla Scheda Cliente e consentire monointestazione o cointestazione secondo le regole del prodotto.

**FR-15 — Operatività.** Il sistema deve acquisire il potere di firma/operatività congiunta o disgiunta e applicare i controlli coerenti alla composizione del rapporto.

**FR-16 — Delegati opzionali.** Il sistema deve offrire l’azione “Aggiungi delegati”. Se selezionata, deve avviare il sotto-processo FrEE di gestione delegati; altrimenti deve portare direttamente alle condizioni economiche.[doc:turn1doc1]

**FR-17 — Esito deleghe.** Il processo principale deve ricevere dal sotto-processo almeno identificativo, soggetti, tipologia di delega, operatività, stato ed esito; non deve duplicare l’inserimento dei dati già gestiti dal sotto-processo.

### D. Condizioni economiche e deroghe

**FR-18 — Condizioni standard.** Il sistema deve recuperare e mostrare tutte le condizioni applicate automaticamente in base a intestatario, tipologia/dimensione della cassetta, gruppo condizioni e numero di cointestatari/delegati, incluse le spese di apertura.[doc:turn1doc1]

**FR-19 — Trasparenza economica.** Il sistema deve mostrare ogni voce, periodicità, base di calcolo, eventuale agevolazione, imposte applicabili e totale, distinguendo condizioni standard e derogate.

**FR-20 — Deroga DOL.** Il gestore deve poter avviare una richiesta di deroga in DOL. Il nuovo processo non deve consentire la modifica manuale della percentuale di sconto precedentemente gestita in B000.[doc:turn1doc1]

**FR-21 — Stato deroga.** Il processo deve gestire almeno gli stati non richiesta, in lavorazione, approvata, rifiutata, annullata ed errore tecnico; le condizioni definitive devono essere aggiornate solo dopo un esito valido.

### E. Adeguata verifica KYC

**FR-22 — KYC per intestatario.** Il sistema deve verificare o avviare il questionario KYC per ogni intestatario. L’assenza del questionario deve impedire la stipula.[doc:turn1doc2]

**FR-23 — Validità KYC.** Il sistema deve controllare che il questionario specifico per l’apertura sia valido alla data di stipula; la validità prevista è di 15 giorni di calendario.[doc:turn1doc2]

**FR-24 — Aggiornamento automatico.** Dopo la stipula, il sistema contratti deve associare automaticamente al questionario KYC il numero del rapporto, senza richiedere un nuovo accesso del gestore alla procedura specialistica.[doc:turn1doc2]

### F. Finalizzazione, documenti e firma

**FR-25 — Checklist di stipulabilità.** Prima della finalizzazione, il sistema deve verificare: dati obbligatori completi, cassetta ancora disponibile, conto valido, soggetti coerenti, KYC valido per tutti gli intestatari, condizioni definitive e sotto-processi obbligatori conclusi.

**FR-26 — Riepilogo finale.** Il sistema deve presentare in sola lettura dati della cassetta, intestatari, delegati, operatività, conto di addebito, assicurazione e condizioni economiche definitive, consentendo al gestore di tornare agli step modificabili prima della firma.[doc:turn1doc1]

**FR-27 — Gestione delle modifiche.** La modifica di uno step precedente deve invalidare e ricalcolare gli step successivi interessati, incluse condizioni, KYC, documenti o firma quando dipendenti dal dato modificato.

**FR-28 — Generazione documenti.** Il sistema deve produrre automaticamente la documentazione contrattuale applicabile, valorizzandola con i dati confermati e conservando la versione utilizzata.

**FR-29 — Firma unica.** Il sistema deve avviare il Documentale e raggruppare in un unico iter la sottoscrizione dei documenti del processo principale e degli eventuali sotto-processi collegati.[doc:turn1doc1][doc:turn1doc10]

**FR-30 — Gestione firmatari.** Il Documentale deve consentire la selezione dei firmatari ammessi e applicare una modalità di firma coerente al processo principale e ai sotto-processi, secondo le abilitazioni effettive.

**FR-31 — Esiti firma.** In caso di firma completata il processo deve procedere alla stipula; in caso di annullo, rifiuto, scadenza o errore deve ritornare al punto precedente alla firma, mantenere la tracciatura e indicare la prossima azione.

**FR-32 — Stipula atomica.** Il numero di contratto deve essere assegnato automaticamente. Il rapporto deve diventare attivo solo se registrazione contrattuale, documenti e firma risultano coerenti; in caso di esito parziale il sistema deve evitare duplicazioni e consentire il recupero controllato.[doc:turn1doc3]

**FR-33 — Esito complessivo.** Il sistema deve mostrare l’esito del processo principale e dei sotto-processi, il numero di contratto, la cassetta associata, la data di decorrenza, i documenti prodotti e le eventuali azioni residue.

### G. Messaggi, accessibilità e assistenza operativa

**FR-34 — Messaggi azionabili.** Ogni blocco deve indicare cosa non è valido, perché impedisce la prosecuzione e quale azione può risolverlo. Per KYC assente deve essere mantenuto il significato del messaggio bloccante “Indicare numero questionario di adeguata verifica”.[doc:turn1doc2]

**FR-35 — Accessibilità.** Tutti i controlli devono essere utilizzabili da tastiera, avere etichette programmatiche, focus visibile, ordine di navigazione logico e messaggi non basati sul solo colore. Testi e importi devono poter essere ingranditi senza perdita di informazioni.

**FR-36 — Chiarezza linguistica.** Le etichette devono usare termini comprensibili al gestore; i codici tecnici devono essere mostrati solo quando utili, accompagnati dalla relativa descrizione.

---

## Requisiti Non Funzionali

### Sicurezza

- **NFR-S01 — Autenticazione e autorizzazione:** accesso tramite identità aziendale; autorizzazioni per ruolo, filiale e funzione.
- **NFR-S02 — Segregazione territoriale:** il gestore può operare sulle cassette del proprio impianto, salvo permessi espliciti per consultazioni o strutture centrali.
- **NFR-S03 — Protezione dati:** cifratura dei dati personali e contrattuali in transito e a riposo, secondo gli standard della Banca.
- **NFR-S04 — Minimizzazione:** visualizzare e trasferire ai sotto-processi solo i dati necessari alla specifica finalità.
- **NFR-S05 — Audit:** tracciare avvio, modifiche, controlli, override consentiti, esiti dei sotto-processi, documenti, firme, stipula, annullo e identità dell’operatore.
- **NFR-S06 — Integrità:** garantire idempotenza per creazione pratica, avvio sotto-processi, generazione documenti e stipula, evitando duplicazioni dopo retry.
- **NFR-S07 — Sessione:** proteggere le sessioni inattive e salvaguardare la bozza prima di timeout tecnici, quando consentito.
- **NFR-S08 — Riservatezza operativa:** non esporre informazioni sul contenuto della cassetta; limitare la visibilità dell’impianto agli utenti autorizzati.
- **NFR-S09 — Conservazione:** applicare i tempi di conservazione definiti dalla normativa e dalle policy della Banca; il periodo puntuale per pratica e audit è da confermare.

### Integrazioni

- **FrEE Scheda Cliente:** fornisce contesto di avvio, NDG, dati anagrafici e rapporti pertinenti.
- **Orchestratore FrEE:** governa step, sotto-step, dipendenze, stati, salvataggio, ripresa, annullo, eventi ed esito complessivo.[doc:turn1doc1]
- **Sistema contratti B000 o servizio target:** verifica disponibilità, applica gli automatismi contrattuali, assegna il numero contratto e registra la stipula.
- **Impianto cassette:** espone cassette, classe, dimensione, fila, codice/numero e stato; deve supportare controllo e riserva concorrente.
- **Anagrafe/rapporti:** verifica soggetti, poteri, conti di addebito e relazioni di intestazione.
- **Gestione delegati FrEE:** riceve il contesto del rapporto e restituisce soggetti, poteri, stato ed esito.
- **KYC/GIANOS 3D o servizio FrEE equivalente:** gestisce il questionario per ciascun intestatario, data di compilazione, validità ed esito.
- **ZC00/motore condizioni:** restituisce gruppo e condizioni standard applicabili.
- **DOL:** avvia e aggiorna la deroga, restituendo stato, condizioni approvate, decorrenza ed esito.
- **Documentale/DDC:** genera i documenti, seleziona firmatari e modalità, gestisce la firma unica e restituisce gli esiti.[doc:turn1doc10]
- **Repository documentale:** conserva i documenti prodotti e sottoscritti secondo le regole della Banca.
- **Eventi di integrazione:** ogni scambio deve prevedere identificativo pratica, correlation ID, versione, timestamp, esito funzionale/tecnico e gestione di timeout, retry e duplicati.
- **Prestazioni:** apertura di una schermata o passaggio ordinario tra step con riscontro percepibile entro 3 secondi nel 95° percentile, esclusi i tempi dei sistemi esterni; valore da validare con gli standard tecnici aziendali.
- **Disponibilità:** il processo deve degradare in modo controllato se un sistema esterno non è disponibile, preservando la pratica e consentendo la ripresa; SLA puntuali da definire.

---

## Prossimi Passi

1. **Design** — produrre un mockup navigabile FREE-compliant del processo, includendo percorso standard, cointestazione, delegati, assicurazione integrativa, deroga, KYC, firma ed errori principali.
2. **Validazione** — confermare con Product Owner, Rete, Compliance/AML, Operations, DOL, KYC e Documentale le assunzioni aperte e i confini dell’MVP.
3. **Analisi funzionale** — dettagliare schermata per schermata e campo per campo gli step 1, 2 e 4, definendo controlli, messaggi e ogni esito delle integrazioni.
4. **Contratti di integrazione** — formalizzare dati ed eventi scambiati tra orchestratore, sistema contratti, deleghe, KYC, DOL e Documentale.
5. **Metriche di successo** — concordare baseline e target per tempo medio di apertura, pratiche completate al primo tentativo, errori bloccanti, pratiche sospese e tasso di abbandono.

---

_PRD generato tramite ARchetipo Product Inception - 05/10/2026_  
_Sessione condotta da: stefano marello con il team ARchetipo_
