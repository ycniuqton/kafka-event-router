# Devlens Rules

## Commit Guard (protected)
- Do not run git commit or git push under any circumstances. Only proceed after receiving an explicit user instruction, and clearly indicate before performing the commit.

## Coding Rules
- Rule 1: Functions ≤ 50 lines, decompose into business steps
- Rule 2: Max 3 levels of if/else/switch nesting per function
- Rule 3: Max 3 levels of loop nesting; no helper functions — extract into domain-based classes only
- Rule 4: Body of any if/else/loop ≤ 15 lines
- Rule 5: Divide logic by business/processing steps; one step = one domain method
- Rule 6: Business logic reachable within 3 trace steps (no long forwarding chains)
- Rule 7: Max 3 levels of class inheritance
- Rule 8: Comments only at function/class/module top — never inside function bodies
- Rule 9: Function docs max 3 lines, describe intent not implementation
- Rule 10: Fix structure instead of adding inline comments
- Rule 11: Design classes for future features, not just current requirements
- Rule 12: Concrete implementations must not be entry points; business logic never depends on concrete classes
- Rule 13: Every extensible domain needs a base class; all implementations inherit from it
- Rule 14: Each extensible domain needs a Manager service; business logic talks only to Manager
- Rule 15: Implementation selection must be configurable — no if/switch on concrete types
- Rule 16: Prefer class-based handlers over functions
- Rule 17: No magic strings or numbers — all values belong to a domain
- Rule 18: Use enums or domain constant classes — no standalone constants
- Rule 19: Always ask "what business concept does this belong to?"
- Rule 20: Follow modern language conventions; these rules extend/override them
- Rule 0 (override): Business requirements have absolute priority — ask user if any rule conflicts

## HTML Content Delivery
- For complex detail or content that needs to be shown or discussed (multi-part explanations, reports, comparisons, diagrams, data tables), answer in the CLI as normal AND also generate a self-contained HTML file presenting the same content richly.
- Write HTML files to .devlens/html/ named <timestamp>-<slug>.html (timestamp = YYYYMMDD-HHMMSS, slug = short kebab-case topic).
- Each HTML file must be self-contained: inline all CSS, no external assets or network requests, readable on its own.
- Tell the user the file path. To browse all reports in a browser, run /html_publish (serves the folder on a random port) or `devlens serve-html`.
- Use /html to force this HTML delivery for a specific prompt.

## Feature & System Documentation
- Docs live under /docs/features/<name>/<version>/ or /docs/systems/<name>/<version>/
- Active version = highest numeric folder (no CURRENT/active marker files)
- First version named 00000-init; increments 00001, 00002 … max 5 versions total
- At 5 versions, run /sumu to consolidate all into one new version (next plain increment) and delete old ones — count resets to 1
- Each version folder contains ONLY: 01-requirements.md (≤100 lines), 02-design.md (no limit), 03-plan.md (no limit)
- 03-plan.md MUST be a checklist — every step uses one of three states: [ ] todo, [~] in progress, [x] done
- Coding agent MUST read 03-plan.md at the start of any coding session on that feature, mark the current step [~] before starting, and mark [x] when done — skipped or invalidated steps get an inline note, never deleted
- Checklist items are never removed — only their state changes
- 02-design.md: MUST use ASCII flow diagrams for any flow or logic that is not trivially simple — if it has multiple steps, branching, async boundaries, or touches more than one component, a diagram is required, not optional
- ASCII flow diagram rules:
  1. Start with the trigger — the entry point (HTTP request, event, cron, user action)
  2. Use └─ for causality — each step indented under what caused it; indent = "this leads to"
  3. Tag async boundaries explicitly — [async], [cron], [event] for anything that breaks the sync call chain
  4. Use → for outcomes — condition on the left, result on the right, one line
  5. Show branches as siblings — two outcomes of the same condition at the same indent level
  6. State side effects inline — emails, state changes, emitted events on the same line as the outcome, after the →
  7. Separate paths with --- — each distinct trigger gets its own block; no mixing paths in one block
- Incremental versions write only what changed; state what is unchanged; do not copy full previous content
- If active version cannot be understood alone, reset is required
- Feature names must be broad and generic (top-level domain concepts): user-auth, payments, notifications — not narrow sub-feature names like password-reset-flow
- Before creating a new feature, check if an existing feature could absorb it — prefer /newv over /newf when in doubt
- When no feature name is given, infer from conversation context + existing folder list — only ask if genuinely ambiguous
- Before writing docs run checklist: correct domain, correct name, active = max folder, versions ≤ 5, files ≤ 3, line limits respected — if any check fails → stop and ask the user before proceeding
