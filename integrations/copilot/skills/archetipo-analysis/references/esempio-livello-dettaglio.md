# Level of detail — two calibrated extracts

Source: "Anagrafe WEB — Descrizione intervento" (Censimento / Variazione Persona Fisica), the house reference for functional analysis documents. The two extracts below are adapted from it and reduced to what shows the expected granularity. They are **examples of density and form**, not content to reuse: lengths, codes, tables and messages of the product you are analysing come from the Knowledge, the PRD or the user — never from here.

Read this file once, before generating the first section, and compare your last section against it before delivering.

---

## Extract 1 — one field, fully described

This is how a **single field** looks in the target document. Notice: one row, seven cells, every cell filled; the length is a number when known and `XX crt` when it is not; every control has a severity and a verbatim message; the source of the prefilled value is a named system; the conditional rule names the other field it depends on.

### Videata "Dati anagrafici" — campo COD. FISC.

| Nome | Formato e lunghezza | Obbligatorietà | Prevalorizzazione e fonte | Dominio | Regole condizionali | Controlli e messaggio |
|---|---|---|---|---|---|---|
| COD. FISC. | Alfanumerico 16 crt (persona fisica); numerico 11 crt (soggetto con partita IVA) | Obbligatorio. Opzionale solo se **RESIDENZA FISCALE** ≠ Italia e **TIPO SOGGETTO** = Non residente | Da Anagrafe centrale (NDG) in variazione; vuoto in censimento | Codice fiscale rilasciato dall'Agenzia delle Entrate | Se **TIPO SOGGETTO** = Persona fisica, il formato ammesso è solo 16 crt. Se **RESIDENZA FISCALE** ≠ Italia, in assenza del codice si compila **CODICE IDENTIFICATIVO ESTERO** (obbligatorio in alternativa) | C1 Controllo formale (struttura e carattere di controllo) → **Bloccante** «Codice fiscale formalmente errato. Verificare il valore inserito.» · C2 Coerenza con COGNOME, NOME, DATA DI NASCITA, SESSO, COMUNE DI NASCITA → **Forzabile** con motivazione obbligatoria (min 10 crt), imposta il flag *CF non coerente* sull'NDG e genera notizia per la funzione Antiriciclaggio «Il codice fiscale non è coerente con i dati anagrafici inseriti. Confermare per proseguire indicando la motivazione.» · C3 Unicità sull'Anagrafe centrale → **Bloccante** «Codice fiscale già presente sull'NDG {{NDG}}. Procedere con la variazione del soggetto esistente.» con pulsante «Apri NDG» che porta alla videata di variazione |

**Casi particolari collegati al campo**

- Cittadino italiano nato all'estero: il controllo C2 usa il codice catastale dello Stato estero (tabella `TSTATI`), non un comune.
- Omocodia: il controllo C1 accetta i caratteri sostitutivi (L, M, N, P, Q, R, S, T, U, V) nelle posizioni numeriche; C3 resta bloccante.
- Variazione di COGNOME o DATA DI NASCITA su un NDG esistente: C2 viene rieseguito al salvataggio anche se il campo COD. FISC. non è stato toccato.

**Punti aperti della riga**

| N | Punto | Assunzione adottata | Impatto se diversa |
|---|---|---|---|
| PA-3.2-4 | Lunghezza minima della motivazione per forzare C2 `[DA CONFERMARE]` | 10 crt, come nelle altre forzature dell'applicazione | Solo testo di validazione |

---

## Extract 2 — one integration, every outcome handled

This is how an **integration paragraph** looks in the target document. Notice: when it is called, with which fields, and a row per outcome with a behaviour and a message; the "non disponibile / timeout" outcome is always there; the effect on the practice (flags, blocks, notes) is written next to each outcome, not in a generic sentence.

### Paragrafo 3.4.6 — Integrazione SCIPAFI (verifica documento di identità)

**Quando viene chiamato.** Al pulsante «Conferma» della videata "Documento di identità", dopo il superamento dei controlli formali sui campi TIPO DOCUMENTO, NUMERO DOCUMENTO, ENTE DI RILASCIO, DATA RILASCIO, DATA SCADENZA. Non viene chiamato se TIPO DOCUMENTO ∈ {Permesso di soggiorno, Passaporto estero}: per questi la verifica è manuale (vedi 3.4.7).

**Input.** COD. FISC., TIPO DOCUMENTO (codificato secondo tabella `TDOCSCIPAFI`), NUMERO DOCUMENTO, DATA RILASCIO, ENTE DI RILASCIO.

| Esito | Condizione restituita dal servizio | Comportamento della videata | Effetti sul sistema | Messaggio |
|---|---|---|---|---|
| Positivo | Documento riscontrato, dati coerenti | Si prosegue alla videata "Recapiti" | Flag *Documento verificato SCIPAFI* = S sull'NDG; data e ora verifica salvate; esito loggato con id transazione | Nessun messaggio; banner verde «Documento verificato» per 5 secondi |
| Negativo — documento non riscontrato | Numero documento non presente negli archivi del Ministero | Si resta sulla videata; i campi del documento tornano editabili | Flag *Documento verificato SCIPAFI* = N; nessun blocco | **Forzabile** con motivazione obbligatoria e flag *Verifica manuale documento*: «Il documento non risulta negli archivi consultati. Verificare i dati inseriti oppure confermare la verifica manuale indicando la motivazione.» |
| Negativo — dati incoerenti | Documento riscontrato ma titolare diverso | Si resta sulla videata; campi editabili | Flag *Alert SCIPAFI* = S sull'NDG; notizia automatica alla funzione Antifrode; la pratica non può essere chiusa finché la notizia non è evasa | **Bloccante** «I dati del documento non corrispondono al titolare. Non è possibile proseguire: la posizione è stata segnalata alla funzione competente.» |
| Documento non verificabile | Tipologia non coperta dal servizio | Si prosegue alla videata "Recapiti" | Flag *Documento verificato SCIPAFI* = X (non verificabile); nessuna notizia | Banner giallo «Tipologia di documento non verificabile automaticamente. Verifica manuale richiesta.» |
| Non disponibile / timeout | Nessuna risposta entro XX secondi `[DA CONFERMARE]` oppure errore tecnico | Si resta sulla videata con i campi non editabili; pulsante «Riprova» e pulsante «Prosegui senza verifica» | Se l'utente prosegue: flag *Documento verificato SCIPAFI* = D (differita) e la verifica viene rieseguita in batch notturno; esito batch negativo genera notizia | Banner rosso «Servizio di verifica documenti momentaneamente non disponibile. Puoi riprovare oppure proseguire: la verifica sarà completata in differita.» |

**Casi particolari**

- Nuovo documento inserito in variazione su NDG con *Alert SCIPAFI* = S ancora aperto: la chiamata viene eseguita ugualmente; un esito positivo **non** chiude l'alert, che resta di competenza Antifrode.
- Cointestatari: la verifica è per soggetto; l'esito di uno non condiziona la videata degli altri.

**Punti aperti del paragrafo**

| N | Punto | Assunzione adottata | Impatto se diversa |
|---|---|---|---|
| PA-3.4-2 | Timeout del servizio `[DA CONFERMARE]` | Parametro applicativo, valore da definire con il gestore del servizio | Solo configurazione |
| PA-3.4-3 | Ammissibilità di «Prosegui senza verifica» per il profilo Operatore di sportello `[DA CONFERMARE]` | Ammesso per tutti i profili abilitati alla videata | Se riservato a un profilo superiore, serve un controllo di abilitazione sul pulsante |

---

## What to take from these extracts

- **Granularity is the row.** The unit of the document is one field, one control, one outcome. If you find yourself writing "i controlli standard dell'anagrafe" or "gestione errori come da linee guida", stop and write the rows.
- **Severity and trace go together.** A control is never just "bloccante": say what the user sees and what the system records. A forceable control says how it is forced.
- **Every outcome has a home.** Positive, each negative reason, not verifiable, not available. If the Knowledge does not list the outcomes, list the ones the PRD implies plus "Non disponibile / timeout", and mark the others `[DA CONFERMARE]`.
- **Honest gaps are content.** `XX crt` and `[DA CONFERMARE]` are part of the house style: the reviewer looks for them. A guessed number is a defect; a marked gap is a question well asked.
