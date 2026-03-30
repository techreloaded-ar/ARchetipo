---
name: airchetipo-loop
description: Executes a prompt iteratively in a loop, spawning a dedicated subagent for each iteration to keep context clean. Stops when a user-defined exit condition is met or the maximum number of iterations is reached. Use this skill whenever the user wants to repeat an action multiple times, run a task in a loop, iterate over a set of items, batch-process stories or tasks, or execute any repetitive workflow until a condition is satisfied. Also triggers when the user says things like "do this for all stories", "keep going until done", "repeat until X", "batch execute", "loop through", or any variation of iterative/repeated execution.
---

# AIRchetipo Loop — Iterative Prompt Execution

You are a **loop controller** that executes a prompt iteratively, spawning a fresh subagent for each iteration to prevent context pollution. You coordinate the loop, track state, and decide when to stop.

---

## Input Parameters

The user provides three inputs when invoking the skill:

| Parameter | Format | Example |
|---|---|---|
| **prompt** | The prompt to execute at each iteration | `"esegui /airchetipo-implement sulla prossima storia PLANNED"` |
| **max-loop** | Maximum number of iterations (default: 5) | `--max-loop 10` |
| **stop-when** | Exit condition in natural language | `--stop-when "tutte le storie sono in DONE"` |

**Argument parsing:**
- The first argument (in quotes) is the prompt
- `--max-loop N` sets the maximum limit (defaults to 5 if omitted)
- `--stop-when "condition"` defines the exit condition (if omitted, the loop executes exactly max-loop iterations)

**Invocation example:**
```
/airchetipo-loop "esegui /airchetipo-implement sulla prossima storia PLANNED" --max-loop 5 --stop-when "tutte le storie del backlog sono in DONE"
```

---

## Architecture

```
Loop Controller (main context, lightweight)
  │
  ├─ Iteration 1 → Subagent (isolated context) → result
  ├─ Iteration 2 → Subagent (isolated context) → result
  ├─ Iteration 3 → Subagent (isolated context) → result
  └─ ...
```

Each iteration runs in a **subagent with a dedicated context**, so that:
- The controller context stays lightweight (summaries only)
- Each iteration starts fresh, with no residue from previous ones
- The controller always has a clear view of overall state

---

## State File

Each loop generates a **unique state file** in the project's `.airchetipo/` folder, using the unix timestamp as identifier:

```
.airchetipo/loop-state-{unix_timestamp}.yaml
```

For example: `.airchetipo/loop-state-1711187400.yaml`

If the `.airchetipo` folder does not exist, create it before writing the state file. This file serves as persistent memory for the loop and is updated after each iteration.

```yaml
loop:
  prompt: "esegui /airchetipo-implement sulla prossima storia PLANNED"
  exit_condition: "tutte le storie del backlog sono in DONE"
  max_iterations: 5
  current_iteration: 2
  status: running  # running | completed | max_reached | error | stopped
  started_at: "2026-03-23T10:30:00"
  updated_at: "2026-03-23T10:45:30"

iterations:
  - iteration: 1
    summary: "Implementata US-001 - Login utente"
    result: success  # success | error | skipped
    timestamp: "2026-03-23T10:32:15"
  - iteration: 2
    summary: "Implementata US-002 - Dashboard principale"
    result: success
    timestamp: "2026-03-23T10:45:30"
```

The `updated_at` field is updated at each iteration and is used to identify orphan loops (see PHASE 0).

The state file has two purposes:
1. **Resilience** — if the session is interrupted, the loop can be resumed
2. **Context for subagents** — each subagent receives the summary of previous iterations, not the details

---

## Workflow

### PHASE 0 — Initialization

1. Parse user arguments (prompt, max-loop, stop-when)

2. **Cleanup residual state files:** find all `.airchetipo/loop-state-*.yaml` files with terminal status (`completed`, `max_reached`, `stopped`) and delete them. These files belong to already-finished loops and are no longer needed.

3. **Active loop detection:** find all `.airchetipo/loop-state-*.yaml` files with `status: running` or `status: error`.

   **If none found:** proceed normally to step 4.

   **If one found:**
   - Check the `updated_at` field: if it is older than **2 hours**, flag it as a probable orphan loop:
     ```
     Trovato un loop in stato "running", ma l'ultima attività risale a {tempo_fa}.
     Probabilmente la sessione si è interrotta.
     - **Prompt:** "{prompt del loop}"
     - **Progresso:** iterazione {N}/{max}

     Vuoi riprenderlo o scartarlo e avviarne uno nuovo?
     ```
   - If `updated_at` is recent (less than 2 hours), the loop is probably still active on another instance. Warn the user:
     ```
     Trovato un loop attivo (ultima attività {tempo_fa}):
     - **Prompt:** "{prompt del loop}"
     - **Progresso:** iterazione {N}/{max}

     Potrebbe essere in esecuzione su un'altra sessione. Vuoi comunque riprenderlo, oppure avviare un nuovo loop indipendente?
     ```
   - If the user wants to **resume**: read the state file, set `current_iteration` to the saved value + 1, and proceed from PHASE 1. The subagent will receive the summary of already-completed iterations from the state file, ensuring continuity without re-executing anything.
   - If the user wants to **discard**: delete the state file and proceed normally.
   - If the user wants to **start an independent loop**: proceed normally to step 4 (a new file with a different timestamp will be created).

   **If more than one found:** present a list and ask the user how to proceed:
   ```
   Trovati {N} loop attivi:

   | # | Prompt | Progresso | Ultima attività | Stato |
   |---|---|---|---|---|
   | 1 | "{prompt}" | {N}/{max} | {updated_at} | {running/error} |
   | 2 | "{prompt}" | {N}/{max} | {updated_at} | {running/error} |

   Vuoi riprendere uno di questi, scartarli tutti, o avviare un nuovo loop indipendente?
   ```

4. Generate the current unix timestamp as the loop ID and create the initial state file: `.airchetipo/loop-state-{unix_timestamp}.yaml`

5. Communicate the loop start to the user:

```
## Loop avviato

- **Prompt:** {prompt}
- **Max iterazioni:** {max-loop}
- **Condizione di uscita:** {stop-when}

Avvio iterazione 1/{max-loop}...
```

### PHASE 1 — Iteration Execution

Spawn a subagent to execute the current iteration. The subagent prompt must be constructed including all necessary information so the subagent can operate autonomously, without prior knowledge of the project:

```
## Operational Context

- **Working directory:** {absolute path to project root}
- **Iteration:** {N} of {max}

### Previous Iterations Summary
{summary from previous iterations in the state file, or "First iteration — no prior context." if N=1}

## Task

{user's prompt}

## Instructions

1. Before operating, read the project configuration files if present (CLAUDE.md, README.md, or equivalents) to understand the project structure and conventions
2. Execute the task described above
3. When done, return a concise summary (1-2 sentences) of what you did and the result obtained
```

After the subagent returns its result:
- Update the state file with the summary, iteration result, and the `updated_at` field
- Briefly communicate to the user what happened:

```
### Iterazione {N} completata
{riepilogo dal subagent}
```

### PHASE 2 — Exit Condition Evaluation

After each iteration, evaluate whether the loop should stop. Run checks in this order:

**Check A — Exit condition met:**

If the user specified `--stop-when`, verify the condition. This requires concrete actions: reading files, checking statuses, inspecting the backlog — whatever is needed to determine if the condition is satisfied.

If the condition is met → terminate the loop with `status: completed` and go to PHASE 3.

**Check B — Maximum limit reached:**

If `current_iteration >= max_iterations` → terminate the loop with `status: max_reached` and go to PHASE 3.

**If no exit condition is satisfied** → return to PHASE 1 with the next iteration.

### PHASE 3 — Closure

When the loop ends (for any reason):

1. Update the state file with the final status (`completed`, `max_reached`, `error`, or `stopped`)
2. Present the final summary to the user with this structure:

```
## Loop {final_status}

{closing message appropriate to the status — see below}

### Riepilogo iterazioni

| # | Riepilogo | Risultato |
|---|---|---|
| 1 | {summary iteration 1} | {success/error/skipped} |
| 2 | {summary iteration 2} | {success/error/skipped} |
| ... | ... | ... |

**Iterazioni eseguite:** {N}/{max}
```

**Closing messages by status:**

- `completed`: *"La condizione di uscita è stata raggiunta: \"{stop-when}\""*
- `max_reached`: ALWAYS include the suggestion to continue. Calculate how many iterations would be needed based on remaining work and suggest a concrete value:
  ```
  Raggiunte {max-loop} iterazioni senza soddisfare la condizione di uscita: "{stop-when}".

  **Per proseguire**, riesegui il loop con un limite più alto:
  /airchetipo-loop "{prompt originale}" --max-loop {valore suggerito} --stop-when "{stop-when originale}"
  ```
  The suggested value must be realistic: if 7 tasks remain out of 10 and you completed 3 in 3 iterations, suggest `--max-loop 7` (not an arbitrary double). If you cannot estimate, use `{max * 2}` as fallback.
- `error`: *"Il loop è stato interrotto a causa di un errore alla iterazione {N}."*
- `stopped`: *"Il loop è stato fermato dall'utente alla iterazione {N}."*

3. **Delete the state file** of the just-finished loop. The summary has already been communicated to the user and there is no need to keep the file.

---

## Error Handling

If a subagent fails or returns an error:

1. Record the error in the state file:
   - `result: error` for the current iteration
   - `error_detail:` with the error description
   - `status:` stays `running` (not yet decided whether to stop)
   - Update `updated_at`

2. Ask the user how to proceed:
   - **Riprova** — re-execute the same iteration (do not increment the counter)
   - **Salta** — mark as `skipped`, increment the counter, proceed to next
   - **Ferma** — set `status: stopped` and go to PHASE 3

3. Record the user's choice in the state file:
   - `user_action: retry | skip | stop`

Do not proceed automatically after an error — the user must decide.

---

## Requirements

This skill requires a tool that supports **subagents with isolated context**:
- **Claude Code** — Tool `Agent`
- **Gemini CLI** — Tool `create_sub_agent`
- **Roo Code** — Tool `new_task` / Orchestrator mode
- **Augment Code** — Parallel agents
