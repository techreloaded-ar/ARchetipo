# AIRchetipo

AIRchetipo è un set di skill per AI coding agent che supportano il processo di ideazione, analisi e pianificazione di un progetto software.

### Skill incluse

- **airchetipo-inception** — Facilitazione interattiva per la definizione del prodotto e generazione del PRD
- **airchetipo-backlog** — Generazione del backlog a partire dal PRD
- **airchetipo-design** — Progettazione di interfacce frontend distintive a partire dal PRD
- **airchetipo-plan** — Pianificazione tecnica delle user story con architettura, task breakdown e test strategy
- **airchetipo-implement** — Implementazione guidata delle user story a partire dal piano tecnico
- **airchetipo-autopilot** — Esecuzione autonoma del ciclo plan → implement su tutte le storie del backlog

**Extra Skills**
- **airchetipo-figma-make** — Generazione di prompt strutturati per Figma Make a partire dal PRD
- **airchetipo-vibe-kanban** — Gestione issue su Vibe Kanban a partire dal backlog


### Tool supportati

- Claude Code
- Codex
- Gemini CLI
- OpenCode
- GitHub Copilot

---

## Installazione

### Prerequisiti

- **macOS**: `curl` e `unzip` (presenti di default)
- **Windows**: PowerShell 5.1+

### macOS / Linux

Apri il terminale, posizionati nella directory del tuo progetto e lancia:

```bash
curl -fsSL https://raw.githubusercontent.com/techreloaded-ar/AIRchetipo/main/install.sh | bash
```

L'installer scarica le skill da GitHub e mostra un menu interattivo per selezionare i tool su cui installarle.

### Windows

Apri PowerShell, posizionati nella directory del tuo progetto e lancia:

```powershell
irm https://raw.githubusercontent.com/techreloaded-ar/AIRchetipo/main/install.ps1 | iex
```

