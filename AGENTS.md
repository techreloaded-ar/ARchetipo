# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Progetto

ARchetipo è un set di skill per AI coding agent (Claude Code, Codex, Gemini CLI, OpenCode, GitHub Copilot) che supportano il processo di ideazione, analisi e pianificazione di un progetto software.

## Struttura del repository

```
skills/                  # Skill principali (una dir per skill)
  <skill-name>/
    SKILL.md             # Definizione della skill
    references/          # File di supporto caricati dalla skill
skills-extra/            # Skill extra (stessa struttura)
.archetipo/             # File installati nel progetto target (mirror della struttura target)
  config.yaml            # Template di configurazione per il progetto target
  contracts.md           # Catalogo di tutte le operazioni connector (fonte di verità)
  connectors/
    file.md              # Implementazione connector filesystem
    github.md            # Implementazione connector GitHub Issues/Projects
install.ps1 / install.sh # Installer per i vari tool
```

## Architettura connector

Le skill non gestiscono direttamente la persistenza. Il flusso è sempre:

1. La skill legge `.archetipo/config.yaml` nel **progetto target** per sapere quale connector usare (`file` o `github`)
2. La skill carica `.archetipo/connectors/{connector}.md` dal progetto target
3. La skill esegue le operazioni definite in `.archetipo/contracts.md`

`.archetipo/contracts.md` è la fonte di verità per tutte le operazioni disponibili. Leggerlo prima di modificare skill o connector. Nel **source repo** si trova in `.archetipo/contracts.md`.

## Regole per skill author (da `contracts.md`)

- Chiama solo le operazioni che la skill usa realmente
- I template di contenuto (formato del piano, corpo delle issue, struttura storie) vanno nella skill, non nel connector
- La logica di validazione e post-processing va nella skill
- Le operazioni no-op sono esplicite nel connector: la skill non deve fallire, deve saltare il passo
- Caricare `.archetipo/contracts.md` e il connector file **una sola volta** all'avvio della skill

## Installazione (per utenti finali)

Gli installer (`install.ps1` / `install.sh`) copiano le skill dalla dir `skills/` nelle directory specifiche di ogni tool. Il flag `-Local` installa dalla copia locale invece che da GitHub.

## Note operative

- `.archetipo/config.yaml` in questo repo è un **template**: viene copiato nel progetto target dell'utente sotto `.archetipo/config.yaml`
- Il connector `file` è il default e usa file markdown locali. Il connector `github` richiede `gh` CLI autenticato
- Non esiste ancora un processo di test formale
