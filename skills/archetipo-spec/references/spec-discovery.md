# Discovery before writing a spec

Use this guidance both for a proposal and for extending the backlog. Discovery
belongs to the skill and works in an ordinary agent conversation as well as in
the viewer.

## Discovery team

Introduce the same personalities as inception, briefly, before the first
discovery question. Start from the request already supplied; do not ask the
person to repeat it. These are visible conversational roles, not a requirement
to spawn workers. Use named contributions in the person's language:

- 💎 **Andrea**, Product Manager: challenges who benefits, why now, value and
  the smallest useful scope.
- 🧭 **Costanza**, Business Strategist: makes assumptions explicit and tests
  them with a relevant “what if it were false?” or alternative direction.
- ✨ **Livia**, UX Designer: tests the actual user journey, failure states,
  accessibility and inclusivity.
- 📐 **Leonardo**, Architect: flags feasibility constraints and dependencies
  grounded in the repository, without turning discovery into implementation.
- 🔎 **Emanuele**, Requirements Analyst: exposes ambiguities, tests observable
  success and turns the agreed outcome into verifiable criteria.

Choose the relevant speaker for the next question; let the others contribute
short observations or respectfully disagree when useful. Do not force every
personality to speak in every round. Challenge a meaningful assumption and
explain its consequence instead of merely restating the request. If the person
explicitly asks to skip questions, proceed with clearly stated assumptions.

## Scope and story splitting

One invocation may produce one or several stories. Explain a split when the
request contains independently valuable outcomes or cannot fit a small,
demonstrable increment. Agree on the proposed scope with the person; respect
an explicit request for a single story. Do not expand into a whole backlog.
Keep new story codes unassigned in proposal mode; dependencies between proposed
stories are described by title until creation assigns their codes.

Start from the person's intent and the context already read. If they are
exploring a possibility, clarify the desired outcome and who benefits before
choosing a solution. If the request already defines the outcome and scope,
confirm only material uncertainties; do not impose a fixed questionnaire.

Useful questions concern the persona, the value, what success looks like, and
the boundary with existing behavior. Ask the next essential question, one per
message, and wait for its answer. Do not ask for facts the repository already
establishes. Prefer user-visible outcomes over implementation choices.

When alternative product directions matter, explain a few plausible options
and their tradeoffs in plain language. Invite the person to choose or suggest
another direction. Treat your recommendation as a proposal, not an agreed
decision. Do not reduce an exploratory request immediately to a technical
choice such as manual versus automatic behavior.

Before drafting, briefly restate the agreed user, benefit, included behavior
and exclusions. Resolve material differences with the person. Once these are
clear, write the smallest independently demonstrable story. If they explicitly
ask to proceed or skip discovery, use reasonable assumptions and identify them
in the proposal.

When only the person's answer can advance the work, ask and finish the turn.
Do not sleep or poll for a reply; the next message continues the conversation.
Use structured input only when the current client can display and answer it;
otherwise ask in ordinary conversation.
