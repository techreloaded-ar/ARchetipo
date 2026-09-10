# Sessioni native — note di rilascio e migrazione

Rilascio del cambio di direzione descritto in [`docs/wiki/decisions/native-harness-sessions.md`](../wiki/decisions/native-harness-sessions.md): le conversazioni di `archetipo view` diventano sessioni durevoli del vero harness di sviluppo.

Questo documento dice tre cose: che cosa cambia per chi usa ARchetipo, che cosa succede ai dati che esistono già, e che cosa **non** è incluso in questo rilascio.

## Versionamento

**Nessuna operazione CLI pubblica è cambiata in modo incompatibile.** Le 14 operazioni restano quelle che erano: firme, envelope e `error.code` sono invariati, la suite di conformance dei connector è verde, e nessun comando è stato aggiunto o rimosso. La direzione riguarda `archetipo view` — un percorso HTTP locale, non la superficie CLI — e gli adapter dei provider di esecuzione.

Il rilascio è quindi **minor**, non major, e non richiede la policy di breaking change di `AGENTS.md`. Non è stato applicato nessun tag e non è stato eseguito nessun publish o deploy: la pubblicazione resta un gesto separato (`npm run build:npm`, poi `npm run publish:npm`, dopo un tag `v*`).

Una sola cosa cambia nel **layout del repository**, ed è interna al packaging: il template di configurazione è ora `.archetipo/config.template.yaml`, mentre `.archetipo/config.yaml` è la configurazione viva del workspace di questo repository e non viene spedita. Il pacchetto npm continua a chiamarlo `runtime/config.yaml`, quindi nulla cambia per chi installa. Chi lavora sul repository e usa una CLI compilata da sorgente prima di questo cambiamento deve ricompilarla: le vecchie build installavano il file sbagliato.

## Che cosa cambia per chi usa View

- Una conversazione **non scade più**: nessun timeout globale, nessun timeout di inattività. Chiudere la tab non interrompe il lavoro.
- Una conversazione **torna dopo un riavvio di View**, con lo stesso identificativo e la stessa sessione nativa dell'harness. L'agente ricorda: il contesto è suo e viene ripreso per riferimento, non riassunto.
- L'agente **può agire** — modificare file, usare i tool, avviare processi, invocare skill — sotto la permission policy nativa dell'harness, le cui domande arrivano come approval nel pannello. Non esiste più il divieto di agire né l'obbligo di proporre ogni passo.
- **Modello ed effort si scelgono per turno**, dal catalogo che il runtime della sessione dichiara, e la scelta non riscrive il default del workspace.
- Le **skill offerte sono quelle del runtime**, non più una scansione delle directory dei provider.
- I **passi di processo lavorano nel thread aperto**: `plan`, `implement`, `review`, inception, backlog e bozza di spec avviano un turn nella conversazione invece di un secondo agente. La conversazione resta aperta durante e dopo.
- Un workspace può tenere **quante conversazioni si vuole** contemporaneamente. Il limite di tre che esisteva prima è stato rimosso.
- **Fermare, archiviare ed eliminare** sono tre gesti distinti, con tre rotte distinte, e la risposta dice quale è stato fatto.

## Migrazione dei dati esistenti

**Non serve fare nulla, e nulla viene distrutto.**

- I record di conversazione precedenti restano leggibili. Il record è versionato: `version: 1` è il vecchio formato, `version: 2` quello che porta metadati di sessione, stati separati e timeline. Un record vecchio viene letto senza essere riscritto.
- La timeline di una conversazione nuova vive in un file affiancato, `<id>.events.jsonl`, append-only e paginabile. Un record vecchio che porta i propri eventi nel JSON continua a essere ricomposto come prima.
- Una conversazione precedente **non ha un riferimento nativo**, perché quando è stata scritta le sessioni native non esistevano. Non può quindi essere ripresa nativamente: l'API e l'indice lo dichiarano esplicitamente (`legacy_limit`). Ciò che si può fare è aprire una conversazione nuova **seminata** con il suo transcript — un gesto esplicito, che dichiara da quale conversazione proviene, e non il resume normale.
- Le execution già registrate sotto `.archetipo/executions/` restano valide e interrogabili. Il percorso batch `archetipo execution run` non è cambiato.
- `.archetipo/config.yaml` non viene riscritto dall'aggiornamento. Le scelte di modello fatte dentro una conversazione non lo toccano mai.
- Eliminare una conversazione da View rimuove i dati di ARchetipo — record e timeline — e la risposta lo dichiara con `native_session_preserved: true`. Il transcript nativo dell'harness e i file di lavoro non vengono toccati: se li vuoi rimuovere, è un gesto tuo, con gli strumenti dell'harness.

## Che cosa non è incluso

- **Il percorso remoto.** Il provider `arcipelago` non tiene sessioni native: continua a eseguire `spec.plan` e `spec.implement` come run batch, con il proprio pannello run. I task 10 e 11 del piano — hub runner e adapter — non sono consegnati. Una conversazione su un workspace il cui default è `arcipelago` usa ancora il percorso legacy.
- **La sopravvivenza garantita di un job allo spegnimento di View.** Che un server avviato dall'agente resti in vita dipende dall'harness, e i due misurati non si comportano allo stesso modo. Non è promesso che sopravviva; è promesso che la conversazione torni e che da lì il servizio si possa verificare, fermare o riavviare.
- **Il possesso attraverso la rete.** Il lock che impedisce a due viewer di scrivere sulla stessa sessione nativa si basa sul pid: vale per due processi sulla stessa macchina, non per due macchine che montano lo stesso workspace via rete.
- **L'input strutturato di Claude.** Il runtime provato non lo espone: le sue domande arrivano come testo. La capability `turn.input` è dichiarata soltanto da Codex.

La matrice completa delle capacità misurate per provider, con le prove osservate, è in [`docs/plans/sessioni-native/accettazione.md`](../plans/sessioni-native/accettazione.md).
