# Processo Apertura Contratto Cassette di Sicurezza - Documento dei Requisiti di Prodotto

**Autore:** ARchetipo  
**Data:** 05/10/2026  
**Versione:** 1.0

---

## Elevator Pitch

> Portare l'apertura del contratto di cassetta di sicurezza dalla procedura B000/3270 a un processo guidato, orchestrato e tracciabile in FrEE.
>
> Per **i gestori di filiale che devono attivare una cassetta di sicurezza per un cliente**, che oggi devono distribuire l'operatività tra mappe 3270, gestione soggetti, condizioni economiche, KYC e documenti, **Processo Apertura Contratto Cassette di Sicurezza** è un **processo digitale orchestrato di apertura rapporto** che **precompila i dati disponibili, applica automaticamente le condizioni corrette, coordina i sotto-processi e porta alla firma unica della documentazione**. A differenza della **procedura manuale B000/3270**, il processo **centralizza il percorso e conserva uno stato unico della pratica**.

---

## Vision

Rendere l'apertura di una cassetta di sicurezza un'esperienza unica, controllata e verificabile: dalla Scheda Cliente FrEE alla stipula definitiva, senza duplicazioni di inserimento e senza affidare al gestore il coordinamento manuale tra sistemi. Il processo preserva le regole operative B000, trasferendo l'esperienza di lavorazione su FrEE. I colli chiusi sono un'estensione futura, non parte dell'MVP.

### Differenziatore del prodotto

- Avvio contestuale dalla Scheda Cliente FrEE.
- Precompilazione di intestatario e cointestatari.
- Percorso unico con stepper, salvataggio e ripresa.
- Condizioni economiche calcolate e trasparenti, senza modifiche libere.
- Integrazione con delegati, KYC, DOL e firma unica.
- Riepilogo finale coerente con i dati inviati ai sistemi bancari.

La procedura B000 attuale assegna automaticamente il numero contratto, richiede una cassetta libera e disponibile nell'impianto della filiale operante, gestisce soggetti, condizioni e stipula tramite ACC/ACD/F10. Il nuovo processo deve conservarne le regole senza riprodurne la frammentazione[doc:turn1doc3].

---

## User Personas

### Persona 1: Gestore di filiale

**Ruolo:** Consulente/operatore abilitato alla vendita e all'apertura rapporti  
**Età:** 25-60 anni | **Contesto:** filiale, cliente presente, obbligo di correttezza operativa

**Obiettivi:** aprire senza passaggi ridondanti; avere dati già disponibili; comprendere costi e condizioni; gestire soggetti; sapere cosa manca.

**Punti dolenti:** navigazione tra B000/3270, ACD, ZC00/DOL, KYC e DynDoc; duplicazione dati; poca visibilità delle dipendenze; rischio di condizioni non ammesse; difficoltà nel riprendere pratiche interrotte.

**Comportamenti e strumenti:** parte dalla Scheda Cliente FrEE, usa applicativi bancari legacy e documentazione digitale, verifica la disponibilità nell'impianto e lavora con il cliente davanti a sé.  
**Motivazioni:** velocità, conformità, riduzione del lavoro manuale, sicurezza.  
**Dimestichezza tecnologica:** media.

#### Viaggio del cliente - Gestore di filiale

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Consapevolezza | Avvia dalla Scheda Cliente | Devo partire dal cliente corretto | Attento | Ingresso contestuale |
| Valutazione | Verifica cassetta, soggetti, assicurazione e condizioni | Non voglio dimenticare un costo | Prudente | Precompilazione |
| Primo utilizzo | Compila mancanti e avvia sotto-processi | Cosa posso modificare? | Concentrato | Dati automatici/editabili |
| Uso ricorrente | Riprende pratiche | Devo ripartire da dove ero rimasto | Rassicurato | Widget e autosalvataggio |
| Promozione | Conclude e riutilizza il flusso | È più semplice del vecchio | Soddisfatto | Esito e documenti |

---

### Persona 2: Cliente titolare o cointestatario

**Ruolo:** Cliente maggiorenne che sottoscrive da solo o con altri intestatari  
**Età:** 18+ | **Contesto:** cliente CAI residente o non residente in Italia, consumatore o non consumatore, con conto corrente CAI

**Obiettivi:** comprendere servizio, massimale e costi; definire intestatari/delegati; firmare documentazione coerente e unica.

**Punti dolenti:** dati ripetuti, costi poco leggibili, firme separate, incertezza su chi può operare.  
**Comportamenti e strumenti:** fornisce conferme al gestore, può indicare delegati, usa il canale di firma con assistenza.  
**Motivazioni:** tutela, riservatezza, semplicità, certezza.  
**Dimestichezza tecnologica:** variabile; deve funzionare anche con assistenza completa.

#### Viaggio del cliente - Cliente titolare o cointestatario

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Consapevolezza | Valuta il servizio | Voglio proteggere beni importanti | Interessato | Benefici e limiti chiari |
| Valutazione | Definisce soggetti e cassetta | Chi potrà accedere e quanto pagherò? | Cauto | Riepilogo leggibile |
| Primo utilizzo | Fornisce dati, KYC e verifica | I dati devono essere corretti | Coinvolto | Conferme |
| Uso ricorrente | Usa servizio e documenti | Devo dimostrare cosa ho sottoscritto | Tranquillo | Copia documenti |
| Promozione | Raccomanda il servizio | La banca ha gestito tutto | Fiducioso | Ridurre tempi |

---

## Risultati del brainstorming

> Sono state esplorate alternative e rischi del passaggio da B000/3270 a FrEE.

### Assunzioni messe alla prova

1. Digitalizzare la sola ACC basta: falso, serve orchestrare soggetti, condizioni, KYC, DOL e documentale.
2. Tutti i dati sono editabili: rischio; dati e sconti derivati devono essere protetti e le eccezioni passare da DOL.
3. Serve solo al gestore: l'audience flip evidenzia leggibilità di costi, delegati, massimale e firma.
4. Cassetta e collo chiuso nello stesso MVP: le mappe sono differenti; si parte dalla cassetta con modello estendibile.

### Nuove direzioni individuate

Modello parametrico; distinzione tra dati automatici, inseriti e calcolati; sotto-processi con stato/esito/dipendenze; riepilogo economico prima di KYC e firma; pratica riprendibile e consultabile.

### Assunzioni da validare

- Disponibilità firma unica per tutti i documenti.
- API/eventi ACC/B000 per contratto intermedio e stipula.
- Selezione cassetta libera eseguibile dalla nuova esperienza o da servizio dedicato.
- Regole di target market e condizioni esposte da fonte centralizzata.
- Deroga DOL sospendibile e restituibile con esito strutturato.
- Correlazione stabile con KYC e regola su istanze concorrenti per NDG.
- Stessa fonte per condizioni riepilogate e contratto.

### Rischi principali

Disallineamento tra FrEE, B000, anagrafe, rapporti e documentale; errata applicazione di target market, poteri, condizioni e sconti; timeout dei sotto-processi; cassetta non più libera tra selezione e stipula; bassa adozione; esposizione impropria di dati; pratica intermedia non riprendibile.

---

## Perimetro del prodotto

### MVP - Prodotto minimo funzionante

Processo avviato dalla Scheda Cliente FrEE e gestito dall'orchestratore:

1. Inserimento dati cassetta, contratto e assicurazione; intestatario/cointestatari precompilati; regole automatiche B000 mantenute; campi non necessari rimossi; azione delegati.
2. Aggancio al processo FrEE dei delegati, se richiesti.
3. Calcolo e visualizzazione delle condizioni per intestatario, tipologia/dimensione, cointestatari e delegati; deroga tramite DOL.
4. Aggancio KYC e blocco se obbligatorio e non concluso.
5. Riepilogo, conferma e produzione documenti.
6. Documentale, firma unica, esito e disponibilità documentazione.

Inclusi autosalvataggio, ripresa, controlli d'ingresso, audit, errori di integrazione e consultazione. Gli step 1, 2 e 4 producono documento di requisitazione; delegati, KYC e documentale/firma riusano processi esistenti[doc:turn1doc1].

### Funzionalità di crescita (post-MVP)

Variante colli chiusi; avvio da altri contesti; promozioni autorizzate; pagamenti/cassa; dashboard filiale; inquiry canoni/insoluti/impianto; modifica delegati, contratto ed estinzione; notifiche cliente.

### Visione (futuro)

Ciclo di vita completo: apertura, accessi, blocchi/sblocchi, addebiti, insoluti, modifica e chiusura; motore comune cassette/colli; controlli automatici; analisi tempi e abbandoni; esperienza cliente assistita.

---

## Requisiti funzionali

### Avvio, eleggibilità e pratica

**RF-01. Avvio contestuale:** avvio dal menù laterale della Scheda Cliente FrEE con cliente e NDG.  
**RF-02. Pratiche concorrenti:** verifica pratica aperta per NDG e ripresa o nuova istanza secondo regola.  
**RF-03. Controlli iniziali:** gestore, visibilità/circolarità, eleggibilità, conto di addebito e impianto.  
**RF-04. Stato pratica:** stato, step, sotto-processi, ultima modifica, gestore, sospensione/blocco.\n+
### Dati e soggetti

**RF-05. Precompilazione:** intestatario e cointestatari dalla Scheda Cliente, marcati come precompilati.  
**RF-06. Cassetta:** identificazione di cassetta libera e disponibile con filiale, fila, numero, classe e caratteristiche.  
**RF-07. Contratto:** conto, durata, poteri, chiavi, recapiti e note, distinguendo automatico/editabile.  
**RF-08. Assicurazione:** copertura base e integrativa con massimale, premio, decorrenza e modalità; massimale base **€52.000,00**[doc:turn1doc9].  
**RF-09. Cointestatari:** dati, operatività congiunta/disgiunta e diritti; **€5,16 annui per ogni cointestatario oltre il primo**[doc:turn1doc2].  
**RF-10. Delegati:** azione dedicata, sospensione del principale, avvio processo FrEE e restituzione esito.  
**RF-11. Validazione:** obbligatorietà, formato, dominio e dipendenze B000/FrEE; blocco in caso di errore.\n+
### Condizioni e deroga

**RF-12. Calcolo:** condizioni per intestatario, tipologia/dimensione, cointestatari, delegati e offerta autorizzata.  
**RF-13. Trasparenza:** canone, riduzione, diritti, spese, bollo, premio e modalità/frequenza addebito.  
**RF-14. Scontistica protetta:** divieto di modifica libera; eccezioni solo via DOL.  
**RF-15. Deroga:** avvio DOL, correlazione, sospensione e ricezione condizioni/motivazione.\n+
### KYC, finalizzazione e documentale

**RF-16. KYC:** avvio, correlazione e ricezione esito/dati.  
**RF-17. Blocco KYC:** impedire finalizzazione/firma per questionario mancante, scaduto, non valido o bloccante.  
**RF-18. Riepilogo:** cassetta, contratto, soggetti, poteri, assicurazione, condizioni ed esiti.  
**RF-19. Conferma:** conferma riepilogo e produzione; modifiche riportano allo step corretto e ricalcolano dipendenze.  
**RF-20. Documenti:** requisitazione per step 1, 2 e 4 e documenti contrattuali applicabili.  
**RF-21. Firma unica:** firma su tutti i documenti; efficacia solo con esito positivo.  
**RF-22. Stipula:** creazione/aggiornamento e acquisizione numero assegnato automaticamente.  
**RF-23. Esito:** completato, KO o sospeso con pratica, contratto, documenti, motivazione e azione.\n+
### Continuità, integrazioni e audit

**RF-24. Autosalvataggio:** almeno a ogni sotto-step e prima di ogni sotto-processo.  
**RF-25. Ripresa:** ripartenza dallo step corretto con dati ed esiti.  
**RF-26. Errori:** dati mantenuti, sistema identificato, retry governato e niente duplicati.  
**RF-27. Audit:** eventi, modifiche, stati, integrazioni, conferme, annulli e firma con utente/data/ora.  
**RF-28. Consultazione:** step completati in sola lettura per almeno **6 mesi** dalla chiusura[doc:turn1doc7].  
**RF-29. Annullamento:** motivazione obbligatoria e chiusura/compensazione sotto-processi.  
**RF-30. Ruoli:** rispetto di filiale, impianto, ruolo e circolarità.\n+
---

## Requisiti non funzionali

### Sicurezza

- Autenticazione FrEE e autorizzazione verificata lato servizio.
- Minimizzazione e protezione di conto, codice fiscale e dati delegati.
- Segregazione per filiale, impianto, ruolo e circolarità.
- Comandi idempotenti con identificativo pratica.
- Audit non alterabile con utente, timestamp, esito e sistema.
- Policy di privacy, conservazione, accesso e cancellazione.
- Firma associata ai documenti riepilogati, senza sostituzioni non tracciate.
- Messaggi comprensibili e codice di correlazione, senza stack trace.

### Integrazioni

- **RNF-I01:** FrEE/Scheda Cliente per contesto, NDG e soggetti.
- **RNF-I02:** orchestratore per step, sotto-step, dipendenze, loader, stato, sospensione, ripresa, annullo ed esito.
- **RNF-I03:** B000/ACC per disponibilità, dati contratto, numero e stipula.
- **RNF-I04:** B000/ACD o FrEE per intestatari, cointestatari, delegati e poteri.
- **RNF-I05:** ZC00/DOL per condizioni e deroga; nessuna scontistica libera in B000.
- **RNF-I06:** KYC per questionario, correlazione ed esito.
- **RNF-I07:** Documentale/DynDoc per requisitazione e documenti, incluso **17.006 Locazione cassette di sicurezza**.
- **RNF-I08:** Firma FrEE per firma unica ed esito.
- **RNF-I09:** eventi/API con pratica, NDG, contratto, versione, timestamp, esito ed errore.
- **RNF-I10:** timeout, retry, compensazione, doppio invio e intervento manuale definiti per integrazione.

### Prestazioni, accessibilità e usabilità

- Loader full-page per caricamenti non immediati senza perdita dati[doc:turn1doc7].
- Tempi di navigazione e salvataggio compatibili con il lavoro allo sportello; obiettivi puntuali da definire.
- Uso da tastiera, focus coerente, etichette accessibili e messaggi associati ai campi.
- Errori comunicati con testo e focus sul primo errore bloccante.
- Stepper con stato corrente, completato, sospeso e bloccato; consultazione degli step conclusi[doc:turn1doc7].
- Indicazione esplicita dei dati automatici.

---

## Prossimi passi

1. **Design** - mockup per compilazione, delegati, deroga, KYC, errore, sospensione, riepilogo, firma ed esito.
2. **Validazione** - verifica con Rete, Conti Correnti e Prestiti, KYC, DOL, Documentale, Sicurezza e FrEE.
3. **Contratti di integrazione** - eventi, API, payload, idempotenza, errori e compensazioni.
4. **Regole economiche** - matrice di condizioni, promozioni, diritti e deroghe.
5. **Pilota operativo** - casi monointestato, cointestato, delegato, assicurazione, deroga, KYC bloccante e ripresa.

---

_PRD generato tramite ARchetipo Product Inception - 05/10/2026_  
_Sessione condotta da: stefano marello con il team ARchetipo_
