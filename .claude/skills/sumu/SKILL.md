---
name: sumu
description: Merge all version docs of a feature or system into one consolidated version, then delete old sub-versions
---

The user wants to consolidate a feature or system's documentation history into a single up-to-date version.

## Steps

1. **Identify the domain and feature.**
   - Scan `/docs/features/` and `/docs/systems/` and list what exists.
   - Cross-reference with the current conversation context to infer which feature is being discussed.
   - Only ask the user if 2 or more candidates are equally likely.
   - If domain is still unclear, ask: `/docs/features/` or `/docs/systems/`?

2. **Read all version folders** under the chosen path, sorted numerically ascending. Read every file inside each version.

3. **Synthesize one consolidated doc set** from all versions:
   - `01-requirements.md` — final, current requirements only. No history duplication. Max 100 lines.
   - `02-design.md` — complete current design, absorbing all changes across versions. No line limit.
   - `03-plan.md` — full checklist carrying all items from all versions. No line limit. Preserve each item's last known state (`[ ]`, `[~]`, `[x]`). Do not delete any item — completed items stay as `[x]`, in-progress as `[~]`, todo as `[ ]`.

4. **Determine the new version folder name:**
   - Find the highest numeric version folder name
   - New version = highest + 1 (zero-padded to 5 digits, e.g. `00004` → `00005`)
   - Always use a plain numeric name — no reset naming, no special suffix, no cycle tracking

5. **Print the list of folders that will be deleted**, e.g.:
   ```
   Deleting: 00000-init, 00001, 00002, 00003, 00004
   Creating: 00005
   ```

6. **Write the new version folder** with the 3 consolidated files.

7. **Delete all old version folders** — no confirmation needed, just proceed.

8. **Report** the new active version and confirm old versions were removed.

## Constraints
- Follow all Feature & System Documentation rules from `.devlens/rules.md`
- Never create more than 3 files per version folder
- Requirements must stay ≤ 100 lines after consolidation — summarize if needed; 03-plan.md has no line limit
- Do not keep any CURRENT/active marker files
- After sumu, version count resets to 1 — the new folder is the only version remaining
