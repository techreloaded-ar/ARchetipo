# Apertura contratti cassette di sicurezza — Documento dei requisiti di prodotto

**Autore:** ARchetipo  
**Data:** 2026-10-05  
**Versione:** 1.0

---

## Presentazione sintetica

> Un percorso assistito nella Scheda Cliente FrEE per aprire un contratto di locazione di cassetta di sicurezza: acquisire i dati noti, verificare disponibilità e requisiti, mostrare condizioni, collegare deleghe e KYC, sottoscrivere i documenti e conoscere l'esito effettivo. Il documento di processo futuro prescrive cinque step, uno step facoltativo delegati e l'orchestrazione dei passaggi.[doc:turn1doc1]
>
> Per **gestori di filiale**, che devono coordinare B000, anagrafica, adeguata verifica e documenti, **Apertura contratti cassette di sicurezza** è un **percorso di vendita in FrEE** che **rende visibili prerequisiti, condizioni ed esito**. Diversamente dall'operatività distribuita tra transazioni B000 e compilazione manuale DynDoc, collega sistemi e attività in un'esperienza unitaria senza eliminare i controlli bancari.[doc:turn1doc3][doc:turn1doc4][doc:turn1doc1]

---

## Visione

Portare la richiesta di cassetta dalla Scheda Cliente alla conferma di contratto senza trascrizioni evitabili, condizioni opache o stati ambigui. **Obiettivi da misurare, non baseline accertate:** tempo mediano dall'avvio all'esito, percentuale conclusa senza reinserimento, abbandoni per prerequisito, scarti documentali e casi «firmato ma non stipulato». Nessun valore iniziale o target è stato fornito.

### Elemento distintivo del prodotto

Non una mera nuova maschera 3270: il percorso collega soggetti, cassetta fisica, conto, condizioni, delegati, KYC, documenti e stato finale. Il processo futuro richiede firma unica dei documenti; l'integrazione FrEE prevede un unico step documentale quando possibile.[doc:turn1doc1][doc:turn1doc7]

---

## Profili utente

### Profilo 1: Giulia, gestore di filiale

**Ruolo:** operatrice autorizzata alla vendita assistita.  
**Età:** non rilevante | **Contesto:** assiste i clienti e applica i vincoli di filiale.

**Obiettivi:** trovare una cassetta idonea, illustrare costi verificati e completare l'apertura senza discrepanze tra sistemi.

**Difficoltà:** l'operatività attuale attraversa ACC/ACD, inquiry impianto ACV, condizioni ZC00/DOL e contratto DynDoc da compilare manualmente; B000 distingue inoltre «in attesa di definizione» dalla stipula definitiva.[doc:turn1doc3][doc:turn1doc4]

**Comportamenti e strumenti:** usa FrEE, B000 e sistemi specialistici; controlla manualmente anomalie e dati mancanti.  
**Motivazioni:** servizio affidabile e meno rilavorazione.  
**Familiarità digitale:** alta negli strumenti interni, ma le interfacce eterogenee aumentano il carico cognitivo.

#### Percorso utente — Giulia

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Scoperta | Riceve la richiesta in filiale | «C'è una cassetta adatta?» | Attenta | Disponibilità e caratteristiche leggibili |
| Valutazione | Verifica conto, persone e KYC | «Quali prerequisiti mancano?» | Prudente | Controlli espliciti e sanabili |
| Primo utilizzo | Avvia da Scheda Cliente | «Non voglio ricopiare i dati» | Fiduciosa | Precompilazione verificabile |
| Uso ricorrente | Verifica importi, firma ed esito | «Il rapporto è attivo?» | Responsabile | Riepilogo e stato affidabili |
| Condivisione | Raccomanda il percorso ai colleghi | «Si può usare ogni giorno» | Soddisfatta | Processo coerente anche in caso di sospensione |

---

### Profilo 2: Marco, cliente assistito

**Ruolo:** cliente che vuole locare una cassetta, eventualmente con cointestatari o delegati.  
**Età:** adulto; età specifica non rilevante | **Contesto:** desidera custodire beni o documenti. La circolare indica clienti maggiorenni residenti e non residenti in Italia, con verifica del target market.[doc:turn1doc4]

**Obiettivi:** conoscere costi, durata, copertura e persone che potranno accedere; ricevere copia e conferma dell'apertura.

**Difficoltà:** distinguere proposta, disponibilità fisica e contratto perfezionato; comprendere firme congiunte/disgiunte e voci accessorie.

**Comportamenti e strumenti:** interagisce soprattutto con il gestore; il mod. 17.006 richiede durata, dati della cassetta, delegati e tipo di operatività.[doc:turn1doc5]  
**Motivazioni:** riservatezza e chiarezza prima della firma.  
**Familiarità digitale:** variabile; evitare di presupporre smartphone o posta elettronica.

#### Percorso utente — Marco

| Fase | Azione | Pensiero | Emozione | Opportunità |
|---|---|---|---|---|
| Scoperta | Chiede informazioni | «È il servizio giusto?» | Interessato | Spiegazione semplice |
| Valutazione | Esamina condizioni e facoltà | «Quanto costa e chi accede?» | Cauto | Importi e poteri comprensibili |
| Primo utilizzo | Fornisce dati e svolge controlli | «Perché servono?» | In attesa | Spiegazione del KYC |
| Uso ricorrente | Firma e riceve documenti | «È operativo?» | Rassicurato | Esito non ambiguo |
| Condivisione | Racconta la propria esperienza | «Mi hanno spiegato tutto» | Fiducioso | Linguaggio inclusivo e supporto umano |

---

## Spunti emersi dalla riflessione

> Esplorazioni autonome, non decisioni confermate dal committente.

### Ipotesi messe in discussione

- **Sfida all'assunto:** «Il problema è solo la schermata 3270». Se falso, ridisegnare ACC senza coordinare KYC, condizioni e documenti non elimina l'attrito: da qui un percorso orchestrato che riusa sistemi specialistici.[doc:turn1doc1][doc:turn1doc3]
- **Anti-problema:** «Come rendere l'apertura inaffidabile?» Mostrare cassetta ormai occupata, firmare condizioni superate, perdere delegati tra processi o scambiare «firmato» per «attivo». Contromisure: ricontrollo, ricalcolo e riconciliazione.
- **E se** il cliente non potesse usare la firma digitale? Preservare una modalità assistita; FrEE descrive opzioni Cartacea, OTP e VAD con limiti per i sottoprocessi, la cui abilitazione va verificata per questo prodotto.[doc:turn1doc7]

### Nuove direzioni individuate

- Limitare la prima consegna alla nuova apertura, anziché aggiungere modifica, estinzione, accessi, blocchi e canoni: l'elenco Rete li tratta come funzioni distinte.[doc:turn1doc9]
- Esporre una pratica «sospesa per prerequisito» anziché forzare l'avanzamento; spiegare al cliente i passaggi senza sostituire il contratto ufficiale.

### Ipotesi da validare

1. **Perimetro/MVP:** il processo futuro tratta cassette *e colli chiusi*; la richiesta privilegia le cassette. Assunzione: MVP solo cassette; colli successivamente. Da confermare con il product owner.[doc:turn1doc1]
2. **Utente, problema e valore:** gestore come utilizzatore principale, cliente come beneficiario; attrito multi-sistema e beneficio della precompilazione sono inferenze dall'as-is, non risultati di ricerca utenti.[doc:turn1doc3][doc:turn1doc4]
3. **Rischio di adozione:** se le casistiche frequenti obbligano a ripartire su 3270, il gestore non adotterà il percorso. Validare frequenze delle eccezioni.
4. **Deroga pre-stipula — decisione critica:** il to-be chiede di avviare DOL dalle condizioni; la circolare documenta DOL per modifiche su contratti già stipulati e il manuale commerciale esclude ulteriori deroghe in fase di vendita. Verificare norma, identificativo e iter prima di offrire il pulsante come operazione attiva.[doc:turn1doc1][doc:turn1doc4][doc:turn1doc5]
5. **Firma/attivazione — decisione critica:** definire riserva della cassetta, ID provvisorio, firma e stipula B000, con compensazioni in caso di errore. B000 ha stato «in attesa di definizione»; FrEE prevede conferma definitiva dei dati dopo esito firma.[doc:turn1doc3][doc:turn1doc7]
6. **Istanze parallele:** più richieste per lo stesso NDG? Riserva della stessa cassetta per quanto? Le linee guida FrEE chiedono una decisione esplicita sulle istanze contemporanee.[doc:turn1doc7]
7. **Casi eccezionali:** rappresentanti di persone giuridiche, Ente Fiere, uso banca, conto 111111, polizza integrativa e cassetta in altra filiale richiedono una matrice di ammissibilità, senza presumerli inclusi.[doc:turn1doc3][doc:turn1doc4]
8. **Tariffe e documenti:** confermare catalogo, condizioni, moduli, firmatari e limiti della firma unica con i sistemi titolari; non trasformare tariffe commerciali d'esempio in costanti applicative.[doc:turn1doc4][doc:turn1doc5]
9. **Metriche e accessibilità:** definire baseline/target e testare comprensione di importi, facoltà congiunte/disgiunte e modalità di firma con operatori e clienti.

### Rischi principali

- Adozione bassa se eccezioni e sospensioni rimandano alla procedura legacy.
- Disallineamento tra inventario, B000, condizioni, KYC e firma, soprattutto «firmato ma non stipulato».
- Documenti con prezzi non corrispondenti alle condizioni definitive.
- Mancanza di questionario valido per un cointestatario o verifica insufficiente del target market.[doc:turn1doc4]
- Esperienza poco accessibile per clienti con esigenze digitali o cognitive diverse.

---

## Perimetro del prodotto

### MVP — Prodotto minimo utilizzabile

**Perimetro proposto, da confermare:** nuove aperture di *cassette di sicurezza* nella filiale competente per clienti e cointestatari già censiti, con conto corrente idoneo e cassetta libera. L'eventuale delega è gestita tramite processo FrEE esistente: non si sviluppa una nuova applicazione deleghe, KYC o documentale.[doc:turn1doc1] Il conto d'appoggio può avere diversa intestazione dal contratto, ma deve appartenere ad almeno uno degli intestatari della cassetta.[doc:turn1doc4]

**Percorso incluso:** Scheda Cliente → dati cassetta/contratto/assicurazione → eventuali delegati → condizioni → KYC → riepilogo → documentale e firma unica → esito riconciliato. Il to-be richiede la requisitazione degli step **1, 2 e 4**; step **1-bis, 3 e 5** sono collegamenti con processi esistenti.[doc:turn1doc1]

**Cancelli:** disponibilità e filiale, conto, target market, questionario per ciascun intestatario e validità prima della sottoscrizione: il questionario preventivo ha validità di **15 giorni di calendario**.[doc:turn1doc3][doc:turn1doc4] Condizioni lette dal sistema titolare e non alterabili liberamente nel frontend; la deroga pre-stipula resta vincolata alla decisione critica sopra.[doc:turn1doc1][doc:turn1doc4][doc:turn1doc5]

**Esclusioni:** colli chiusi, modifiche/estinzione, accessi, blocchi, insoluti, inquiry generale impianto, pagamento per cassa, tabulati, self-service cliente.[doc:turn1doc9] Per i casi non coperti, avvisare prima della firma e indirizzare all'operatività autorizzata.

### Funzioni di crescita (dopo l'MVP)

- Estensione ai colli chiusi con dati e modulistica propri, come già previsto dal to-be.[doc:turn1doc1]
- Deroga pre-stipula, assicurazione integrativa e casistiche speciali solo dopo conferma normativa e tecnica.
- Inquiry impianto e canoni, variazioni, estinzione, accessi e blocchi come percorsi distinti, già elencati fra le funzioni desiderate dalla Rete.[doc:turn1doc9]

### Visione (futura)

Gestire coerentemente il ciclo di vita della cassetta, con contesto unico e controlli specifici per ogni operazione, senza confondere apertura del contratto e accesso fisico alla cassetta.

---

## Requisiti funzionali

**Legenda:** «to-be» = esplicito nel processo futuro; «proposta» = scelta di prodotto da validare. Campi, formati, testi dei messaggi e mapping tecnici saranno definiti successivamente, senza inventare codici.

### Avvio e verifica

1. **RF-01 (to-be):** avviare «Apertura contratto cassetta» dal menu della Scheda Cliente FrEE con contesto cliente.[doc:turn1doc1]
2. **RF-02 (proposta):** impedire l'avvio a profili non autorizzati o fuori dal cono di visibilità/circolarità, mostrando una motivazione; FrEE colloca tali controlli prima dell'accesso.[doc:turn1doc7]
3. **RF-03 (proposta):** consentire di riprendere pratiche già in corso e applicare la politica di istanze per NDG una volta concordata, senza duplicati silenziosi.[doc:turn1doc7]
4. **RF-04 (proposta):** verificare e registrare l'esito del controllo target market e i prerequisiti del servizio prima della finalizzazione; l'as-is richiede conto corrente e controllo preventivo del gestore.[doc:turn1doc4]

### Dati, cassetta e persone

5. **RF-05 (to-be):** acquisire in una mappa i soli dati necessari alla nuova apertura su cassetta, contratto e assicurazione, mantenendo gli automatismi utili della transazione legacy.[doc:turn1doc1][doc:turn1doc3]
6. **RF-06 (to-be):** precompilare intestatario e cointestatari dai dati della Scheda Cliente, mostrando mancanze e provenienza.[doc:turn1doc1]
7. **RF-07 (proposta):** selezionare solo cassette libere e disponibili ai clienti e ricontrollarne la disponibilità prima di stipulare; la consultazione di altra filiale non autorizza a perfezionare fuori dalla filiale della cassetta.[doc:turn1doc3]
8. **RF-08 (proposta):** validare conto di regolamento appartenente ad almeno un intestatario; non proporre il conto **111111** nell'apertura ordinaria, essendo riservato a eccezioni normate e gestione autorizzata.[doc:turn1doc4]
9. **RF-09 (proposta):** acquisire e riportare scelta di operatività congiunta/disgiunta e rappresentanza pertinente nel riepilogo e nel contratto.[doc:turn1doc3][doc:turn1doc5]
10. **RF-10 (to-be):** collegare il processo facoltativo delegati già gestito in FrEE, trasferire soggetti ed esito al percorso padre e impedire finalizzazione in presenza di una delega richiesta ma incompleta.[doc:turn1doc1]

### Condizioni

11. **RF-11 (to-be):** mostrare condizioni standard applicabili, spese d'apertura e componenti collegate a tipo di cassetta, intestatari e delegati; distinguere ricorrenti, una tantum e assicurazione eventuale.[doc:turn1doc1]
12. **RF-12 (proposta):** recuperare dal sistema titolare il diritto annuo di cointestazione; la circolare indica **€5,16** per ogni cointestatario oltre il primo, **€10,32** per una cassetta intestata a tre soggetti. Non fissare gli importi nel frontend.[doc:turn1doc4]
13. **RF-13 (to-be, condizionato):** prevedere accesso alla deroga DOL solo dopo chiarimento della casistica pre-stipula; rimuovere modifica libera dello sconto da FrEE/B000 nel percorso futuro.[doc:turn1doc1][doc:turn1doc4][doc:turn1doc5]
14. **RF-14 (proposta):** al cambio di cassetta, soggetti, conto o offerta invalidare l'accettazione del vecchio riepilogo e ricalcolare condizioni prima della firma.

### KYC, documenti ed esito

15. **RF-15 (to-be):** collegare il processo KYC FrEE e acquisirne l'esito, senza duplicare la logica specialistica.[doc:turn1doc1]
16. **RF-16 (proposta):** bloccare la stipula senza questionario di adeguata verifica per *ogni* intestatario; verificare i **15 giorni di calendario** del questionario preventivo e collegarne gli identificativi al rapporto.[doc:turn1doc4]
17. **RF-17 (to-be):** riepilogare dati, soggetti, conto, eventuale assicurazione, condizioni definitive e controlli, con azione verso il documentale solo quando i prerequisiti sono soddisfatti.[doc:turn1doc1]
18. **RF-18 (to-be):** produrre e sottoscrivere in uno step di firma i documenti pertinenti; il modulo locazione cassette attuale è **17.006**, l'eventuale copertura integrativa usa **17.007**; composizione e firmatari vanno verificati con il documentale.[doc:turn1doc1][doc:turn1doc4]
19. **RF-19 (proposta):** distinguere documenti generati, firma in corso, firma completata, stipula confermata ed errore da riconciliare; non dichiarare attivo il contratto sulla sola base dell'esito firma.[doc:turn1doc3][doc:turn1doc7]
20. **RF-20 (proposta):** mostrare esito complessivo e sottoprocessi, supportare ripresa dopo sospensione e retry controllato in caso di salvataggio fallito.[doc:turn1doc7]

### Continuità e coerenza

21. **RF-21 (to-be):** scambiare con l'orchestratore avanzamenti e informazioni fra step; il micro frontend mantiene i dati temporanei e almeno un autosalvataggio per sottostep, come richiesto dalle linee guida.[doc:turn1doc1][doc:turn1doc7]
22. **RF-22 (proposta):** alla modifica di dati prima della firma riaprire controlli e documenti dipendenti, evitando la sottoscrizione di una versione superata.
23. **RF-23 (proposta):** permettere uscita, ripresa e annullamento coerente di processo padre e sottoprocessi, senza lasciare rapporto o cassetta falsamente attivi.[doc:turn1doc7]
24. **RF-24 (proposta):** rendere la pratica consultabile nel widget processi; le linee guida richiedono la consultazione di ciascuno step in sola lettura almeno fino a **6 mesi** dalla chiusura, derogabili secondo necessità. Confermare la retention del prodotto.[doc:turn1doc7]

---

## Requisiti non funzionali

### Sicurezza

- Autorizzazione secondo ruolo, visibilità e circolarità, con nuovo controllo in fase di perfezionamento; separazione fra accesso in lettura e conferma da validare.[doc:turn1doc7]
- Minimizzazione, protezione e tracciamento degli accessi ai dati anagrafici, di conto, KYC e firma; politiche concrete di conservazione concordate con i titolari.
- Idempotenza e protezione dalle corse concorrenti sulla stessa cassetta; riconciliazione dei casi «firmato ma non stipulato». Sono requisiti proposti, non capacità già accertate.
- Fruizione da tastiera e con ausili, etichette leggibili, errori comprensibili non comunicati dal solo colore e modalità assistita di sottoscrizione; verificare lo standard di accessibilità aziendale applicabile.
- Audit su identità operatore, versione delle condizioni, documenti e transizioni; nessuna memorizzazione superflua del contenuto KYC nel frontend.

### Integrazioni

- **Scheda Cliente/anagrafica FrEE:** cliente, soggetti, filiale, rapporti e origine/freschezza dei dati.[doc:turn1doc1]
- **B000 ACC/ACD/ACV:** inventario, soggetti, contratto provvisorio e perfezionato; risolvere il punto di consistenza con la firma prima del disegno tecnico.[doc:turn1doc3][doc:turn1doc7]
- **ZC00/DOL:** condizioni e, solo se normativamente ammesso, esito della deroga. La vista **CAS01** riguarda le cassette; **CAS02** riguarda i colli, fuori MVP.[doc:turn1doc4]
- **Delegati FrEE e KYC:** avvio, correlazione pratica, esiti e ritorno; la circolare descrive questionario preventivo GIANOS 3D e collegamento successivo B000, mentre il to-be chiede aggancio al KYC FrEE: mapping da definire.[doc:turn1doc1][doc:turn1doc4]
- **Documentale e orchestratore FrEE:** pacchetto documenti, firmatari, firma unica, notifiche, dati temporanei e pagina d'esito; il micro frontend conserva i dati temporanei, non l'orchestratore.[doc:turn1doc7]
- Prima della specifica funzionale dettagliare per ogni passaggio identificativi di pratica/cliente/cassetta, versione condizioni, esito, timestamp, titolare del retry e gestione timeout; non si assumono API né codici d'errore non documentati.

---

## Prossimi passi

1. **Disegno** — rappresentare dati, condizioni, riepilogo ed esiti secondo le linee guida FREE, dopo conferma del perimetro.
2. **Validazione** — confermare con Rete, B000, ZC00/DOL, KYC, documentale e conformità perimetro cassette/colli, deroga prima della stipula e sequenza firma–attivazione; misurare baseline e testare i punti critici con gestori e clienti.

---

_PRD generato da ARchetipo — 2026-10-05_  
_Sessione condotta da Stefano Marello con il team ARchetipo; le scelte non confermate sono esplicitamente indicate come ipotesi._
