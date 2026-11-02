# Collaudo MVP: lavoro sul prodotto dalla chat

Data di avvio: 29 ottobre 2026. Branch: `codex/chat-first-workspace`.

## Ipotesi da validare

Una persona descrive un problema, prova un risultato e lo modifica nella stessa
chat, senza coordinare manualmente agenti, worktree e servizi. Il risultato da
consegnare è un commit con specifica e prove originali; riuscita tecnica e valore
business restano osservazioni diverse.

## Installazione e metodo

RideAtlas e digital_twin sono i progetti di prova autorizzati. Rimuovere prima
le installazioni ARchetipo precedenti, conversazioni, ricette, helper e skill
gestite; ripartire con il pacchetto della branch, provider Codex reale e backlog
file. Non riusare configurazioni incompatibili. Conservare i sorgenti applicativi
e i dati esistenti; i servizi del collaudo usano dati sintetici locali.

Interagire attraverso il viewer come utente. Le API e il filesystem sono oracoli
di verifica e strumenti di diagnosi, non scorciatoie per produrre il risultato
che dovrebbe produrre l'agente. Ogni aiuto tecnico manuale conta come attrito.
Annotare versione installata, richiesta esatta, sessione, incremento, commit,
evidenza, tempi e interventi. Le correzioni al prodotto richiedono regressione
mirata, reinstallazione e ripetizione del passaggio fallito.

## Scenari e oracoli

| ID | Percorso | Risultato osservabile |
| --- | --- | --- |
| M01 | Primo avvio Digital Twin senza ricetta: apri app attuale | Scoperta Python reale, setup qualificato, preview utilizzabile, nessun incremento implicito |
| M02 | Digital Twin: esportazione di un riepilogo dei risultati di simulazione | Scoperta proporzionata, specifica persistita, implementazione e test nel worktree; download corrispondente ai dati mostrati |
| M03 | Feedback Digital Twin: nome del CSV riconoscibile dall'azienda | Stessa sessione e worktree, candidato precedente revocato, dati conservati, prove rinnovate |
| M04 | RideAtlas: pagina pubblica di preparazione a un'uscita in bici | Primo messaggio crea lavoro; agente configura Next senza usare servizi reali; checklist interattiva, accessibile e verificata |
| M05 | RideAtlas: feedback sulla checklist e reload | Selezioni conservate secondo la specifica; nessun reset involontario; preview stabile e stato comprensibile |
| M06 | Arresto, ripresa, storico e dossier | Stop libera le porte possedute; reload non riavvia; sessione durevole e dossier coerente con commit e prove |
| M07 | Rifiuto e recupero di setup/check | Errore spiegato, risposta o bozza conservata, nessun falso ready; ripresa verificabile |
| M08 | Gate di consegna e regressione automatica | Nessuna PR senza approvazione del commit; smoke credential-free verdi e richiesti in CI |

Per i dettagli degli incrementi rispondere come un utente con esempi e priorità,
senza prescrivere i comandi della ricetta. Non usare credenziali o dati reali
dei progetti per far apparire riuscita una demo. La pubblicazione esterna non è
necessaria per validare questo ciclo: la protezione e l'idempotenza della PR
sono coperte dal test con hosting simulato; una PR reale richiede la decisione
esplicita sul candidato visualizzato.

## Gate di accettazione

- Nessun errore bloccante irrisolto nei percorsi eseguiti; zero perdita di dati,
  contaminazione fra worktree o successo dichiarato con verifiche fallite.
- Preview realmente utilizzabile su entrambi gli stack; comportamento richiesto
  verificato nell'app, oltre alla lettura dei report dell'agente.
- Ogni candidato richiede tutti i check dichiarati con prove correnti e integre.
- Feedback e reload conservano identità, requisiti, dati e bozze; vecchi errori
  recuperati non devono sembrare blocchi attivi.
- Le verifiche Go, frontend, browser e ambiente previste dal repository passano.
- Esiti `pass`, `fail`, `blocked` e `not_run` sono distinti. Un tentativo o un
  test simulato non vengono descritti come validazione con modello reale.

Metriche: tempo fino alla prima app utilizzabile, numero di interventi tecnici,
domande ridondanti, passaggi senza indicazione chiara, durata di una revisione,
identità delle sessioni prima/dopo ripresa, integrità dei dati e porte dopo stop.
I tempi includono il modello solo nei percorsi reali; i benchmark CLI restano
misure separate. Questo collaudo valuta fattibilità e attrito, non dimostra da
solo domanda di mercato o usabilità con persone esterne.

## Evidenze

Gli artefatti locali del run sono in
`test/workspaces/mvp-acceptance/2026-10-29/` (ignorato da Git). Il resoconto finale
deve riportare anche i limiti e i percorsi non conclusi, senza dichiarare pronto
alla produzione un candidato con gate ancora aperti.

Esiti del primo ciclo: [resoconto del collaudo](mvp-production-acceptance-results.md).
