# England Software

A small Go website with a contact form and an unlinked admin inbox. Contact messages, failed login attempts, temporary IP lockouts, and admin sessions are stored in `data/main.sqlite`.

## Run locally

Requires Go 1.26 or newer. From the project root:

```sh
go run .
```

Open <http://localhost:8493>. The admin sign in page is at <http://localhost:8493/admin/login>; it is intentionally absent from the public navigation. Set `ENGLANDSOFTWARE_PORT` to use another port.

The server loads `config/.env` and creates `data/main.sqlite` and the `data` directory when needed. The database and credentials file are ignored by Git. The credentials file needs:

```dotenv
ENGLANDSOFTWARE_ADMIN_USERNAME=admin
ENGLANDSOFTWARE_ADMIN_PASSWORD=<your password>
ENGLANDSOFTWARE_SECURE_COOKIES=false
```

The local `config/.env` contains the admin password in plaintext. To change it, edit `ENGLANDSOFTWARE_ADMIN_PASSWORD` directly. Environment variables supplied to the process override values in `.env`.

## Admin and form behavior

- The contact form saves name, email, and message to SQLite. Its hidden honeypot silently discards bot submissions, and each IP can send three messages in a rolling 24 hour period.
- Admin sign in checks the password from `config/.env`. A filled honeypot or wrong credentials count as failed attempts. Five failures from one IP within a rolling 24 hour period cause a 24 hour lockout.
- Failed login rows and contact submission counters older than 24 hours, plus expired lockouts and sessions, are deleted on startup, hourly, and as relevant requests arrive. SQLite reuses pages freed by deleted rows, so the rate-limit tables do not accumulate daily history. Contact messages themselves remain until deleted from the inbox.
- Admin sessions expire after 24 hours. The inbox can mark messages read or unread and delete them. These actions require a session and a CSRF token.

For HTTPS deployments behind a reverse proxy, set `ENGLANDSOFTWARE_SECURE_COOKIES=true` in `config/.env`. If the proxy forwards client IPs, also set `ENGLANDSOFTWARE_TRUSTED_PROXY_CIDRS` to the proxy's actual network range, such as `127.0.0.1/32` for a local proxy. Forwarded IP headers are ignored unless the request comes from a trusted proxy. Mount or back up both `config` and `data` when deploying in a container.

## Build and test

```sh
go test ./...
go build -o englandsoftware .
./englandsoftware
```

Docker is supported with `make docker-build` and `make docker-run`. The run target mounts the local `config` and `data` directories and uses port 8493 by default.
