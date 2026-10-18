---
name: html
description: Also deliver the current prompt's output as a self-contained HTML report saved to .devlens/html/
---

The user wants the output of the current prompt delivered as an HTML file in addition to the normal CLI answer.

## Steps

1. Produce your normal CLI answer as usual.
2. Also generate a self-contained HTML document that presents the same content richly — headings, sections, code blocks, tables, and ASCII/SVG diagrams as needed.
   - Inline all CSS. No external assets, fonts, scripts, or network requests.
   - It must read well as a standalone document.
3. Save it to `.devlens/html/<timestamp>-<slug>.html`, where timestamp is `YYYYMMDD-HHMMSS` and slug is a short kebab-case topic name. Create `.devlens/html/` if it does not exist.
4. Tell the user the exact file path you wrote.
5. Mention they can run `/html_publish` to open all HTML reports in a browser.

## Constraints
- One new file per invocation — never overwrite an existing file (the timestamp keeps it unique).
- Keep the HTML self-contained and offline-friendly.
