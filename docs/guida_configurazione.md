# 🚀 Guida alla Configurazione dell'Ambiente di Sviluppo

Questa guida illustra i passaggi necessari per configurare l'ambiente di
sviluppo, installare le dipendenze essenziali e configurare l'IDE
OpenCode con il plugin di autenticazione OpenAI.

## 1. 🌳 Installazione di Node.js (LTS)

Node.js è essenziale e deve essere installato per eseguire gli script di
progetto e le dipendenze. Si raccomanda la versione LTS (Long-Term
Support).

### macOS e Windows

Scarica e installa l'ultima versione LTS dal sito ufficiale di Node.js.

Verifica l'installazione aprendo il terminale (o Prompt dei
comandi/PowerShell su Windows):

    node -v
    npm -v

## 2. 📝 Installazione di OpenCode


### Installazione

Scarica e installa la versione adatta al tuo sistema operativo (macOS o
Windows) dal [sito ufficiale di OpenCode](https://opencode.ai/docs/#install).

### Avvio

Per avviare l'applicazione da qualsiasi posizione nel terminale:

    opencode

## 3. 🔑 Configurazione di OpenCode con Plugin OpenAI OAuth

L'installazione del plugin avviene tramite la configurazione, e l'IDE lo
scaricherà e lo installerà automaticamente.

### 3.1. Installazione

Apri il file di configurazione globale di OpenCode (solitamente
`~/.config/opencode/opencode.json`  e segui le istruzioni fornite sulla [documentazione ufficiale](https://github.com/numman-ali/opencode-openai-codex-auth?tab=readme-ov-file#installation).


Al primo avvio, OpenCode scaricherà e installerà automaticamente il
plugin.

**Nota:** Se desideri abilitare tutte le varianti di ragionamento (Low,
Medium, High) per Codex, consulta la sezione "Recommended: Full
Configuration" nel README ufficiale del plugin.

### 3.2. Autenticazione (Login OAuth)

Esegui questo comando nel tuo terminale per avviare la procedura di
autenticazione OAuth:

    opencode auth login

Seleziona **OpenAI** quando richiesto.

Verrà aperto automaticamente il tuo browser predefinito per il flusso di
autenticazione. Segui le istruzioni per autorizzare l'accesso.

Dopo l'autenticazione, il plugin sarà operativo e connesso al tuo
account OpenAI.

### 3.3. Selezione del Modello

Esegui `Ctrl + x` per aprire il menù di selezione del modello

Scegli l'opzione **"OpenAI GPT 5.1 Codex Medium (OAuth)"**.



## 4. 🗃️ Inizializzazione del Repository Git

Per iniziare il controllo versione del tuo progetto:

### 4.1. Crea e Inizializza il Progetto

Apri il terminale, crea la cartella di progetto e inizializza Git:

    # Crea e vai alla cartella di progetto
    mkdir IlMioProgetto
    cd IlMioProgetto

    # Inizializza un repository Git locale
    git init

### 4.2. Configurazione del Repository Remoto (Opzionale)

Per collegare il progetto a un repository remoto (es. su GitHub):

    # Collega il repository locale a quello remoto (sostituisci l'URL)
    git remote add origin <URL_DEL_TUO_REPO>

    # Rinomina il branch principale
    git branch -M main

## 5. ▶️ Comandi per Iniziare a Sviluppare

Per aprire il progetto e iniziare a lavorare con OpenCode:

    # Apri la cartella di progetto nell'IDE OpenCode
    opencode .

Il plugin OpenAI Codex sarà attivo e pronto per l'assistenza al codice.
