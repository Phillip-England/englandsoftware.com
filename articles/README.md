# Articles

Each published article is a numbered Markdown file (`1.md`, `2.md`, etc.) with YAML front matter. Cover images are in `images/`. Both belong in Git.

```markdown
---
id: 1
title: Example article
image: abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ.webp
created_at: "2026-10-11T12:00:00Z"
updated_at: "2026-10-11T12:00:00Z"
---

Article content in Markdown.
```

The admin editor writes these files directly. Commit new or changed Markdown and image files to Git to publish them through a normal deployment. The site reads the files on each request, so edits in a local checkout appear immediately.
