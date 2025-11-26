# Web Bundler Usage

ALPHA NOTE: Bundling of individual agents might work, team bundling is being reworked and will come with Beta release soon.

The web bundler creates self-contained XML bundles for AIRchetipo agents, packaging all dependencies for web deployment.

## Quick Start

```bash
# Bundle all agents from all modules
npm run bundle

# Clean and rebundle (removes old bundles first)
npm run rebundle
```

## Custom Output Directory

```bash
# Bundle to custom directory
node tools/cli/bundlers/bundle-web.js all --output ./my-bundles

# Rebundle to custom directory (auto-cleans first)
node tools/cli/bundlers/bundle-web.js rebundle --output /absolute/path/to/custom/directory

# Bundle specific module to custom directory
node tools/cli/bundlers/bundle-web.js module aim --output ./custom-folder

# Bundle specific agent to custom directory
node tools/cli/bundlers/bundle-web.js agent aim analyst -o ./custom-folder
```

## Output

Bundles are generated in `web-bundles/` directory by default when run from the root of the clones project:

```
web-bundles/
├── [module-name]/
│   └── agents/
│       └── [agent-name].xml
```

## Skipping Agents

Agents with `bundle="false"` attribute are automatically skipped during bundling.

## Bundle Contents

Bundles are generated in `web-bundles/` directory by default when run from the root of the clones project:

```
web-bundles/
├── [module-name]/
│   └── agents/
│       └── [agent-name].xml
```

## Skipping Agents

Agents with `bundle="false"` attribute are automatically skipped during bundling.

## Bundle Contents

Each bundle includes:

- Agent definition with web activation
- All resolved dependencies
- Manifests for agent/team discovery

## Limitare i workflow inclusi nei bundle di team

Quando un team web deve supportare solo uno specifico workflow (ad esempio `party-mode`), è possibile alleggerire il pacchetto
aggiungendo alla configurazione del team la lista `bundle.allowed_workflows`. Solo i workflow elencati vengono vendorizzati e i menu
che puntano a workflow esclusi vengono rimossi automaticamente dal bundle, riducendo dimensioni e dipendenze.

Esempio:

```yaml
bundle:
  name: Team Product Inception
  icon: "🚀"
  allowed_workflows:
    - air/core/workflows/party-mode/workflow.yaml
```

In questo modo il bundle contiene solo gli asset necessari per eseguire Party Mode mantenendo invariati persona e comportamento degli
agenti coinvolti.

