# England Software

**Slogan:** Make your business easier to run.

A small Go website with a public home page, articles, a contact form, and an unlinked admin portal. The home page includes websites, applications, consultation, about, recent articles, and contact. Former service URLs redirect to the relevant sections. Articles and cover images are stored in the tracked `articles/` directory. Contact messages, failed login attempts, temporary IP lockouts, and admin sessions are stored in `data/main.sqlite`.

Business identity, social links, current proposed offers, pricing, scope, and open decisions are collected in [business/MANIFEST.md](business/MANIFEST.md).

## Run locally

Requires Go 1.26 or newer and FFmpeg (with the `libwebp` encoder) on `PATH`. From the project root:

```sh
make run
```

Open <http://localhost:8493>. The admin sign in page is at <http://localhost:8493/admin/login>; it is intentionally absent from the public navigation. Use `make run APP_PORT=9000` to choose another port.

`make run` creates `config/.env` with a random admin password on the first run and prints the credentials. Add the egate key and recipient address to that file, then run `make run` again; later runs reuse it. The server creates `data/main.sqlite` and the `data` directory when needed. The database and credentials file are ignored by Git. To run `go run .` directly or use Docker, create a credentials file containing:

```dotenv
ENGLANDSOFTWARE_ADMIN_USERNAME=admin
ENGLANDSOFTWARE_ADMIN_PASSWORD=<your password>
ENGLANDSOFTWARE_SECURE_COOKIES=false
EGATE_HOST=https://englandsoftware.com
EGATE_API_KEY=<registered egate app key>
CONTACT_EMAIL_TO=<your inbox address>
```

The local `config/.env` contains the admin password in plaintext. To change it, edit `ENGLANDSOFTWARE_ADMIN_PASSWORD` directly. Environment variables supplied to the process override values in `.env`.

## Admin and form behavior

- The contact form saves name, email, message, and an optional consulting/application email follow-up preference to SQLite. The preference defaults to off and appears in the admin inbox. Eight hidden honeypot fields silently discard bot submissions, and each IP can send three messages in a rolling 24 hour period. Accepted messages are queued for egate delivery by a separate worker process; see [EMAIL_FORM.md](EMAIL_FORM.md) for setup and delivery operations.
- Admin sign in checks the password from `config/.env`. A filled honeypot or wrong credentials count as failed attempts. Five failures from one IP within a rolling 24 hour period cause a 24 hour lockout.
- Failed login rows and contact submission counters older than 24 hours, plus expired lockouts and sessions, are deleted on startup, hourly, and as relevant requests arrive. SQLite reuses pages freed by deleted rows, so the rate-limit tables do not accumulate daily history. Contact messages themselves remain until deleted from the inbox.
- Admin sessions expire after 24 hours. The inbox can mark messages read or unread and delete them. These actions require a session and a CSRF token.
- The admin Articles link opens `/admin/articles`, where you can create, edit, preview, and delete articles. Each article needs a title, a cover image, and Markdown content. The preview appears beside the editor on wider screens and updates after you pause typing or change an image; **Update preview** is also available. Preview does not save. New articles start as drafts. **Save article** writes `articles/{id}.md` with YAML metadata and the selected Draft or Live status. Drafts stay in the admin list but are hidden from the home page and public article URLs; the four newest live articles appear on the home page. Commit the Markdown files and images to Git after editing through the admin portal; Git does not commit them automatically.
- Uploaded covers are limited to 18 MB. FFmpeg takes the first frame, applies image rotation, scales it to fit within 1600 × 1200 pixels, and writes a compressed WebP and a 1200 × 630 JPEG sharing copy in `articles/images/`. Commit both images with their articles. Live articles publish Open Graph and Twitter Card tags pointing to a versioned JPEG URL so their covers appear in link previews. Platforms may retain an older preview until they re-scrape the article URL.
- On the first start after this update, any articles in an older SQLite `articles` table are copied to `articles/` with their images and the old table is removed. Commit the exported files to Git. Keep a backup of `data/` until the export is verified.

For HTTPS deployments behind a reverse proxy, set `ENGLANDSOFTWARE_SECURE_COOKIES=true` in `config/.env`. If the proxy forwards client IPs, also set `ENGLANDSOFTWARE_TRUSTED_PROXY_CIDRS` to the proxy's actual network range, such as `127.0.0.1/32` for a local proxy. Forwarded IP headers are ignored unless the request comes from a trusted proxy. Mount or back up `config`, `data`, and `articles` when deploying in a container. Admin edits inside a container require a writable `articles` mount, followed by a Git commit from the host checkout.

## Build and test

```sh
go test ./...
go build -o englandsoftware .
./englandsoftware
```

Docker is supported with `make docker-build` and `make docker-run`. The run target mounts the local `config`, `data`, and `articles` directories and uses port 8493 by default.

# Hero and search metadata

The hero uses a canvas illustration of downtown Tulsa from `static/skyline.js`. It shows a sunny scene from 7 a.m. to 7 p.m. in the visitor's local time and a moonlit scene at night. The hero button switches views manually for the current page visit; a reload returns to the time-based view. The scene includes drifting clouds, moving street lights, and the green roof of Mid-Continent Tower. Reduced-motion visitors get a still scene. The Tulsa photograph at `static/images/tulsa-skyline-hero.webp` remains the social preview image. The homepage carries Tulsa-focused title, description, social preview tags, and structured business data. `/robots.txt` points crawlers to the homepage-only `/sitemap.xml`.
