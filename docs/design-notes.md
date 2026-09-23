# Original design prompts

These are the two prompts that shaped PulseCheck. They are kept as design background. How to run the service is in the README.

The shipped app follows the smaller of the two ideas: a public report page, an aggregate chart, and private delivery to support. It does not implement accounts, organizations, or SSO.

---

A polished positioning could be:

PulseCheck
Customer-reported service signals, alongside your official status page.

Or:

PulseCheck — See when other customers are experiencing problems.

When the official status page is clear because an issue is still being investigated, narrowly scoped, transient, or not yet confirmed, customers can see anonymized recent reports from other users. A customer can add a brief description, and the system automatically forwards that report to support.



Below is a complete, copy-pasteable specification prompt optimized for LLM coding tools like Cursor (using Composer) or Claude Code. It specifies stack choices, rate limiting, data privacy, and support routing to ensure you get a clean, functional codebase on the first pass.

```
# Project Goal: Build "PulseCheck" – An Open-Source, Crowdsourced Signal & Outage-Reporting Layer

## Overview
Build a lightweight, open-source web application ("PulseCheck") that acts as an early-warning signal layer for SaaS platforms. Unlike traditional authoritative status pages, PulseCheck allows users to self-report issues, visualizes report spikes over time via a public graph, and routes details directly to support while preserving privacy.

## Key Technical Decisions & Architecture
- **Framework:** Next.js (App Router, TypeScript, Tailwind CSS)
- **Database:** Supabase (PostgreSQL) or SQLite via Prisma/Drizzle (keep it easily deployable on Vercel / Railway / self-hosted)
- **Charting:** Recharts or Chart.js (for rendering 5-minute/15-minute time-bucketed report spikes)
- **Email Delivery:** Resend / SendGrid API (or SMTP fallback) to email support when a user submits issue details
- **Styling:** Modern, clean, dark-mode friendly using Shadcn UI + Tailwind

---

## Core Features & System Requirements

### 1. Public Signal Dashboard (`/`)
- **Header:** Displays company/platform logo, title, and current 1-hour "Signal Volume" badge (e.g., "Normal Signal Level" vs. "Elevated Reports").
- **Time-Series Graph:**
  - Displays a bar chart showing report counts over the last 24 hours, bucketed in 15-minute intervals.
  - Interactive tooltips showing total report counts per time bucket.
  - Color-coded bars (e.g., Green for <3 reports/bucket, Amber for 3–10, Red for >10).
- **Service Selector:** A dropdown/tab filter allowing users to view reports by specific service/microservice (e.g., "API", "Web App", "Billing", "Integrations", "All Services").
- **Recent Anonymous Activity Feed:**
  - Displays recent public tags/categories (e.g., "API Timeout reported 4m ago") without displaying user identities, emails, or specific enterprise tenant names.

### 2. "I'm Having an Issue" Submission Flow
- **Primary CTA:** A prominent, high-visibility "Report an Issue" button on the dashboard.
- **Report Modal / Form Steps:**
  1. **Quick Signal (1-Click):** Select the affected service (e.g., "Dashboard", "API") and category (e.g., "Latency / Slow", "Error / Down", "Authentication").
  2. **Details & Support Dispatch (Optional but Encouraged):**
     - Text area: "Describe what you're seeing (optional)."
     - Input field: "Your Email Address (optional - to receive support updates)."
  3. **Submit Action:**
     - Instantly updates local time-series state to reflect the new report.
     - Saves event record to the database.
     - **Email Trigger:** If the user provided issue details, send an automated HTML email to the pre-configured support email address containing timestamp, category, optional user email, and description.

### 3. Anti-Spam & Abuse Safeguards
- **Client Rate Limiting:** Prevent a single user/IP from spamming the "Report" button (max 1 report per service per 10 minutes per IP/fingerprint).
- **Graceful Rate-Limit UX:** If rate-limited, show a non-intrusive toast: *"You've already reported an issue recently. Your signal is counted!"*

### 4. Admin / Configuration Settings
- **Environment Variables:**
  - `SUPPORT_EMAIL`: Pre-configured email destination for incoming reports.
  - `SERVICES_LIST`: Comma-separated list of services (e.g., `API, Dashboard, Webhooks, Billing`).
  - `SPIKE_THRESHOLD_AMBER`: Number of reports in a 15-min window to trigger "Elevated" warning.
  - `SPIKE_THRESHOLD_RED`: Number of reports in a 15-min window to trigger "High Volume" warning.
- **Simple Admin Route (`/admin`):** Password-protected page to prune spam reports, update threshold limits, or mark resolution notes on the public chart.

---

## Folder & Code Structure
Please organize the code logically:
- `/app` – Pages (`/`, `/admin`), API routes (`/api/report`, `/api/signal-data`)
- `/components` – UI components (`ReportModal`, `SignalChart`, `ServiceFilter`, `Header`)
- `/lib` – Database helpers, email dispatch logic (Resend/SMTP), rate limiter, timestamp bucketing utility
- `/types` – TypeScript interfaces for Report, Service, TimeBucket

---

## Instructions for AI Coder
1. Generate complete, clean code with no placeholder TODOs for core features.
2. Implement robust error handling on API endpoints.
3. Include clear inline documentation and a `README.md` explaining how to deploy this repository on Vercel/Railway with zero friction.
```


another AI Prompt suggestion on how to vibe code this:

```
You are a senior staff full-stack engineer building a secure, production-minded,
self-hosted open-source application named “SignalStatus.”

SignalStatus is NOT an official status-page replacement and must never present
unverified user reports as confirmed outages.

Its purpose is to provide a customer-signal layer alongside a company’s
authoritative status page:

- Customers can see anonymized, aggregate, recent reports that other users are
  experiencing trouble with a service or component.
- Customers can quickly submit a report such as “I cannot log in” or “API calls
  are failing.”
- The report contributes only an anonymized count to a public time-series graph.
- The report’s private description and reporter contact information are stored
  privately and automatically sent to a configured support email address.
- Operators can use report spikes as an early-warning signal, but only an
  operator can publish an official incident through another system.

Build a complete MVP as a self-hosted application. Prioritize correctness,
security, privacy, accessibility, test coverage, documentation, and a clean
developer experience. Do not implement unrelated enterprise features.

============================================================
1. TECHNICAL STACK
============================================================

Use this stack unless there is a compelling implementation issue:

- Next.js 15+ with App Router
- TypeScript with strict mode enabled
- PostgreSQL
- Prisma ORM
- Tailwind CSS
- shadcn/ui or accessible equivalent components
- Zod for all request, environment, and form validation
- Auth.js / NextAuth with a credentials-based development login and a clean
  provider abstraction that can later support OIDC/SAML-compatible enterprise SSO
- Resend as the default email provider, behind an EmailProvider interface
- Docker Compose for local and production-like self-hosted deployment
- Vitest for unit tests
- Playwright for end-to-end tests
- ESLint, Prettier, and a CI GitHub Actions workflow

Use server-side route handlers or server actions appropriately. Do not place
secrets in the browser. Do not use an external hosted database dependency.

The result must run locally with:

  cp .env.example .env
  docker compose up -d db
  npm install
  npx prisma migrate dev
  npm run dev

Also provide a fully containerized:

  docker compose up --build

deployment path.

============================================================
2. CORE PRODUCT PRINCIPLES
============================================================

Use these rules throughout UI copy, API responses, database naming, and
documentation:

1. Reports are “unverified customer reports,” never “confirmed outages.”
2. The official status page is the authoritative source for confirmed incidents.
3. Public pages must never reveal which person, customer, organization, account,
   IP address, exact location, or session submitted a report.
4. Public pages must never show free-text report descriptions or raw events.
5. A customer report should be fast enough to submit in under one minute.
6. Store the report before attempting email delivery.
7. Email delivery failure must not cause the customer report submission to fail.
8. Make privacy-respecting behavior the default.
9. Secure public submission endpoints against abuse and automated manipulation.
10. Preserve a clean line between public aggregate signal data and private
    support/administrative information.

============================================================
3. USER ROLES AND MODES
============================================================

Implement these roles:

- Public viewer:
  Can view the public dashboard and aggregate report charts.

- Authenticated reporter:
  Can submit reports. In default production configuration, reporting requires
  authentication, but the dashboard remains public.

- Admin:
  Can manage components, view private reports, configure support email
  recipients, configure the official status page URL, and manage application
  settings.

Implement these deployment modes through environment/config settings:

A. hybrid mode, default:
   - Public dashboard and graphs
   - Authenticated reporting required

B. authenticated-only mode:
   - Dashboard and reporting require login

C. public-reporting mode:
   - Public dashboard and public report submission
   - Require CAPTCHA provider integration if enabled
   - Apply more restrictive rate limits and anti-abuse checks
   - Display an admin warning that public submissions produce lower-trust data

Do not implement CAPTCHA vendor logic unless easy to do cleanly. Instead,
provide a CaptchaProvider interface, a no-op development provider, a clearly
documented Cloudflare Turnstile implementation, and fail closed in public mode
if CAPTCHA is configured as required but unavailable.

============================================================
4. DATA MODEL
============================================================

Use Prisma migrations. Create at least these models:

User:
- id
- email
- name nullable
- role enum: USER, ADMIN
- createdAt
- updatedAt

Organization:
- id
- name
- externalId nullable and unique where practical
- createdAt
- updatedAt

Membership:
- userId
- organizationId
- role enum: MEMBER, ORG_ADMIN
- unique userId + organizationId

Component:
- id
- slug unique
- name
- description nullable
- sortOrder
- isActive
- createdAt
- updatedAt

IssueCategory enum:
- OUTAGE
- ERRORS
- SLOW_PERFORMANCE
- LOGIN_OR_AUTH
- DELAYED_DATA
- INTEGRATION
- OTHER

ImpactLevel enum:
- BLOCKED
- DEGRADED
- MINOR

Report:
- id, preferably UUID
- componentId
- category
- impact
- description nullable, private only, max 2,000 characters
- reporterEmail nullable, private only
- contactConsent boolean
- userId nullable
- organizationId nullable
- ipHash nullable, not the raw IP; use a rotating server-side pepper strategy
- userAgentHash nullable
- dedupeKey
- createdAt
- publicVisibleAt
- emailDeliveryStatus enum: PENDING, SENT, FAILED, SKIPPED
- emailProviderMessageId nullable
- emailLastError nullable, private only
- emailSentAt nullable

ReportAggregate:
- id
- componentId
- category nullable
- bucketStart timestamp
- bucketMinutes integer
- reportCount integer
- updatedAt
- unique componentId + category + bucketStart + bucketMinutes

AppSetting:
- key unique
- value JSON
- updatedAt

AuditLog:
- id
- actorUserId nullable
- action
- entityType
- entityId nullable
- metadata JSON
- createdAt

Use a 5-minute aggregate bucket by default. Keep raw reports private. Generate
public graphs from aggregates, not raw report records.

Implement the aggregation so a report increments the appropriate aggregate in
the same database transaction, using an upsert or equivalent safe pattern.

============================================================
5. DEDUPLICATION AND ABUSE PREVENTION
============================================================

Implement conservative, configurable anti-abuse protections.

Requirements:

- Limit each authenticated user to one counted report per component every 30
  minutes by default. They may still be offered a path to contact support, but
  duplicates should not inflate the public count.
- For public-reporting mode, rate limit by a privacy-preserving IP hash and
  session/device cookie. Default maximum:
  - 3 attempts per IP hash per hour per component
  - 10 attempts per IP hash per day across all components
- Add a hidden honeypot field. Reject or silently discard submissions with it
  filled.
- Enforce server-side request-body and field-length limits.
- Validate all inputs using Zod on the server.
- Add CSRF protection appropriate for the auth/session implementation.
- Use generic error messages that do not reveal whether an email, user, tenant,
  or internal record exists.
- Do not count duplicate submissions in public aggregates.
- Log rate-limit and abuse events privately without storing raw IP addresses.
- Design a RateLimiter interface. Use PostgreSQL for a simple initial
  implementation and document how to replace it with Redis for multi-instance
  deployments.
- Include a README security note explaining that rate limiting is essential for
  public-reporting mode. The implementation should follow the general principle
  of limiting how often a client can call an endpoint, enforcing payload limits,
  and validating server-side inputs.

Do not make unsupported claims that the system is immune to manipulation. In
the UI, distinguish “unverified reports” from a measured availability SLA.

============================================================
6. PUBLIC USER EXPERIENCE
============================================================

Create a professional, accessible, responsive public dashboard at `/`.

Header:
- Product name: SignalStatus
- Tagline: “Customer-reported service signals”
- Link/button to a configurable official status page URL
- Clear label: “Official status is maintained separately.”
- Sign in link when reporting requires auth

Dashboard:
- Intro copy:
  “This page shows anonymized, unverified reports from customers who recently
  experienced trouble. A rise in reports can indicate a broader issue, but it
  is not confirmation of an outage. For verified incidents and maintenance,
  check the official status page.”

- Display active components as cards or rows.
- For each component show:
  - Component name and short description
  - Reports in the last 30 minutes
  - Reports in the last 24 hours
  - A compact sparkline or graph
  - A “Report a problem” button
  - An “Official status” link if configured
- Component detail page at `/components/[slug]`
- Show a 24-hour chart in 5-minute buckets, with a selectable 72-hour window.
- Do not show a numeric report count below a configurable public privacy
  threshold. For example, show “Low report volume” for counts below 3.
  The default threshold should be configurable.
- Public graph data should be delayed by 5 minutes by default. Make the delay
  configurable to reduce the usefulness of real-time manipulation.
- Display all times in the viewer’s local time, with accessible labels/tooltips.
- Include an empty state:
  “No elevated customer reports recently.”
  Do not claim the service is operational merely because there are no reports.

Report modal/page:
- Route: `/report` and optionally `/components/[slug]/report`
- Fields:
  - Component, required
  - Issue category, required
  - Impact level, required
  - Description, optional but clearly encouraged, max 2,000 chars
  - Email, optional for authenticated users and required only if the
    deployment setting requires it
  - “You may contact me about this report” consent checkbox
- Include a short privacy note:
  “Your identity and description are sent privately to the service provider.
  Other visitors only see anonymized, aggregated report counts.”
- Submit button: “Submit report”
- Success message:
  “Your report has been recorded. If you provided contact details, your
  description has also been sent to support. This report is not a confirmation
  of a platform-wide incident.”
- Link to the official status page and a conventional support path.

Accessibility:
- Semantic forms and labels
- Keyboard-operable dialog/modal behavior
- Clear focus styles
- Sufficient color contrast
- Graph has a tabular/text equivalent for screen readers
- Never rely only on red/green color to communicate state

============================================================
7. PRIVATE ADMIN EXPERIENCE
============================================================

Protect all `/admin` routes with the ADMIN role.

Create:

1. `/admin`
   - Overview:
     - reports in last hour, 24 hours, 7 days
     - report counts by component and category
     - email delivery failures
     - a simple “possible spike” indicator, clearly labeled as a heuristic

2. `/admin/reports`
   - Paginated private report table
   - Filter by date range, component, category, impact, organization, email
     delivery status
   - View report detail including description, reporter email, organization,
     timestamps, and email status
   - Never render untrusted description text as HTML
   - CSV export is optional; if implemented, make it admin-only and audit it

3. `/admin/components`
   - Create, edit, activate/deactivate, order components
   - Prevent deletion of a component with reports; allow deactivation instead

4. `/admin/settings`
   - Product name
   - Official status page URL
   - Support email recipient or recipient list
   - From email
   - Public dashboard enabled
   - Reporting mode
   - Public graph delay
   - Privacy display threshold
   - Authenticated dedupe window
   - Rate-limit values
   - Contact/support URL
   - Email enabled/disabled

5. `/admin/audit-log`
   - Basic audit log for admin setting and component changes

Add a simple possible-spike heuristic:
- For each component, highlight if there are at least 3 unique counted reports
  from at least 2 organizations in the last 15 minutes.
- If organization identity is unavailable, use a conservative substitute and
  label it as lower confidence.
- This is internal-only and must not automatically create or publish incidents.

============================================================
8. EMAIL DELIVERY
============================================================

Build an EmailProvider interface with methods such as:

- sendSupportNotification(report)
- optionally verifyConfiguration()

Implement Resend first, using a server-only API key:
- RESEND_API_KEY
- SUPPORT_FROM_EMAIL
- SUPPORT_TO_EMAILS, comma-separated

On report submission:
1. Validate and store the report transactionally.
2. Mark emailDeliveryStatus PENDING if an email should be sent.
3. Enqueue a background job or implement a durable retry mechanism.
4. Send a clear, structured transactional support email.
5. Update the stored report with SENT/FAILED and the provider message ID/error.
6. Retry transient failures with bounded exponential backoff.
7. Do not retry permanent validation/configuration errors indefinitely.
8. If no email provider is configured, store the report and set status SKIPPED.

Support email subject example:
  [SignalStatus] BLOCKED — API — Errors

Support email body must contain:
- Report ID
- Timestamp in UTC
- Component
- Category
- Impact
- Description
- Reporter email, only if supplied
- Contact consent
- Organization name/ID if available
- Private admin report URL
- Explicit notice: “This is one unverified customer report.”

Never expose the mail API key or support recipient list in client-side code,
public APIs, screenshots, logs, or generated example files.

============================================================
9. API DESIGN
============================================================

Create documented API routes, using JSON consistently:

Public:
- GET `/api/public/components`
- GET `/api/public/components/[slug]/report-trend?range=24h|72h`
- GET `/api/public/summary`

Reporting:
- POST `/api/reports`

Admin:
- GET `/api/admin/reports`
- GET/POST/PATCH `/api/admin/components`
- GET/PATCH `/api/admin/settings`
- GET `/api/admin/metrics`

Requirements:
- Authorize every route on the server.
- Return no private report data from public endpoints.
- Apply rate limiting to public endpoints where appropriate.
- Use consistent error shapes.
- Validate query parameters and request bodies.
- Add OpenAPI-style documentation or a concise API section in the README.

Public trend endpoint output must contain only:
- public component identity
- public bucket timestamp
- count after privacy thresholding
- bucket interval
- aggregation/display delay metadata
- unverified-report labeling metadata

Do not return raw report IDs or individual event times from public API endpoints.

============================================================
10. CONFIGURATION
============================================================

Provide `.env.example` with safe placeholder values and comments. Include:

DATABASE_URL=
NEXTAUTH_SECRET=
NEXTAUTH_URL=
APP_BASE_URL=
APP_NAME=SignalStatus
OFFICIAL_STATUS_PAGE_URL=
SUPPORT_URL=
SUPPORT_FROM_EMAIL=
SUPPORT_TO_EMAILS=
RESEND_API_KEY=
REPORTING_MODE=hybrid
PUBLIC_REPORTING_ENABLED=false
PUBLIC_GRAPH_DELAY_MINUTES=5
PUBLIC_REPORT_PRIVACY_THRESHOLD=3
AUTHENTICATED_DEDUPE_WINDOW_MINUTES=30
IP_HASH_PEPPER=
TURNSTILE_SITE_KEY=
TURNSTILE_SECRET_KEY=
DEMO_ADMIN_EMAIL=
DEMO_ADMIN_PASSWORD=

Do not use real credentials. Validate required settings at startup and show
actionable configuration errors only to administrators/server logs.

============================================================
11. SECURITY AND PRIVACY
============================================================

Treat this as an internet-exposed app with a potentially abused submission
endpoint.

Implement and document:

- Strict TypeScript.
- Server-side auth and role enforcement.
- Zod validation.
- Output escaping; no unsafe HTML rendering of descriptions.
- Security headers appropriate for Next.js:
  - Content-Security-Policy
  - X-Content-Type-Options
  - Referrer-Policy
  - frame-ancestors / clickjacking prevention
- CORS restricted to same origin unless intentionally configured.
- CSRF protections appropriate to implementation.
- Secure, HttpOnly, SameSite cookies.
- Rate limits and deduplication.
- Hash IP addresses with a server-side secret pepper; do not store raw IPs.
- Document retention guidance for private descriptions and emails.
- Admin audit logs.
- Clear data deletion path for admins.
- No analytics trackers by default.
- No public reporter identity, description, customer, organization, or exact
  location.
- No email attachments or file uploads in MVP.
- A safe error boundary and useful server-side logging without secrets or
  private report bodies in logs.

Include a `SECURITY.md` explaining how users can report vulnerabilities.

============================================================
12. SEED DATA AND DEMO MODE
============================================================

Create a seed script that adds:

Components:
- Web Application
- API
- Authentication
- Background Jobs
- Webhooks and Integrations

Create development-only demo reports spread over the last 72 hours so the
dashboard graphs are useful immediately.

Create a documented development-only demo admin account. Ensure it is impossible
to accidentally use known demo credentials in a production environment.

============================================================
13. TESTS
============================================================

Write meaningful tests before considering the MVP complete.

Unit tests:
- Report Zod validation
- Deduplication behavior
- Aggregate upsert behavior
- Privacy thresholding
- Public graph delay
- Rate-limit behavior
- Email-provider error handling
- Admin authorization

Integration/end-to-end tests:
- A public viewer can see aggregate component data but cannot access raw reports
- An authenticated user can submit a report
- A duplicate report does not inflate a public aggregate
- A report stores successfully even when the email provider fails
- An admin can add a component and view a private report
- A non-admin cannot access admin routes
- The public API never returns description, email, organization, IP hash,
  user ID, raw report ID, or raw exact report timestamp
- Public mode refuses report submissions if CAPTCHA is required but not valid

============================================================
14. DOCUMENTATION
============================================================

Create a high-quality `README.md` containing:

- What SignalStatus is
- What it is not
- Why it exists
- Architecture diagram in Mermaid
- Screenshots placeholders, not external image URLs
- Requirements
- Quick start
- Docker deployment
- Environment variables
- Admin bootstrap
- Configuration guide
- Authentication/SSO integration approach
- Email-provider setup
- Official-status-page integration guidance
- Public vs authenticated reporting mode trade-offs
- Abuse-prevention and privacy model
- Data model overview
- API summary
- Backups, database migrations, observability, and upgrade notes
- Reverse proxy/TLS guidance
- Clear caveat: a report count is unverified user signal, not uptime monitoring
  or an official incident declaration
- Roadmap ideas that are deliberately not in the initial MVP

Also create:
- LICENSE: Apache-2.0
- SECURITY.md
- CONTRIBUTING.md
- CODE_OF_CONDUCT.md
- `.github/workflows/ci.yml`
- `docs/architecture.md`
- `docs/threat-model.md`
- `docs/deployment.md`

============================================================
15. EXECUTION PLAN
============================================================

Work incrementally, but actually implement the project rather than merely
describing it.

Phase 1:
- Initialize project and toolchain
- Docker Compose/PostgreSQL/Prisma
- Environment validation
- Schema and migrations
- Seed data
- Public dashboard and read-only graph API

Phase 2:
- Authentication and roles
- Component management
- Secure report submission, deduplication, rate limiting, aggregate updates
- Private admin report views

Phase 3:
- Email abstraction, durable delivery state, Resend implementation, retries
- Settings UI
- Security headers and privacy checks
- Tests and documentation

At the end of each phase:
- Run lint, typecheck, unit tests, and relevant end-to-end tests.
- Fix failures rather than leaving TODOs.
- Summarize changed files, commands run, test results, and unresolved decisions.

Before writing code, briefly state:
1. The proposed file/folder structure
2. Any deliberate implementation choices or trade-offs
3. Any assumptions

Then begin Phase 1. Do not wait for more clarification unless a blocker makes
implementation impossible.
```
