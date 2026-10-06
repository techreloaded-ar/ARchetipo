# apertura-contratto - Documento dei requisiti di prodotto

**Autore:** ARchetipo
**Data:** 30/09/2026
**Versione:** 1.0

---

## Elevator Pitch

> apertura-contratto digitalizza integralmente in FrEE la stipula delle cassette di sicurezza.
>
> Per **i gestori di filiale**, che oggi devono attraversare sistemi e controlli distinti, **apertura-contratto** è un **processo bancario orchestrato** che **riduce con pari priorità tempi, errori e rilavorazioni**. Diversamente da 3270/B000 e dalla compilazione manuale, offre **un percorso unico con dati precompilati, controlli anticipati, pratica recuperabile e firma unica**.

---

## Visione

Consentire a ogni gestore autorizzato di completare una nuova apertura ordinaria senza conoscere codici o sequenze di 3270/B000, mantenendo la conformità normativa e contrattuale. FrEE diventa il solo punto di accesso e governo; i sistemi specialistici operano tramite integrazioni trasparenti.

### Elemento differenziante del prodotto

Il prodotto non replica le mappe legacy: ricompone dati, controlli e sotto-processi in una esperienza end-to-end, richiede solo le informazioni mancanti e conduce il gestore fino alla sottoscrizione e all’esito definitivo.

---

## Personas utente

### Persona 1: Giulia, gestore di filiale

**Ruolo:** Gestore abilitato all’apertura di cassette di sicurezza
**Età:** 25-65 anni | **Contesto:** Opera in filiale, serve il cliente in presenza e usa FrEE.

**Obiettivi:**
- Concludere l’apertura durante l’appuntamento.
- Sapere quali dati, controlli e firme mancano.
- Evitare reinserimenti, errori e rilavorazioni.
- Sospendere e riprendere la pratica senza perdere dati.

**Punti di dolore:**
- Passaggi tra applicazioni e transazioni differenti.
- Codici e campi tecnici da ricordare.
- Compilazione manuale di dati già presenti.
- Errori tardivi e pratiche interrotte difficili da ricostruire.

**Comportamenti e strumenti:**
- Avvia dalla Scheda Cliente FrEE.
- Consulta anagrafe, rapporti, cassette e condizioni.
- Usa FrEE per delegati, KYC e firma; DOL per le deroghe.

**Motivazioni:** Servire rapidamente il cliente e concludere correttamente al primo tentativo.
**Competenza digitale:** Variabile; non è richiesta conoscenza di 3270/B000.

#### Customer Journey - Giulia, gestore di filiale

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Consapevolezza | Riceve la richiesta. | Posso offrire il servizio? | Attenzione | Mostrare prerequisiti e disponibilità. |
| Valutazione | Avvia il processo. | Il cliente è idoneo? | Cautela | Anticipare controlli e sanatorie. |
| Primo utilizzo | Seleziona cassetta, soggetti e assicurazione. | Sto inserendo solo ciò che manca? | Fiducia | Precompilare e guidare. |
| Uso regolare | Verifica condizioni, KYC e riepilogo. | Posso finalizzare? | Controllo | Rendere chiari dipendenze ed esiti. |
| Promozione | Riutilizza il processo. | È più semplice del legacy. | Soddisfazione | Garantire coerenza e velocità. |

---

### Persona 2: Marco, cliente correntista

**Ruolo:** Cliente maggiorenne della Banca, consumatore o non consumatore al dettaglio
**Età:** 18+ | **Contesto:** Richiede una cassetta e può indicare cointestatari e delegati.

**Obiettivi:**
- Comprendere caratteristiche, costi e assicurazione.
- Scegliere una cassetta adeguata e disponibile.
- Definire soggetti e poteri di firma.
- Firmare tutta la documentazione senza ripetizioni.

**Punti di dolore:**
- Attese dovute a passaggi interni.
- Richiesta ripetuta delle stesse informazioni.
- Costi e opzioni difficili da comprendere.
- Necessità di tornare in filiale per completare la pratica.

**Comportamenti e strumenti:**
- Interagisce con il gestore, conferma dati e scelte.
- Completa la adeguata verifica e firma i documenti.

**Motivazioni:** Proteggere beni e documenti con un servizio affidabile e comprensibile.
**Competenza digitale:** Non determinante; l’esperienza include clienti anziani o con disabilità.

#### Customer Journey - Marco, cliente correntista

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Consapevolezza | Chiede informazioni. | È adatto a me? | Interesse | Spiegare benefici e vincoli. |
| Valutazione | Confronta costi e assicurazione. | Quanto costa e cosa copre? | Prudenza | Mostrare un riepilogo chiaro. |
| Primo utilizzo | Fornisce dati e soggetti. | Perché servono questi dati? | Incertezza | Spiegare finalità e avanzamento. |
| Uso regolare | Verifica e firma. | I documenti riflettono le scelte? | Fiducia | Assicurare coerenza. |
| Promozione | Riceve conferma. | È stato semplice. | Sicurezza | Consegnare esito e documenti. |

---

## Risultati del brainstorming

> Principali scoperte e direzioni alternative esplorate durante la sessione di inception.

### Assunzioni messe in discussione

- Sostituire B000 non significa replicarne mappe e codici: vanno preservati dati, regole ed esiti.
- Il percorso deve funzionare anche per chi non conosce il legacy.
- Gli errori devono essere anticipati, non concentrati alla fine.
- Una pratica può richiedere più sessioni e più gestori autorizzati.
- Il rischio dominante è la incompletezza delle integrazioni e dei casi operativi.

### Nuove direzioni individuate

- Punto unico FrEE con sistemi specialistici trasparenti.
- Precompilazione estesa e controlli contestuali.
- Condizioni automatiche e deroga solo tramite DOL.
- Salvataggio per sotto-step e ripresa puntuale.
- Riepilogo prima dei documenti ed esito complessivo con retry sicuro.

### Assunzioni da validare

- Rilevare baseline di tempo medio, rilavorazioni e campi manuali prima di fissare target numerici.
- Confermare autorizzazioni per la ripresa da parte di un altro gestore della stessa U.O.
- Confermare fonti autorevoli e dati recuperabili.
- Definire gestione transitoria delle pratiche legacy incomplete.
- Confermare unicità, durata e scadenza delle pratiche sospese.
- Validare controlli POG e KYC e documenti prodotti per ogni configurazione.

### Rischi principali

- Integrazioni incomplete o indisponibili.
- Disallineamento tra dati temporanei e definitivi.
- Regole legacy non censite.
- Condizioni o documenti incoerenti.
- Perdita dati o aggiornamenti concorrenti.
- Terminologia tecnica e accessibilità insufficiente.

---

## Perimetro del prodotto

### MVP - Minimum Viable Product

- Solo apertura di cassette di sicurezza dalla Scheda Cliente FrEE.
- Monointestazione, cointestazione, delegati e poteri di firma.
- Selezione di una cassetta libera nella filiale competente.
- Precompilazione di cliente, intestatari, filiale e rapporti.
- Conto di addebito e assicurazione standard o integrativa.
- Condizioni automatiche e avvio deroga DOL.
- Controlli Target Market e KYC per ogni intestatario.
- Riepilogo, documenti, firma unica ed esito.
- Salvataggio, sospensione, ripresa, annullamento e passaggio tra gestori autorizzati della stessa U.O.
- Esclusione dei casi con conto interno 111111.
- Fuori perimetro: colli chiusi, modifiche post-apertura, estinzione, accessi, blocchi, sblocchi e insoluti.

### Funzionalità evolutive (post-MVP)

- Casi eccezionali con conto 111111.
- Modifica di delegati, contratto e coordinate di addebito.
- Estinzione e recesso.
- Inquiry impianto estesa ad altre filiali.
- Accessi, blocchi, sblocchi, canoni, insoluti e assicurazioni.
- Dashboard su tempi, anomalie e rilavorazioni.

### Visione futura

- Gestione in FrEE dell’intero ciclo di vita della cassetta.
- Preparazione omnicanale e prenotazione.
- Prevenzione proattiva degli errori.
- Monitoraggio operativo e normativo centralizzato.
- Estensione controllata ad altri servizi di custodia.

---

## Requisiti funzionali

### Avvio e controlli

**RF-01.** Avviare il processo dal menu laterale della Scheda Cliente FrEE.

**RF-02.** Verificare ruolo, U.O., visibilità e possibilità di operare su cliente e filiale.

**RF-03.** Impedire l’avvio in presenza di controlli bloccanti, spiegandone il motivo.

**RF-04.** Individuare una pratica già attiva e proporne la ripresa secondo la regola di unicità approvata.

**RF-05.** Creare una pratica univoca e pubblicarla nel widget Gestione Processi.

**RF-06.** Verificare maggiore età, perimetro cliente e Target Market.

**RF-07.** Verificare un conto idoneo; se diversamente intestato, deve appartenere ad almeno un intestatario.

**RF-08.** Mostrare i controlli sanabili in uno step dedicato e rieseguirli dopo la sanatoria.

### Dati, cassetta e soggetti

**RF-09.** Recuperare dalla Scheda Cliente e dall’anagrafe i dati del primo intestatario.

**RF-10.** Aggiungere cointestatari e scegliere operatività congiunta o disgiunta.

**RF-11.** Mostrare solo cassette libere e disponibili della filiale competente.

**RF-12.** Acquisire filiale, eventuale fila, numero, classe, volume e caratteristiche necessarie.

**RF-13.** Riconfermare la disponibilità prima della stipula per evitare assegnazioni concorrenti.

**RF-14.** Se richiesto, avviare il sotto-processo delegati e acquisirne esito e poteri.

### Assicurazione e condizioni

**RF-15.** Mostrare la copertura standard inclusa con massimale di EUR 52.000,00.

**RF-16.** Consentire la copertura integrativa con massimale aggiuntivo e conto di addebito.

**RF-17.** Recuperare premio, decorrenza e condizioni della copertura integrativa.

**RF-18.** Calcolare condizioni standard, costi ricorrenti, una tantum, imposte e spese.

**RF-19.** Impedire la modifica diretta di sconti e condizioni calcolate.

**RF-20.** Avviare la deroga DOL e usare solo le condizioni definitive restituite.

### KYC, documenti e firma

**RF-21.** Collegare un KYC valido a ogni intestatario e impedire la stipula in assenza.

**RF-22.** Verificare la validità del questionario, pari a 15 giorni di calendario per questa apertura.

**RF-23.** Riepilogare cassetta, soggetti, poteri, addebito, assicurazione e condizioni.

**RF-24.** Consentire correzioni agli step precedenti e invalidare gli output da rigenerare.

**RF-25.** Generare contratto e documenti applicabili senza ricompilazione manuale.

**RF-26.** Avviare un unico iter di firma per documenti e firmatari.

**RF-27.** In caso di annullo o scadenza firma, tornare allo stato precedente e consentire la ripresa.

### Persistenza ed esito

**RF-28.** Salvare automaticamente i dati almeno a ogni sotto-step in archivio temporaneo.

**RF-29.** Consentire sospensione e ripresa dall’ultimo sotto-step completato.

**RF-30.** Consentire presa in carico da altro gestore autorizzato della stessa U.O., tracciandola.

**RF-31.** Prevenire modifiche concorrenti con presa in carico o blocco esplicito.

**RF-32.** Consentire l’annullamento confermato della pratica e dei sotto-processi.

**RF-33.** Attivare il contratto solo dopo firma e sotto-processi conclusi positivamente.

**RF-34.** Mostrare esito, numero contratto, documenti, sotto-processi e azioni residue.

**RF-35.** Consentire retry idempotente in caso di errore, senza duplicazioni.

**RF-36.** Rendere la pratica conclusa consultabile nello storico del widget.

### Misurazione

**RF-37.** Rilevare durata totale e per step, sospensioni, errori, controlli KO, firma e rilavorazioni.

**RF-38.** Distinguere dati precompilati, inseriti e modificati manualmente.

**RF-39.** Confrontare le metriche con le baseline senza esporre dati cliente non autorizzati.

---

## Requisiti non funzionali

### Sicurezza

- Autenticazione e autorizzazione per ruolo, U.O., cliente e impianto.
- Minimo privilegio e audit non alterabile.
- Cifratura in transito e a riposo, inclusi dati temporanei.
- Identificativi univoci, idempotenza e protezione dalla doppia stipula.
- Conservazione controllata dei dati temporanei e definitivi.
- Nessun dato sensibile in log o errori non autorizzati.
- Timeout di sessione e ripresa sicura.
- Conformità a privacy, antiriciclaggio, POG e policy Banca.
- Accessibilità da tastiera e tecnologie assistive, focus visibile, etichette chiare e messaggi non basati solo sul colore.
- Errori comprensibili con causa, impatto e azione di recupero.

### Integrazioni

- **Scheda Cliente e anagrafe:** NDG e dati dei soggetti.
- **Rapporti:** conti di addebito.
- **Impianto cassette:** disponibilità, caratteristiche e conferma atomica.
- **Motore contratti:** numero, attivazione e persistenza.
- **Delegati FrEE:** avvio, dati ed esito.
- **Target Market/POG:** verifica preventiva.
- **KYC:** identificativo, data, validità ed esito per intestatario.
- **ZC00 o servizio condizioni:** condizioni standard.
- **DOL:** deroga, correlazione, stato e condizioni definitive.
- **Servizio assicurativo:** copertura, premio e decorrenza.
- **Documentale e firma:** modelli, documenti, firmatari ed esito.
- **Orchestratore FrEE:** step, dipendenze, ripresa, annullo ed esito.
- **Widget Gestione Processi:** pratiche in corso e storico.
- **Audit e monitoraggio:** eventi tecnici e di business correlati.
- Le integrazioni di finalizzazione devono avere timeout controllati, retry idempotenti e riconciliazione degli esiti incerti.

---

## Passi successivi

1. **Design** - richiedere i mockup UI del processo applicando le FREE UX Guidelines
2. **Validazione** - revisionare con gli stakeholder e verificare le assunzioni più rischiose

---

_PRD generato tramite ARchetipo Product Inception - 30/09/2026_
_Sessione condotta da: Stefano Marello con il team ARchetipo_
