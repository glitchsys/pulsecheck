# PulseCheck

PulseCheck is a customer-signal page for one product. Visitors report a problem without an account, and the public page shows those reports as counts on a 24-hour chart. A description or email is stored privately and, when SMTP is configured, sent to your support address.

Confirmed incidents stay on your official status page. A rise in reports is a signal to look at, and the page says so.

One deployment is one product. You name the services (API, web, authentication, and so on). It is not a directory of other companies.

## Run it

The same binary runs directly on Linux or in Docker. The database is a separate setting:

| | SQLite file | External MySQL |
|---|---|---|
| Linux | `DB_DRIVER=sqlite` | `DB_DRIVER=mysql` |
| Docker | `DB_DRIVER=sqlite` plus a volume | `DB_DRIVER=mysql` |

SQLite is a single file and a single process. Use MySQL when more than one copy of PulseCheck will run, including Cloud Run. Cloud Run has no durable local disk, so that deploy uses MySQL.

Copy the example config and replace the three secrets before anything but your own machine can reach the port:

```sh
cp .env.example pulsecheck.env
```

`pulsecheck.env` is a `KEY=VALUE` file. Real environment variables override the file. `-config` and `PULSECHECK_CONFIG` both name the file. Docker Compose injects `.env` into the process, so the container does not need `-config`.

### Linux and SQLite

Build a static binary (Go 1.22 or newer):

```sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o pulsecheck ./cmd/pulsecheck
./pulsecheck -config pulsecheck.env
```

With `DB_DRIVER=sqlite`, the database file defaults to `./data/pulsecheck.db`. The directory is created on startup. Open `http://127.0.0.1:8080/`.

For a host that should keep running, install the binary and a systemd unit:

```sh
sudo useradd --system --home /var/lib/pulsecheck --create-home pulsecheck
sudo install -m 755 pulsecheck /usr/local/bin/pulsecheck
sudo mkdir -p /etc/pulsecheck /var/lib/pulsecheck
sudo cp pulsecheck.env /etc/pulsecheck/pulsecheck.env
sudo chown root:pulsecheck /etc/pulsecheck/pulsecheck.env
sudo chmod 640 /etc/pulsecheck/pulsecheck.env
sudo chown pulsecheck:pulsecheck /var/lib/pulsecheck
```

Set `SQLITE_PATH=/var/lib/pulsecheck/pulsecheck.db` in that file, then:

```sh
sudo cp deploy/pulsecheck.service /etc/systemd/system/pulsecheck.service
sudo systemctl daemon-reload
sudo systemctl enable --now pulsecheck
```

Put a reverse proxy in front for TLS. See below.

### Linux and MySQL

Create an empty database and a user that can create tables in it. In `pulsecheck.env`:

```
DB_DRIVER=mysql
DATABASE_URL=pulsecheck:password@tcp(db.example.com:3306)/pulsecheck?parseTime=true&charset=utf8mb4
```

MySQL 8 is the version this was written for. The app creates its tables on startup. It does not install or administer MySQL. Start the binary the same way as the SQLite install. The systemd unit can stay; the data directory is unused when the driver is MySQL.

### Docker and SQLite

```sh
cp .env.example .env
docker compose up --build
```

Compose sets `SQLITE_PATH=/data/pulsecheck.db` and stores that file in the `pulsecheck-data` volume. That overrides a `./data/...` path in `.env`.

### Docker and MySQL

For a MySQL server you already run, set `DB_DRIVER=mysql` and `DATABASE_URL` in `.env`, then `docker compose up --build`.

To start a local MySQL 8.4 in Compose as well (password `pulsecheck`, database `pulsecheck`, hostname `mysql`):

```
DB_DRIVER=mysql
DATABASE_URL=pulsecheck:pulsecheck@tcp(mysql:3306)/pulsecheck?parseTime=true&charset=utf8mb4
```

```sh
docker compose --profile mysql up --build
```

That MySQL password is for this local profile only.

### Cloud Run

Build and deploy the same image. Set `DB_DRIVER=mysql` and a Cloud SQL URL, for example:

```
user:password@unix(/cloudsql/PROJECT:REGION:INSTANCE)/pulsecheck?parseTime=true&charset=utf8mb4
```

Also set `TRUST_PROXY=true` and `COOKIE_SECURE=true`. Attach the Cloud SQL instance to the service. Leave SQLite unset. The process listens on `PORT`.

`GET /healthz` returns `ok` when the database answers.

## Using the page

The public page shows a last-hour badge, per-service counts, and a 24-hour bar chart. Times on the chart are UTC. The badge compares the last hour with `SPIKE_THRESHOLD_AMBER` and `SPIKE_THRESHOLD_RED`. Each bar uses those same numbers against that single bucket.

Anyone can submit a report: service, problem type, optional description (2,000 characters), optional email. Other visitors only see counts. The description and email are on the admin page and in the support email.

`/admin` asks for `ADMIN_PASSWORD`. From there you can read private reports and delete spam. Deleting a report removes it from the chart count. There is no account system.

If SMTP is not configured, the report is still stored and its mail status is `skipped`. If sending fails, the visitor still gets a success page and the status is `failed`, so the report remains on the admin page. Mail is sent during the request, before the response, so a platform that freezes the process after the response still delivers it.

A second report for the same service from the same client inside `DEDUPE_WINDOW_MINUTES` does not increase the count. The raw IP address is not stored. The database stores an HMAC of the address with `IP_HASH_PEPPER`.

## Reverse proxy

Terminate TLS at the proxy and forward to port 8080:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Set `TRUST_PROXY=true` and `COOKIE_SECURE=true` in that setup. Leave `TRUST_PROXY` false when clients connect straight to PulseCheck. With the proxy flag on, the first `X-Forwarded-For` value is what the dedupe window uses, so the process should not be reachable except through the proxy.

## Configuration

| Variable | Purpose |
|---|---|
| `PORT` | Listen port, default `8080` |
| `DB_DRIVER` | `sqlite` or `mysql` |
| `SQLITE_PATH` | SQLite file, default `./data/pulsecheck.db` |
| `DATABASE_URL` | MySQL DSN, required when the driver is `mysql` |
| `ADMIN_PASSWORD` | Admin page password, at least 8 characters |
| `SESSION_SECRET` | Signs the admin cookie, at least 16 characters |
| `IP_HASH_PEPPER` | HMAC key for client addresses, at least 16 characters |
| `PRODUCT_NAME` | Name in the header, default PulseCheck |
| `OFFICIAL_STATUS_URL` | Optional official status link |
| `SERVICES` | `slug:Name` pairs, comma-separated |
| `SPIKE_THRESHOLD_AMBER` | Last-hour count for the elevated badge, default 3 |
| `SPIKE_THRESHOLD_RED` | Last-hour count for the high badge, default 10 |
| `DEDUPE_WINDOW_MINUTES` | Minimum gap between counted reports, default 10 |
| `BUCKET_MINUTES` | Chart bucket, one of 5, 10, 15, 20, 30, 60 |
| `TRUST_PROXY` | Trust `X-Forwarded-For` from a reverse proxy |
| `COOKIE_SECURE` | Mark cookies Secure |
| `SMTP_HOST` | Empty disables mail |
| `SMTP_PORT` | Default 587 (STARTTLS). Port 465 uses implicit TLS |
| `SMTP_USER`, `SMTP_PASSWORD` | Optional SMTP auth |
| `SMTP_FROM` | From address, required when SMTP is on |
| `SUPPORT_EMAIL` | One or more recipient addresses, comma-separated |

`SERVICES` is written into the database only when the service list is empty. Changing the variable later does not add or rename services. To start over with SQLite, stop the process and remove the database file.

## Data

Public charts read aggregate buckets. Each counted report increments the bucket for its service, problem type, and time window in the same transaction. Private rows hold the description, email, mail status, and the IP hash. Admin pages render description text as text.

## Development

```sh
go test ./...
CGO_ENABLED=0 go build -o pulsecheck ./cmd/pulsecheck
```

`TestMySQLSubmitDedupeAndDelete` runs when `MYSQL_TEST_DSN` is set to an empty MySQL 8 database.

The background notes that led to this layout are in [docs/design-notes.md](docs/design-notes.md).
