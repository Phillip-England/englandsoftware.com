# Contact form email delivery

This Go app sends contact messages through [egate](EGATE.md). The web process never waits for an email provider response. It commits the message, its IP rate-limit record, and an email job to `data/main.sqlite` in one transaction, then returns a confirmation. A separate worker process is started and supervised by the web process. A one-byte stdin nudge wakes it after a new job; it also checks SQLite every 15 seconds, including at startup, so a lost nudge does not lose mail.

## Configuration

Register this website as an app in egate and obtain its `eg_...` key. Set these server-side values in `config/.env` or the process environment:

```dotenv
EGATE_HOST=https://englandsoftware.com
EGATE_API_KEY=<registered app key>
CONTACT_EMAIL_TO=<inbox that should receive inquiries>
```

The web process requires a valid recipient and nonempty key at startup; it does not accept contact messages without delivery configuration. Environment variables override `config/.env`. Keep the key and database out of source control. The `from` address is `noreply@messages.englandsoftware.com`, and the visitor's validated address is sent as `reply_to`. The email includes the name, address, message, and follow-up preference. No egate key is sent to the browser.

Keep `data/` on persistent storage and back it up. The worker shares the web process's SQLite file and inherits its environment. In Docker, mount `data/` and `config/` as described in the README. Start the normal web executable only; it launches the `email-worker` child. If that child exits, the web process restarts it. The child can also be run manually as `./englandsoftware email-worker` from the project root when diagnosing delivery, but do not run multiple workers at once.

## Form protection and limits

- The visible form asks for a name, email, message, and optional marketing follow-up consent. The message counter shows how many of its 3,000 characters remain. The server enforces the same message limit, a 12,000-byte message cap, a 120-character name cap, a 254-byte email cap, and a 16 KiB total request cap. It validates the email address again on the server.
- Eight offscreen text inputs are honeypots. Any nonempty value in any of them makes the server silently discard the submission while returning the normal confirmation. They are not a guarantee against a bot that deliberately avoids them.
- For an accepted submission, SQLite first removes IP records that are at least 24 hours old. It then counts that IP's accepted submissions within the rolling 24-hour window. The first three are accepted; later requests receive HTTP 429 with a phone link to 918-500-8396. The page does not advertise the limit or remaining count in advance.
- Only configured trusted reverse proxies may supply the client IP through `X-Forwarded-For`; other requests use the connecting IP. Set `ENGLANDSOFTWARE_TRUSTED_PROXY_CIDRS` to the actual proxy range. A shared IP also shares the three-message allowance. IP limits and honeypots reduce abuse but cannot identify every bot or distinguish people behind one shared address.

## Delivery and recovery

The worker reads due `contact_email_jobs` rows and posts JSON to `EGATE_HOST/v1/email` with a 10-second HTTP timeout. A 2xx response deletes the job. A definite non-2xx response keeps it in `pending` status and schedules another attempt with increasing delay, capped at one hour. If the connection fails without a response, egate might already have accepted the message. The worker changes that job to `review` and does not retry it automatically, to avoid sending a duplicate. The original message remains in the admin inbox in every case. Check application logs for `contact email job` errors.

Inspect jobs with SQLite:

```sh
sqlite3 data/main.sqlite 'SELECT id,status,attempts,next_attempt_at,last_error FROM contact_email_jobs ORDER BY id;'
```

For a `review` job, confirm in egate whether it was delivered. If it was delivered, remove the job by ID. If it was not delivered, return it to the queue with `UPDATE contact_email_jobs SET status='pending', next_attempt_at=datetime('now') WHERE id=<id>;`. The worker will pick it up within 15 seconds. Use a UTC RFC3339 timestamp for `next_attempt_at` if setting a future time; the `datetime('now')` value above is immediately due. Never reset a `review` job before checking for delivery.

Because egate does not provide a documented idempotency key for this endpoint, a crash after egate accepts an email but before the worker removes its row could also lead to a duplicate on restart. Keep the admin inbox as the source of truth for all submitted messages. Sent jobs are deleted; rate-limit IP rows are pruned on each accepted contact request, at startup, and hourly. Contact messages remain until removed from the admin inbox.

## Reusing this pattern

For another project, keep the public handler limited to validation, honeypot checks, a transactional IP count and queue insert, and a quick response. Give the worker the same durable database, a wake signal plus periodic polling, server-side egate credentials, a bounded HTTP timeout, and explicit handling for uncertain delivery. Change the recipient and form fields for that project's needs while keeping the egate request contract in `EGATE.md`.
