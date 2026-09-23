# Landmines

- `SERVICES` is inserted only when `components` is empty. Later edits to the env var do not rename or add services.
- SQLite uses one connection and one process. A second replica on the same file will fail or corrupt it. Use MySQL for more than one instance, including Cloud Run.
- Support mail is sent inside the HTTP request, then the row's `email_status` is updated. A goroutine would be dropped when Cloud Run freezes CPU after the response. Mail failure still returns success to the visitor.
- `report_windows` dedupe is an upsert. Allowed means `RowsAffected > 0`. MySQL reports 0 when the `IF` writes the same timestamp. Do not enable the MySQL `CLIENT_FOUND_ROWS` flag, or duplicates will be counted.
- The window is "at least `DEDUPE_WINDOW_MINUTES` after the previous counted report," compared in unix seconds. It is not a sliding check done with `SELECT COUNT`.
- `TRUST_PROXY` trusts the first `X-Forwarded-For` hop. Leave it false unless a reverse proxy is the only client that can reach the process.
- Deleting a report decrements the aggregate bucket for that report's original timestamp.
- Public handlers read `aggregates` only. Descriptions and emails live on `reports` and are rendered as text on `/admin`.
