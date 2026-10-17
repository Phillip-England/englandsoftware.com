# Connect this application to egate

egate is an HTTP email gateway. Use the HTTP API directly with your application's existing HTTP client; no egate SDK or package is required. The gateway sends through Resend, so the calling application does not need a Resend API key.

## Gateway and credentials

- Gateway base URL (example): `https://egate.englandsoftware.com`
- Email endpoint: `POST https://egate.englandsoftware.com/v1/email`
- Health check: `GET https://egate.englandsoftware.com/healthz`
- Sender email: `noreply@messages.englandsoftware.com`

Replace the example hostname with the actual deployed egate hostname. An egate administrator signs in at `https://egate.englandsoftware.com/login`, registers this application under **Apps**, and gives you its `eg_...` app key. The key is displayed only once. If it is lost or revoked, register a replacement app and update the calling application's secret.

Set `EGATE_HOST=https://egate.englandsoftware.com` in the calling application's server-side configuration and set `EGATE_API_KEY` to its registered app key in server-side secret storage. The hostname shown here is an example; the key must still be established for this application. Never commit, log, or expose the key to browser code. These variables belong to the calling application. On the egate server, `EGATE_HOST` is a separate setting for its listen address.

## Send an email

Make an HTTP `POST` request to `https://egate.englandsoftware.com/v1/email` with these headers:

```http
Authorization: Bearer <EGATE_API_KEY>
Content-Type: application/json
Accept: application/json
```

The JSON object has these fields:

| Field | Requirement | Meaning |
| --- | --- | --- |
| `from` | Required, nonempty string | Use `noreply@messages.englandsoftware.com` as the sender address. A display name may be added. |
| `to` | Required, nonempty string | One recipient address per request. |
| `subject` | Required, nonempty string | Email subject. |
| `text_body` | At least one body required | Plain text content. |
| `html_body` | At least one body required | HTML content. Supply both body formats when possible. |
| `reply_to` | Optional string | Reply address. |

Example request using the configured key:

```sh
curl 'https://egate.englandsoftware.com/v1/email' \
  -H "Authorization: Bearer $EGATE_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "from": "noreply@messages.englandsoftware.com",
    "to": "recipient@example.com",
    "subject": "Welcome",
    "text_body": "Welcome to Example.",
    "html_body": "<p>Welcome to Example.</p>",
    "reply_to": "support@example.com"
  }'
```

The gateway accepts one JSON object per request, with a maximum body of 1 MiB. The calling app may send `X-Egate-Client-IP` with the validated visitor IP for its per-IP allowance. A valid request returns `200 OK` with `{"status":"accepted"}` whether the gateway queues it or silently suppresses it under the app's limit. This confirms receipt, not provider delivery. Invalid JSON or missing required fields returns `400`; a missing, invalid, or revoked app key returns `401`; a database failure returns `503`. Treat non-2xx as a failed acceptance. Set a request timeout and investigate an uncertain response before resending, since egate may already have accepted it. Egate handles delivery retries and removes email jobs and IP records after 24 hours.

## Instructions for a coding assistant

Inspect the application's existing email events and configuration. Add `EGATE_API_KEY` as a server-side secret and set `EGATE_HOST` to the base URL above. Set the `from` field to `noreply@messages.englandsoftware.com` for every email; a display name may be added. Send each email using the application's standard HTTP client and the contract in this document. Preserve the existing email content and error handling expectations. Keep credentials out of source control and client bundles. Test against a mock HTTP server rather than sending real mail. Route the application's email through egate instead of calling Resend directly.
