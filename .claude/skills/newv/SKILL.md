---
name: newv
description: Create a new version for an existing feature or system under /docs/features/ or /docs/systems/, following versioning and line-limit rules
---

The user wants to add a new version to an existing feature or system's documentation.

## Steps

1. **Identify the domain and feature.**
   - Scan `/docs/features/` and `/docs/systems/` and list what exists.
   - Cross-reference with the current conversation context to infer which feature is being discussed.
   - Only ask the user if 2 or more candidates are equally likely.

2. **Run the pre-write checklist** (from `.devlens/rules.md`):
   - Confirm the folder exists under the correct domain
   - List all version folders, find the highest numeric one — that is the active version
   - Count total versions
   - If any check fails → stop and ask the user before proceeding

3. **Determine the new version folder name:**
   - If total versions < 5: next increment (e.g. active is `00002` → new is `00003`)
   - If total versions == 5: do not add another version. Tell the user to run `/sumu` first to consolidate, then `/newv` again.

4. **Ask the user** what changed in this version (if not already described in the prompt).

5. **Write the new version folder** with only the changed content:
   - `01-requirements.md` — only what changed + explicit statement of what remains unchanged. Max 100 lines.
   - `02-design.md` — only design changes. No line limit. Use ASCII flow diagrams for any flow that is not trivially simple.
   - `03-plan.md` — updated checklist. No line limit. Carry over incomplete items from the previous version's plan with their current state. New steps start as `[ ]`. Every item MUST use `[ ]` todo, `[~]` in progress, or `[x]` done — no prose steps.
   - For a reset version, all 3 files must be fully self-contained.

6. **Report** the new version folder name and confirm the active version.

## Constraints
- Follow all Feature & System Documentation rules from `.devlens/rules.md`
- Never create more than 3 files per version folder
- Never skip version numbers unless user explicitly instructs
- Requirements must stay ≤ 100 lines; 03-plan.md has no line limit
- 03-plan.md must be a checklist — no prose steps, only `[ ]` / `[~]` / `[x]` items; items are never deleted, only state changes
- At the start of any coding session on this feature, read 03-plan.md, mark the current step `[~]` before starting, mark `[x]` when done
- If any pre-write check fails, stop and ask the user
