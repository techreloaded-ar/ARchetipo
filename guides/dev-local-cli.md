# Sviluppo locale della CLI ARchetipo

Questa guida serve quando vuoi compilare una versione locale di `archetipo`, provarla in un progetto/sandbox e iterare senza pubblicare pacchetti npm.

## Prerequisiti

- Go nella versione indicata da `cli/go.mod`.
- Node.js solo se vuoi testare anche lo shim npm (`npm/archetipo/bin/archetipo.js`).
- Un clone locale del repository ARchetipo.

> Nei comandi sotto, `ARCHETIPO_REPO` punta alla root del clone ARchetipo.

## Percorso rapido: binario Go locale

Da usare per il normale ciclo dev: compila solo il binario della tua piattaforma, non tocca npm e non pubblica nulla.

```bash
cd /path/to/ARchetipo
export ARCHETIPO_REPO="$PWD"

mkdir -p .local/bin
(
  cd cli
  go build \
    -ldflags "-X github.com/techreloaded-ar/ARchetipo/cli/internal/version.Version=dev-local" \
    -o "$ARCHETIPO_REPO/.local/bin/archetipo" \
    ./cmd/archetipo
)

export ARCHETIPO_DATA_DIR="$ARCHETIPO_REPO"
export PATH="$ARCHETIPO_REPO/.local/bin:$PATH"

archetipo version
```

Perché `ARCHETIPO_DATA_DIR` è importante: `archetipo init` deve trovare le skill in `skills/` e gli asset runtime in `.archetipo/`. Lo shim npm lo imposta automaticamente; con un binario locale conviene esportarlo esplicitamente.

### Windows PowerShell

```powershell
cd C:\path\to\ARchetipo
$env:ARCHETIPO_REPO = (Get-Location).Path

New-Item -ItemType Directory -Force .local\bin | Out-Null
Push-Location cli
go build `
  -ldflags "-X github.com/techreloaded-ar/ARchetipo/cli/internal/version.Version=dev-local" `
  -o "$env:ARCHETIPO_REPO\.local\bin\archetipo.exe" `
  .\cmd\archetipo
Pop-Location

$env:ARCHETIPO_DATA_DIR = $env:ARCHETIPO_REPO
$env:PATH = "$env:ARCHETIPO_REPO\.local\bin;$env:PATH"

archetipo version
```

## Smoke test completo in una sandbox

Dopo aver eseguito il percorso rapido e avere `archetipo` nel `PATH`, questi comandi creano un progetto temporaneo, inizializzano ARchetipo con connector `file`, aggiungono una spec, la pianificano e marcano un task come completato.

```bash
SANDBOX="$(mktemp -d)"
cd "$SANDBOX"
git init -b main 2>/dev/null || git init

archetipo init --tool pi --connector file --yes
archetipo doctor

cat > specs.json <<'JSON'
{
  "specs": [
    {
      "code": "US-001",
      "title": "Smoke test locale",
      "priority": "HIGH",
      "points": 1,
      "status": "TODO",
      "epic": { "code": "EP-DEV", "title": "Dev workflow" },
      "body": "## User story\nCome developer voglio verificare la CLI locale.\n\n## Acceptance criteria\n- La spec viene creata e letta dal connector file."
    }
  ]
}
JSON

archetipo spec add --file specs.json
archetipo spec list --status TODO
archetipo spec show US-001

cat > plan.json <<'JSON'
{
  "plan_body": "## Piano\nEseguire uno smoke test della CLI locale.",
  "tasks": [
    { "id": "TASK-01", "title": "Verifica comando locale", "type": "Test", "status": "TODO" }
  ]
}
JSON

archetipo spec plan US-001 --file plan.json
archetipo task done US-001 TASK-01
archetipo metrics
```

## Usare il binario locale dentro un progetto reale

```bash
cd /path/to/progetto-da-testare
export ARCHETIPO_DATA_DIR="/path/to/ARchetipo"
/path/to/ARchetipo/.local/bin/archetipo doctor
/path/to/ARchetipo/.local/bin/archetipo spec list
```

Se vuoi evitare il path assoluto a ogni comando:

```bash
export PATH="/path/to/ARchetipo/.local/bin:$PATH"
archetipo doctor
```

Per controllare di non stare usando la versione pubblicata globalmente:

```bash
which -a archetipo
archetipo version
```

## Loop di sviluppo consigliato

```bash
cd "$ARCHETIPO_REPO"

# 1. modifica codice Go / skill / runtime

# 2. controlli rapidi
(
  cd cli
  gofmt -l .
  go test ./...
)

# 3. ricompila il binario locale
(
  cd cli
  go build \
    -ldflags "-X github.com/techreloaded-ar/ARchetipo/cli/internal/version.Version=dev-local" \
    -o "$ARCHETIPO_REPO/.local/bin/archetipo" \
    ./cmd/archetipo
)

# 4. riprova nel progetto target
cd /path/to/progetto-da-testare
archetipo doctor
```

Prima di consegnare una modifica alla CLI, esegui la suite completa usata in CI:

```bash
cd "$ARCHETIPO_REPO/cli"
gofmt -l .          # deve essere vuoto
go vet ./...
go build ./...
go test ./...
golangci-lint run --timeout 5m ./...
```

## Testare anche lo shim npm, senza pubblicare

Usa questa se devi verificare il comportamento del pacchetto `@techreloaded/archetipo` e del sub-package nativo della piattaforma corrente, cioè simulare quello che ottiene un utente con `npm install -g @techreloaded/archetipo` ma con il codice del repo.

```bash
cd /path/to/ARchetipo
npm run install:dev

archetipo version   # es. 0.0.0-dev.g38acca5
```

Lo script:

- calcola la versione `0.0.0-dev.g<short-sha>` dal commit corrente (con suffisso `.dirty` se ci sono modifiche tracciate non committate), così `archetipo version` dice esattamente da quale build arriva l'installazione globale;
- compila il binario Go della sola piattaforma corrente e prepara i pacchetti npm (shim + skill + runtime risincronizzati dalle sorgenti) in `.dev/npm-staging/`, senza toccare i file tracciati sotto `npm/` — il working tree resta pulito;
- genera i `.tgz` con `npm pack` e li installa con `npm install -g`, verificando alla fine che il binario globale riporti la versione attesa.

Per rimuovere il test globale:

```bash
npm run uninstall:dev
```

Note:

- L'install dev sovrascrive un'eventuale installazione globale stabile di `@techreloaded/archetipo`; per ripristinarla: `npm install -g @techreloaded/archetipo`.
- Se il prefix npm globale è di sistema, `npm install -g` può fallire per permessi: preferisci un prefix utente (nvm/volta) invece di sudo.
- Il notifier di update può segnalare che sul registry esiste una versione "più recente" di `0.0.0-dev.*`: è atteso e innocuo.
- Se hai `.dev/bin` o `.local/bin` nel `PATH` (flusso PATH-based qui sopra), quelli vincono sull'install globale: controlla con `which -a archetipo`.

### Variante release-style con GoReleaser

Se vuoi simulare la pipeline di release locale:

```bash
cd /path/to/ARchetipo
npm run build:cli
npm run build:npm -- 0.0.0-local
```

Nota: `npm run build:npm -- 0.0.0-local` sincronizza asset e versioni dentro `npm/`; può lasciare modifiche nel working tree. Usalo quando vuoi testare il packaging, non per il loop rapido quotidiano.

## Troubleshooting

| Sintomo | Causa probabile | Fix rapido |
|---|---|---|
| `could not locate ARchetipo data directory` | Il binario locale non sa dove trovare `skills/` e runtime. | `export ARCHETIPO_DATA_DIR=/path/to/ARchetipo` |
| `archetipo version` mostra una versione pubblicata | Nel `PATH` vince l'installazione npm globale. | `export PATH=/path/to/ARchetipo/.local/bin:$PATH` e poi `which -a archetipo` |
| Lo shim npm dice che manca il native binary | È stato installato solo il pacchetto principale, non il sub-package piattaforma. | `npm run install:dev` (installa entrambi insieme) |
| `archetipo init` copia skill vecchie | Stai usando asset npm non risincronizzati o `ARCHETIPO_DATA_DIR` punta altrove. | Punta `ARCHETIPO_DATA_DIR` alla root del repo o risincronizza `npm/archetipo/skills` |
| Comandi `github` o `jira` falliscono in sandbox | Mancano credenziali o configurazione esterna. | Per smoke test locali usa `--connector file` |
