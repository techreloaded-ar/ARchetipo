# Template PRD - ARchetipo Inception Copilot Studio

Usare questa struttura esatta per generare il PRD. Tradurre titoli, intestazioni, etichette e frasi statiche nella lingua della conversazione. Mantenere invariati nomi tecnici, identificatori, URL e placeholder interni finche non vengono sostituiti con contenuti reali.

```markdown
# {{PROJECT_NAME}} - Product Requirements Document

**Autore:** ARchetipo
**Data:** {{DATE}}
**Versione:** 1.0

---

## Elevator Pitch

> {{ELEVATOR_PITCH}}
>
> Per **{{TARGET_SEGMENT}}**, che ha il problema di **{{PROBLEM}}**, **{{PRODUCT_NAME}}** e un **{{CATEGORY}}** che **{{KEY_BENEFIT}}**. A differenza di **{{MAIN_ALTERNATIVE}}**, il nostro prodotto **{{DIFFERENTIATOR}}**.

---

## Visione

{{VISION_STATEMENT}}

### Differenziatore di prodotto

{{PRODUCT_DIFFERENTIATOR}}

---

## Personas

### Persona 1: {{PERSONA_1_NAME}}

**Ruolo:** {{ROLE_1}}
**Eta:** {{AGE_1}} | **Background:** {{BACKGROUND_1}}

**Obiettivi:**
{{PERSONA_1_GOALS}}

**Pain point:**
{{PERSONA_1_PAIN_POINTS}}

**Comportamenti e strumenti:**
{{PERSONA_1_BEHAVIORS}}

**Motivazioni:** {{PERSONA_1_MOTIVATIONS}}
**Competenza tecnica:** {{TECH_SAVVINESS_1}}

#### Customer Journey - {{PERSONA_1_NAME}}

| Fase | Azione | Pensiero | Emozione | Opportunita |
|---|---|---|---|---|
| Awareness | {{AWARENESS_1}} | {{AWARENESS_THOUGHT_1}} | {{AWARENESS_EMOTION_1}} | {{AWARENESS_OPPORTUNITY_1}} |
| Consideration | {{CONSIDERATION_1}} | {{CONSIDERATION_THOUGHT_1}} | {{CONSIDERATION_EMOTION_1}} | {{CONSIDERATION_OPPORTUNITY_1}} |
| First Use | {{FIRST_USE_1}} | {{FIRST_USE_THOUGHT_1}} | {{FIRST_USE_EMOTION_1}} | {{FIRST_USE_OPPORTUNITY_1}} |
| Regular Use | {{REGULAR_USE_1}} | {{REGULAR_USE_THOUGHT_1}} | {{REGULAR_USE_EMOTION_1}} | {{REGULAR_USE_OPPORTUNITY_1}} |
| Advocacy | {{ADVOCACY_1}} | {{ADVOCACY_THOUGHT_1}} | {{ADVOCACY_EMOTION_1}} | {{ADVOCACY_OPPORTUNITY_1}} |

---

### Persona 2: {{PERSONA_2_NAME}}

**Ruolo:** {{ROLE_2}}
**Eta:** {{AGE_2}} | **Background:** {{BACKGROUND_2}}

**Obiettivi:**
{{PERSONA_2_GOALS}}

**Pain point:**
{{PERSONA_2_PAIN_POINTS}}

**Comportamenti e strumenti:**
{{PERSONA_2_BEHAVIORS}}

**Motivazioni:** {{PERSONA_2_MOTIVATIONS}}
**Competenza tecnica:** {{TECH_SAVVINESS_2}}

#### Customer Journey - {{PERSONA_2_NAME}}

| Fase | Azione | Pensiero | Emozione | Opportunita |
|---|---|---|---|---|
| Awareness | {{AWARENESS_2}} | {{AWARENESS_THOUGHT_2}} | {{AWARENESS_EMOTION_2}} | {{AWARENESS_OPPORTUNITY_2}} |
| Consideration | {{CONSIDERATION_2}} | {{CONSIDERATION_THOUGHT_2}} | {{CONSIDERATION_EMOTION_2}} | {{CONSIDERATION_OPPORTUNITY_2}} |
| First Use | {{FIRST_USE_2}} | {{FIRST_USE_THOUGHT_2}} | {{FIRST_USE_EMOTION_2}} | {{FIRST_USE_OPPORTUNITY_2}} |
| Regular Use | {{REGULAR_USE_2}} | {{REGULAR_USE_THOUGHT_2}} | {{REGULAR_USE_EMOTION_2}} | {{REGULAR_USE_OPPORTUNITY_2}} |
| Advocacy | {{ADVOCACY_2}} | {{ADVOCACY_THOUGHT_2}} | {{ADVOCACY_EMOTION_2}} | {{ADVOCACY_OPPORTUNITY_2}} |

---

## Insight di brainstorming

> Scoperte principali e direzioni alternative esplorate durante la sessione di inception.

### Riferimenti OneDrive consultati

{{ONEDRIVE_REFERENCES_USED}}

### Progetti simili individuati

{{SIMILAR_PROJECTS_FOUND}}

### Assunzioni sfidate

{{ASSUMPTIONS_CHALLENGED}}

### Nuove direzioni scoperte

{{NEW_DIRECTIONS_DISCOVERED}}

### Assunzioni da validare

{{ASSUMPTIONS_TO_VALIDATE}}

### Rischi principali

{{KEY_RISKS}}

---

## Scope prodotto

### MVP - Minimum Viable Product

{{MVP_SCOPE}}

### Funzionalita di crescita post-MVP

{{GROWTH_FEATURES}}

### Visione futura

{{VISION_FEATURES}}

---

## Architettura tecnica

> **Proposta da:** Leonardo (Architect)

### Architettura di sistema

{{HIGH_LEVEL_ARCHITECTURE}}

**Pattern architetturale:** {{ARCHITECTURE_PATTERN}}

**Componenti principali:**
{{ARCHITECTURE_COMPONENTS}}

### Technology stack

| Layer | Tecnologia | Versione | Razionale |
|---|---|---|---|
| Linguaggio | {{LANGUAGE}} | {{LANGUAGE_VERSION}} | {{LANGUAGE_RATIONALE}} |
| Backend framework | {{BACKEND_FRAMEWORK}} | {{BACKEND_VERSION}} | {{BACKEND_RATIONALE}} |
| Frontend framework | {{FRONTEND_FRAMEWORK}} | {{FRONTEND_VERSION}} | {{FRONTEND_RATIONALE}} |
| Database | {{DATABASE}} | {{DB_VERSION}} | {{DB_RATIONALE}} |
| ORM | {{ORM}} | {{ORM_VERSION}} | {{ORM_RATIONALE}} |
| Auth | {{AUTH_LIB}} | {{AUTH_VERSION}} | {{AUTH_RATIONALE}} |
| Testing | {{TESTING_FRAMEWORK}} | {{TESTING_VERSION}} | {{TESTING_RATIONALE}} |

### Struttura progetto

**Pattern organizzativo:** {{CODE_ORGANIZATION_PATTERN}}

```text
{{DIRECTORY_LAYOUT}}
```

### Ambiente di sviluppo

{{DEVELOPMENT_ENVIRONMENT}}

**Strumenti richiesti:** {{REQUIRED_DEV_TOOLS}}

### CI/CD e deployment

**Build tool:** {{BUILD_TOOL}}

**Pipeline:** {{BUILD_PIPELINE}}

**Deployment:** {{DEPLOYMENT_STRATEGY}}

**Infrastruttura target:** {{TARGET_INFRASTRUCTURE}}

### Architecture Decision Records (ADR)

{{ARCHITECTURE_DECISIONS}}

---

## Requisiti funzionali

{{FUNCTIONAL_REQUIREMENTS}}

---

## Requisiti non funzionali

### Sicurezza

{{SECURITY_REQUIREMENTS}}

### Integrazioni

{{INTEGRATION_REQUIREMENTS}}

---

## Prossimi passi

1. **Backlog** - Trasformare questo PRD in backlog, epiche e user story.
2. **Design** - Produrre mockup o prototipi UI quando applicabile.
3. **Validazione** - Rivedere con stakeholder e testare le assunzioni piu rischiose.

---

_PRD generato con ARchetipo Product Inception - {{DATE}}_
_Sessione condotta da {{USER_NAME}} con il team ARchetipo_
```
