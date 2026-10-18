---
name: newf
description: Create a new feature or system documentation folder under /docs/features/ or /docs/systems/ with the initial version
---

The user wants to start documentation for a new feature or system.

## Steps

1. **Identify the domain.**
   - If not specified in the prompt, ask: is this a feature (`/docs/features/`) or a system (`/docs/systems/`)?

2. **Generate the feature name from the user's description** — do not wait for the user to provide a name.
   - Name must be broad and generic — a top-level domain concept, not an implementation detail.
   - Think: "what is the big topic this belongs to?" not "what is this specific thing?"
   - Good: `user-auth`, `payments`, `notifications`, `task-management`
   - Bad: `password-reset-flow`, `stripe-webhook-handler`, `email-notification-template`
   - Use kebab-case.

3. **Before creating, scan existing features** under both `/docs/features/` and `/docs/systems/`.
   - If the description fits naturally under an existing feature, stop and suggest `/newv` on that feature instead.
   - Only create a new folder if the domain is genuinely new.

4. **Create the initial version folder:** `<domain>/<feature-name>/00000-init/`

5. **Write the 3 required files** based on the description from the prompt:
   - `01-requirements.md` — business intent, functional requirements, constraints. Max 100 lines.
   - `02-design.md` — initial design, architecture, flows. No line limit. Use ASCII flow diagrams for any flow that is not trivially simple.
   - `03-plan.md` — execution checklist. No line limit. Every step MUST use one of: `[ ]` todo, `[~]` in progress, `[x]` done. Start all steps as `[ ]`.
   - If detail is thin, write minimal stubs and note what needs to be filled in.

6. **Report** the generated name, the created path, and confirm the active version is `00000-init`.

## Constraints
- Follow all Feature & System Documentation rules from `.devlens/rules.md`
- Never wait for the user to name the feature — always derive it from context
- Names must be broad/generic — avoid narrow sub-feature folders
- Never create more than 3 files in the version folder
- Requirements must stay ≤ 100 lines; 03-plan.md has no line limit
- 03-plan.md must be a checklist — no prose steps, only `[ ]` / `[~]` / `[x]` items
- If any check fails → stop and ask the user before proceeding
