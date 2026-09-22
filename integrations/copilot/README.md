# ARchetipo on Copilot Studio

Run the first two phases of the ARchetipo method — **Inception** and **Design** — as a single agent in [Microsoft Copilot Studio](https://copilotstudio.microsoft.com/), powered by the GitHub Copilot harness. Everything the agent produces is handed back as a file to download from the conversation.

There is no CLI here. The `archetipo` binary is not invocable inside Copilot Studio, so this integration does not use it and does not use connectors. It does not use SharePoint or OneDrive either, and needs no MCP server at all: the agent writes into the harness working directory and the conversation serves those files as downloads.

## Responsibilities

| Component | Owns |
|---|---|
| ARchetipo | The product method: the discovery flow, the personas, the PRD structure, the design discipline |
| Copilot Studio | Execution: the conversation, the orchestration runtime, skill activation, file delivery |
| The user | Persistence: downloading the artifacts, and attaching the PRD back when work resumes later |

There is no storage behind the agent. Nothing survives a conversation except what the user downloaded.

## What you install

Three things, all by hand in the Copilot Studio UI:

```text
instructions.md                       -> Build > Instructions
skills/archetipo-inception/SKILL.md   -> Build > Skills > Upload a skill
skills/archetipo-design/SKILL.md      -> Build > Skills > Upload a skill
```

No tools, no MCP server. Each skill is a single self-contained `SKILL.md`, so there is nothing to package and no `.zip` to build.

## Where persistence lives

In one place: the **Persistence** section of `instructions.md`. It defines the storage model, the procedures that touch it — *Locate the product*, *Read an artifact*, *Write an artifact*, *Replace an artifact*, *Hand over* — and the limits that follow from it.

Nothing else knows how artifacts are stored. The skills call those procedures by name and never mention downloads, attachments or a working directory. Backing this agent with a document library, a drive or a repository instead is a rewrite of that one section: the method, the PRD template and the design discipline stay untouched.

## Setup

1. **Create the agent.** In Copilot Studio, create a new agent using the **GitHub Copilot harness** — the harness is what gives the agent a working directory to write into.

2. **Paste the instructions.** Copy `instructions.md` into **Build → Instructions**.

3. **Upload the two skills** and save. The **Preview** tab runs the latest saved draft — publishing is only needed to expose the agent in a channel.

4. **Check the round trip once.** Ask for a minimal mockup and confirm the single `.html` comes back as a download in the chat. If the file is only echoed as text, the harness is not exposing the working directory and this integration will not work as designed.

## How work is delivered today

```text
<working directory>/
  Shopper/
    PRD.md
    mockups/
      onboarding.html
```

One folder per product, named exactly as the product. The folders exist to keep the downloads recognisable when a conversation covers several products or mockups — they are not memory.

A mockup is a single file: every screen lives inside it and the file switches between them, because a link from one download to another does not resolve. How many screens it holds is the user's call at the start — **Minimal**, **Standard** or **Complete**.

## How resuming works

By re-uploading. The working directory is per-conversation, so "let's do the design of Shopper", said weeks later in a brand-new conversation, needs the `PRD.md` from that inception attached to the message. The agent reads it as its brief and designs from it.

If the user does not have the PRD any more, the agent offers a short inception rather than inventing the product.

## Known constraints

Stated plainly, because they shape what this integration can and cannot do:

- **No persistence.** The artifacts live only in the conversation. A user who closes the tab without downloading has lost the work; the agent says so at every hand-over, but nothing enforces it.
- **No cross-conversation continuity.** No manifest, no index, no lookup of past products. Continuity is the attached PRD, and nothing else.
- **Mockups are downloads, not pages.** Nothing renders HTML here, and links between downloaded files do not resolve, which is why a mockup is one self-contained file with its screens and its own switcher inside, and never a folder with an `index.html` hub.
- **Attachment support is the hard dependency.** Reading an attached `PRD.md` back is what makes Design-after-Inception possible; a channel that strips attachments limits the agent to single-conversation work.
- **There is no PRD validation.** The `archetipo validate prd` gate lives in the CLI and has no equivalent here. The PRD structure is a template the agent follows, not a contract anything enforces.

## Debugging

Use the **Preview** tab with the **End user preview** toggle off. The activity trace appears beside the chat; select a node to see the input, the output and any error.

The distinction that matters: a missing file is either never written — look for the write in the trace — or written and not surfaced, which is a harness problem, not an agent one.

## Deliberate V1 boundary

Inception and Design only. No backlog, no planning, no implementation, no review, no Wiki. Those phases need a repository and a working tree that outlives a conversation, which this environment does not have.

The two skills keep the same names as their counterparts in `skills/`, and the method inside them is unchanged — only the persistence layer was replaced.
