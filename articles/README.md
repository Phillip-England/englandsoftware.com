# Articles

Each article is a numbered Markdown file (`1.md`, `2.md`, etc.) with YAML front matter. Cover images are in `images/`. Both belong in Git.

```markdown
---
id: 1
title: Example article
image: abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ.webp
created_at: "2026-10-11T12:00:00Z"
updated_at: "2026-10-11T12:00:00Z"
draft: true
---

Article content in Markdown.
```

The admin editor writes these files directly. New articles start as drafts. Set the status to Live in the editor to show an article on the home page and at its public URL. The optional `draft: true` metadata hides an article; omitting it keeps older articles live. The editor's preview shows unsaved changes without writing a file. Commit new or changed Markdown and image files to Git to carry them through a normal deployment. The site reads the files on each request, so local edits appear immediately.
