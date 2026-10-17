# Contact form email delivery

The site validates each contact submission, saves it in the admin inbox, and calls egate directly. Egate accepts valid requests quickly and runs its own delivery worker. The site has no email queue or per-IP sending allowance. Its inbox keeps the newest 200 messages by default; each new message removes the oldest one once capacity is reached. Admins can still delete messages manually.

## Site configuration

Register this website as an app in egate. Set these server-side values in `config/.env` or the process environment:

```dotenv
EGATE_HOST=https://egate.englandsoftware.com
EGATE_API_KEY=<registered app key>
CONTACT_EMAIL_TO=<inbox that should receive inquiries>
ENGLANDSOFTWARE_MESSAGE_CAPACITY=200
```

The hostname above is an example; set it to the actual deployed egate hostname. `EGATE_HOST` on this site is the gateway's public HTTPS base URL. It is different from the egate server's `EGATE_HOST`, which is its listen interface. The site requires a valid recipient and nonempty key at startup. Process environment overrides `config/.env`. Keep the key and database out of source control.

The `from` address is `noreply@messages.englandsoftware.com`; the visitor's address is `reply_to`. The email includes the name, address, message, and follow-up preference. The API key stays on the server. The site passes the visitor IP in `X-Egate-Client-IP`, derived from the direct peer or a configured trusted proxy. Set `ENGLANDSOFTWARE_TRUSTED_PROXY_CIDRS` to the actual reverse-proxy CIDRs in production.

## Behavior and recovery

The form enforces a 3,000-character message limit, a 12,000-byte message cap, a 120-character name cap, a 254-byte email cap, and a 16 KiB request cap. It validates the address on the server and silently discards filled honeypots. A valid message is saved before the gateway call, so it remains in the admin inbox even if egate is unavailable. A gateway failure returns HTTP 502 and is logged as `contact gateway acceptance failed`; the response tells the visitor the message was saved but email delivery could not be confirmed. Check the inbox and gateway before resending to avoid a duplicate.

Egate accepts the request with HTTP 200 and a generic acceptance body whether it queues delivery or suppresses an IP that exceeded the app's configured limit. Delivery failures and retries are managed in egate. In egate's admin page, set this app's per-IP messages per 24 hours to `3`; `0` means unlimited. The Email jail section shows blocked IPs and can release them, clearing their recent count for that app. Egate deletes email jobs, IP counts, and expired jail entries after 24 hours.

Keep both applications' `data/` directories on persistent storage and back them up. The site's admin inbox is the record of submitted messages; egate's recent job counts describe delivery activity within its 24-hour retention window.

## Upgrading from the old site queue

Existing `contact_email_jobs` rows are left in place for review, but the site no longer processes that table. Before replacing a running deployment, check `SELECT id,status,attempts FROM contact_email_jobs ORDER BY id;` in the site's SQLite database. Resolve pending or review rows against egate and the admin inbox before retiring that legacy table. The old IP submission table is also unused. New submissions do not write to either legacy table.
