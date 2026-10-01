# ARchetipo on Copilot Studio

Run the first three phases of the ARchetipo method — **Inception**, **Design** and **Analysis** — as a single agent in [Microsoft Copilot Studio](https://copilotstudio.microsoft.com/), powered by the GitHub Copilot harness. Everything the agent produces is handed back as a file attached to its reply, to download from the conversation.

There is no CLI here. The `archetipo` binary is not invocable inside Copilot Studio, so this integration does not use it and does not use connectors. It does not use SharePoint or OneDrive either, and needs no MCP server at all: the agent creates a file in its sandbox and the harness attaches it to the reply as a *created file* card with a Download action (documented in [Files the agent creates](https://learn.microsoft.com/en-us/microsoft-copilot-studio/agents-experience/created-files-overview)). The agent never stores a file anywhere and never says it did.

## Responsibilities

| Component | Owns |
|---|---|
| ARchetipo | The product method: the discovery flow, the personas, the PRD structure, the design discipline, the analysis grammar |
| Copilot Studio | Execution: the conversation, the orchestration runtime, skill activation, file delivery, the Knowledge |
| The user | Persistence: downloading the artifacts, and attaching the PRD (and the analysis) back when work resumes later |

There is no storage behind the agent. Nothing survives a conversation except what the user downloaded — files stay downloadable inside the conversation for 28 days from its last activity, and are never reachable from another one.

## What you install

Four things, all by hand in the Copilot Studio UI:

```text
instructions.md                            -> Build > Instructions
skills/archetipo-inception/SKILL.md        -> Build > Skills > Upload a skill
skills/archetipo-design/archetipo-design.zip     -> Build > Skills > Upload a skill (packaged skill bundle)
skills/archetipo-analysis/archetipo-analysis.zip -> Build > Skills > Upload a skill (packaged skill bundle)
```

No tools, no MCP server. Check **Build > Tools** is empty: a SharePoint or OneDrive tool left on the agent is what makes it "save" files instead of handing them over, and the instructions forbid using one even if present. A skill that is a single `SKILL.md` is uploaded as is. A skill with a `references/` folder is uploaded as a `.zip` with `SKILL.md` at the root and `references/` next to it (a "packaged skill bundle"); build it from inside the skill folder, so the archive has no extra top-level directory.

The Knowledge holds two corpora: the FREE UX Guidelines kit (19 files) for design, and the as-is documentation of the existing products for analysis. Upload both under **Knowledge**; the agent instructions say how each one is searched.

## Where persistence lives

In one place: the **Persistence** section of `instructions.md`. It defines the delivery model — the agent attaches files to its replies and stores nothing — the procedures that touch it — *Locate the product*, *Read an artifact*, *Deliver an artifact*, *Replace an artifact*, *Hand over* — and the limits that follow from it.

Nothing else knows how artifacts reach the user. The skills call those procedures by name and never mention downloads, attachments or a working directory. Backing this agent with a document library, a drive or a repository instead is a rewrite of that one section: the method, the PRD template and the design discipline stay untouched.

## Setup

1. **Create the agent.** In Copilot Studio, create a new agent using the **GitHub Copilot harness** — the harness is what gives the agent a sandbox to create files in and the *created file* card that hands them to the user.

2. **Paste the instructions.** Copy `instructions.md` into **Build → Instructions**.

3. **Upload the three skills** and save. The **Preview** tab runs the latest saved draft — publishing is only needed to expose the agent in a channel.

4. **Load the Knowledge.** The FREE kit for design; the as-is functional documentation of the products for analysis. Then probe the second corpus with three searches at field level ("as-is conto di addebito campi", "as-is KYC validità questionario", "as-is cassette disponibilità"): if the answers do not reach fields and messages, the analysis will get its detail from questions to the user instead, and the team should know it before the first run.

5. **Check the round trips once.** Ask for a minimal mockup and confirm the single `.html` comes back as a created-file card under the reply; ask for a minimal Word document and confirm the `.docx` comes back the same way, and that the reply does not claim the file was saved anywhere; ask a design question that only `references/free-checklist.md` answers, to confirm the zipped skill reads its references. If a file is only echoed as text, the harness is not surfacing created files and this integration will not work as designed.

## How work is delivered today

```text
PRD.md                    the product definition
Analisi-Funzionale.docx   the functional analysis
onboarding.html           one mockup, every screen inside it
```

Fixed file names, no folders: a created-file card shows a file name, nothing else, and the product name lives in the title or header of each document. When one conversation covers several products the agent says in the reply which product each file belongs to.

A mockup is a single file: every screen lives inside it and the file switches between them, because a link from one download to another does not resolve. How many screens it holds is the user's call at the start — **Minimal**, **Standard** or **Complete**.

A functional analysis is one Word file, `Analisi-Funzionale.docx`: the deliverable the customer reviews in Word and the file the user attaches back for a revision. It is regenerated whole at every revision; there is no patching. The analysis is produced after a question phase — at most three questions per turn, until the open points are exhausted or the user says to proceed — and then generated in a single turn, with its own quality gate, which now also checks that headings and tables survived the trip into Word.

## How resuming works

By re-attaching. Created files belong to the conversation that produced them, so "let's do the design of Shopper", said weeks later in a brand-new conversation, needs the `PRD.md` from that inception attached to the message. The agent reads it as its brief and designs from it. The same `PRD.md` is what the analysis starts from.

To revise an analysis, attach both `PRD.md` and `Analisi-Funzionale.docx`: the harness extracts the text of Word attachments (documented in [Attach files to a conversation](https://learn.microsoft.com/en-us/microsoft-copilot-studio/agents-experience/attachments-overview)), and the agent re-reads the header, the process map and the open points inside the analysis and asks what changes. How faithfully the tables of a long Word file come back through that extraction is not documented: verify it once with a real analysis before relying on revisions.

If the user does not have the PRD any more, the agent offers a short inception rather than inventing the product.

## Known constraints

Stated plainly, because they shape what this integration can and cannot do:

- **Persistence is the conversation's, and it expires.** The artifacts stay downloadable inside the conversation that produced them for 28 days from its last activity, then they are gone; they are never visible from another conversation. The agent says so at every hand-over, but nothing enforces the download.
- **10 MB per file.** That is the hard limit of the harness. Mockups stay far below it by design; a complete functional analysis in Word is larger than a mockup but still fits comfortably.
- **One model per agent.** The model is chosen once, in Build > Model, for every skill of the agent. If the analysis needs a stronger model than inception and design, it is a trade-off for the whole agent — or a reason to move the analysis to a separate agent connected to this one.
- **No cross-conversation continuity.** No manifest, no index, no lookup of past products. Continuity is the attached PRD (and, for a revision, the attached analysis), and nothing else.
- **Mockups are downloads, not pages.** Nothing renders HTML here, and links between downloaded files do not resolve, which is why a mockup is one self-contained file with its screens and its own switcher inside, and never a folder with an `index.html` hub.
- **Attachment support is the hard dependency.** Reading an attached `PRD.md` back is what makes Design-after-Inception possible; a channel that strips attachments limits the agent to single-conversation work.
- **There is no PRD validation.** The `archetipo validate prd` gate lives in the CLI and has no equivalent here. The PRD structure is a template the agent follows, not a contract anything enforces.

## Debugging

Use the **Preview** tab with the **End user preview** toggle off. The activity trace appears beside the chat; select a node to see the input, the output and any error.

The distinction that matters: a missing file is either never created — look for the file creation in the trace — or created and not surfaced as a card, which is a harness problem, not an agent one. A reply that says the file was "saved" somewhere is an instructions problem: check **Build > Tools** for a storage tool and check that the **Persistence** section is the current one.

## Deliberate V1 boundary

Inception, Design and Analysis only. No backlog, no planning, no implementation, no review, no Wiki. Those phases need a repository and a working tree that outlives a conversation, which this environment does not have.

The inception and design skills keep the same names as their counterparts in `skills/`, and the method inside them is unchanged — only the persistence layer was replaced. The analysis skill is specific to this integration: it targets the customer's own "Descrizione intervento" document format, and its detail depends on the as-is documentation loaded in the Knowledge.
