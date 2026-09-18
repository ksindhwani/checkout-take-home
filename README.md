# Payment Gateway

A payment gateway API that lets a merchant process a card payment (authorizing it against
an acquiring bank) and retrieve the details of a previously processed payment.

This implements the [Checkout.com take-home assessment](Checkout-README.md). See that file
for the full functional spec. This document covers how the solution is built, how to run it,
and the reasoning behind the design decisions.

## Architecture

```
HTTP request
   │
   ▼
internal/handlers   (decode request / encode response, map errors to HTTP status)
   │
   ▼
internal/service    (validate, call acquiring bank, persist outcome, record metrics)
   │              │
   ▼              ▼
internal/bank   internal/repository   (acquiring bank HTTP client / in-memory store)
```

- **`internal/handlers`** is a thin HTTP transport layer. It has no business logic in it: it
  decodes JSON, calls the service, and maps the result to a status code and response body.
- **`internal/service`** owns the payment workflow: validate the request, call the bank,
  decide Authorized/Declined/Rejected, persist the record, emit metrics. It depends on small
  interfaces (`Repository`, `bank.Client`) rather than concrete types, so it can be tested
  without spinning up HTTP or storage.
- **`internal/validation`** is a set of pure functions, one per field rule from the spec, plus
  an aggregator that collects every failing field instead of stopping at the first one.
- **`internal/bank`** is the acquiring bank client. `Client` is an interface, and `HTTPClient`
  is the implementation that talks to the Mountebank simulator described in the spec.
- **`internal/repository`** is the in-memory, concurrency-safe payment store (a test double for
  a real database, per the spec).
- **`internal/idempotency`** lets a retried `POST /api/payments` (identified by a client-sent
  `Idempotency-Key` header) replay its original outcome instead of triggering a second bank
  call. It sits in the handler layer rather than the service layer, since it's really about
  caching an HTTP response, not making a payment decision.
- **`internal/platform`** holds cross-cutting concerns, structured logging and Prometheus
  metrics, kept out of the handler/service/repository code they instrument.
- **`internal/config`** loads runtime configuration from environment variables with sane local
  defaults, so the service runs with zero configuration out of the box.

There's deliberately no `cmd/` directory, no abstraction beyond a single interface for the bank
client and repository, and no persistent storage. This is a single binary with one HTTP
entrypoint, and the spec is explicit that a real database isn't required.

## Running it

**Everything, in Docker (recommended):**

```bash
docker-compose up --build
```

This starts the bank simulator (`:8080`) and the payment gateway (`:8090`) on a shared
network, with the gateway configured to reach the simulator by its container hostname.

**Gateway locally, simulator in Docker:**

```bash
docker-compose up bank_simulator
go run main.go
```

The gateway listens on `:8090` by default and talks to the simulator at
`http://localhost:8080` by default. See [Configuration](#configuration) to change either.

**From VS Code:** `.vscode/launch.json` has a "Launch Payment Gateway" configuration that
starts the bank simulator automatically (via `.vscode/tasks.json`) and runs the gateway under
the debugger, breakpoints included. It also has configurations for debugging the tests in
whichever package you have open, and for the bank simulator integration tests specifically.

Once running:
- API: `http://localhost:8090/api/payments`
- Swagger UI: `http://localhost:8090/swagger/index.html`
- Prometheus metrics: `http://localhost:8090/metrics`
- Health check: `http://localhost:8090/ping`

## Configuration

All configuration is via environment variables, each with a default that makes local
development work with zero setup. For local development, `main.go` also loads a `.env` file
if one exists (via [`godotenv`](https://pkg.go.dev/github.com/joho/godotenv)), so you can edit
one file instead of exporting variables in your shell every session. Copy `.env.example` to
`.env` to get started; `.env` itself is gitignored, and a missing one is a harmless no-op, real
environment variables (as set by `docker-compose.yml`, CI, etc.) always take precedence over
anything in the file.

| Variable                   | Default                 | Purpose                              |
|-----------------------------|--------------------------|---------------------------------------|
| `PORT`                      | `8090`                   | Port the gateway listens on           |
| `BANK_SIMULATOR_BASE_URL`   | `http://localhost:8080`  | Base URL of the acquiring bank        |
| `BANK_CALL_TIMEOUT`         | `5s`                     | Timeout for a single call to the bank |
| `LOG_LEVEL`                 | `info`                   | `debug` / `info` / `warn` / `error`   |

## Setting it up and testing it manually

Everything below assumes only Docker and Docker Compose are installed, no Go toolchain
required if you're happy running it in containers.

**1. Clone and start the stack:**
```bash
git clone <this-repo-url>
cd checkout-take-home
docker-compose up --build
```
This builds the gateway image and starts it alongside the bank simulator. Give it a few
seconds, then confirm it's up:
```bash
curl http://localhost:8090/ping
# {"message":"pong"}
```

**2. Authorize a payment.** The bank simulator authorizes any card number ending in an odd
digit (1, 3, 5, 7, 9):
```bash
curl -i -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" \
  -d '{
    "card_number": "2222405343248871",
    "expiry_month": 4,
    "expiry_year": 2030,
    "currency": "GBP",
    "amount": 100,
    "cvv": "123"
  }'
```
Expect `201 Created` with `"status":"Authorized"`. Copy the `id` from the response, you'll
need it for step 5.

**3. Decline a payment.** Same request, card number ending in an even digit (2, 4, 6, 8):
```bash
curl -i -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" \
  -d '{
    "card_number": "2222405343248872",
    "expiry_month": 4,
    "expiry_year": 2030,
    "currency": "GBP",
    "amount": 100,
    "cvv": "123"
  }'
```
Expect `201 Created` with `"status":"Declined"`, still a successfully-processed payment, just
not an authorized one.

**4. Trigger a bank failure.** Card number ending in `0` makes the simulator return a `503`:
```bash
curl -i -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" \
  -d '{
    "card_number": "2222405343248870",
    "expiry_month": 4,
    "expiry_year": 2030,
    "currency": "GBP",
    "amount": 100,
    "cvv": "123"
  }'
```
Expect `502 Bad Gateway`. Nothing gets persisted for this one, see
[Design decisions and assumptions](#design-decisions-and-assumptions) for why.

**5. Reject an invalid request** (never reaches the bank):
```bash
curl -i -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" \
  -d '{
    "card_number": "123",
    "expiry_month": 13,
    "expiry_year": 2020,
    "currency": "JPY",
    "amount": 0,
    "cvv": "12"
  }'
```
Expect `400 Bad Request` with every invalid field listed under `error.fields`.

**6. Retrieve the payment from step 2:**
```bash
curl http://localhost:8090/api/payments/<id-from-step-2>
```
And confirm a missing one 404s:
```bash
curl -i http://localhost:8090/api/payments/does-not-exist
```

**7. Try the idempotency flow** (same key replays instead of reprocessing; a different body
under the same key is rejected):
```bash
KEY="manual-test-$(date +%s)"

curl -i -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" -H "Idempotency-Key: $KEY" \
  -d '{"card_number":"2222405343248871","expiry_month":4,"expiry_year":2030,"currency":"GBP","amount":500,"cvv":"123"}'
# 201, note the id

curl -i -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" -H "Idempotency-Key: $KEY" \
  -d '{"card_number":"2222405343248871","expiry_month":4,"expiry_year":2030,"currency":"GBP","amount":500,"cvv":"123"}'
# 201, same id as above — replayed, not reprocessed

curl -i -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" -H "Idempotency-Key: $KEY" \
  -d '{"card_number":"2222405343248871","expiry_month":4,"expiry_year":2030,"currency":"GBP","amount":999,"cvv":"123"}'
# 422, same key reused with a different body
```

**8. See it beyond curl:**
- Import [`Payment-Gateway.postman_collection.json`](Payment-Gateway.postman_collection.json)
  into Postman for a point-and-click version of everything above, including the idempotency
  flow with the key already wired up via a pre-request script.
- Swagger UI at `http://localhost:8090/swagger/index.html` for interactive, schema-documented
  requests.
- `http://localhost:8090/metrics` to watch `payments_total{status="..."}` and
  `bank_call_duration_seconds` change as you make requests above.

**9. Shut it down:**
```bash
docker-compose down
```

## Testing

```bash
make test              # unit + handler + service tests, with -race
make test-integration  # requires: docker-compose up -d bank_simulator
```

- **Validation** (`internal/validation`) is covered by table-driven tests over every field
  rule, including boundaries (13/14/19/20-digit card numbers, current vs. past expiry month,
  unsupported currencies, zero/negative amounts, 2/3/4/5-digit CVVs).
- **Service** (`internal/service`) is tested with a fake `bank.Client` and the real in-memory
  repository, covering Authorized, Declined, Rejected (never reaches the bank), and
  bank-unavailable (nothing persisted) paths.
- **Bank client** (`internal/bank`) is unit-tested against an `httptest.Server` for every
  response shape, including timeouts and connection failures. A separate test file, gated
  behind the `integration` build tag, exercises the real Mountebank simulator started by
  `docker-compose`. It's excluded from the default `go test ./...` run so the suite stays fast
  and has no external dependency by default.
- **Handlers** (`internal/handlers`) are tested end-to-end through the router with `httptest`,
  covering the full status-code matrix (201/400/404/502) and malformed JSON.
- **Repository** has a concurrent read/write test, run with `-race`, since it backs a server
  that will receive concurrent requests.
- **Idempotency** (`internal/idempotency`) has unit tests for the store's reserve/complete/
  release semantics, including a concurrent-`Reserve` test (`-race`) proving only one caller
  can ever claim a given key. The handler tests go further and exercise the full behaviour over
  HTTP: a replayed request returns the identical cached response without a second bank call, a
  reused key with a different body gets `422`, a genuinely concurrent duplicate gets `409`
  (proven deterministically with a channel-gated fake bank client rather than a sleep), and a
  key released after a bank failure can be retried.

## Design decisions and assumptions

**Currency allowlist.** The spec caps validation at no more than 3 ISO-4217 codes, so this
implementation accepts `GBP`, `USD`, `EUR`. It's a fixed business rule in
`internal/validation` rather than deployment configuration, since adding a currency should be
a one-line, tested code change, not an operational one.

**Amount must be a positive integer.** The spec only states amount "must be an integer,"
it doesn't say anything about zero or negative values. A payment for `0` or a negative amount
isn't a real transaction, so `ValidateAmount` rejects anything `<= 0`. This is an assumption
beyond what's explicitly written in the spec, called out here for that reason.

**Bank simulator errors (503, timeout, unreachable) return `502 Bad Gateway`, and nothing is
persisted.** The spec defines exactly three outcomes: Authorized, Declined, Rejected, and none
of them accurately describes "we don't know what happened." Authorized and Declined are both
bank decisions; a 503 or timeout is the absence of one. Recording it as Declined would
misrepresent it for merchant reconciliation, and there's no way to tell the shopper "your card
was declined" apart from "please try again." So this gateway treats it as its own failure mode:
`502` to the merchant, no payment record created, and a `bank_call_errors_total` metric
incremented so it's visible operationally.

**`201 Created` for both Authorized and Declined.** Both are a successfully created payment
resource in a terminal state, retrievable afterwards via `GET /api/payments/{id}`. Only a
`Rejected` request (`400`) creates nothing.

**Rejected responses return per-field validation errors**, for example
`{"error":{"fields":{"cvv":"must be 3-4 characters long"}}}`, aggregating every failing field
instead of just the first. It's a small amount of extra plumbing for a much more useful
integration experience.

**CVV and last-four card digits are strings, not integers**, unlike the original template. A
`012` CVV or a `0034` last-four is a valid value whose leading zero an `int` would silently
drop, and a JSON number literal with a leading zero isn't even valid JSON in the first place,
so a client couldn't send it that way regardless. Both need to be validated and stored as
strings.

**The CVV is never persisted or returned.** It's used exactly once, to build the bank request,
and it doesn't appear in `models.Payment` or the repository at all: never logged, never
stored. The full card number is treated the same way; only the last four digits make it past
the service layer, per the spec's compliance note.

**One `Payment` type serves as both the persisted record and the API response**, rather than
separate domain/DTO structs with a mapping layer between them. There's currently no field that
needs to exist in storage but never reach the wire, or vice versa, so introducing that split
now would be speculative complexity.

**The in-memory repository is a `map[string]Payment` guarded by a `sync.RWMutex`**, not the
original linear-scan slice with no locking. This isn't really an enhancement so much as a
correctness fix: an HTTP server receives concurrent requests, and an unsynchronized slice
append is a genuine data race, not a hypothetical one, which `go test -race` confirms.

**Idempotency keys protect against double-processing on retry.** A `502` from the bank being
unavailable is a designed outcome of this system (see above), and a merchant retrying that
request is expected, standard client behaviour. Without protection, though, a retry means a
second, independent authorization attempt against the same card for the same logical payment,
which is a far worse failure than most anything else this system could get wrong. So an
optional `Idempotency-Key` header on `POST /api/payments` is supported:
- First request with a key: processed normally, and the terminal outcome (Authorized,
  Declined, or Rejected) is cached against the key.
- Retry with the same key **and the same request body**: the cached response is replayed
  as-is, and the bank isn't called again.
- Retry with the same key but a **different** body: `422 Unprocessable Entity`, since reusing
  a key for a different payment is a client bug, not something to silently allow.
- A **concurrent** duplicate (a second request arrives while the first is still in flight):
  `409 Conflict`, rather than racing both through to the bank.
- If the original attempt resulted in `502`/`500` (an unknown outcome), the key is released
  instead of cached, so a retry can actually re-attempt the bank call rather than replay the
  failure forever.

This is a plain in-memory map guarded by a mutex, matching the rest of this take-home's
storage. There's no TTL or eviction, which a long-lived real deployment would need but is out
of scope for a single process's lifetime here.

**Metrics are exposed, not shipped with a scraper.** `/metrics` serves Prometheus exposition
format via `client_golang`, and no Prometheus/Grafana container is bundled. The point being
demonstrated is that the service is instrumented (per-outcome counters, a bank-call latency
histogram, a bank-error counter) in a standard, pluggable format, not that a monitoring stack
can be stood up, which would be infrastructure for infrastructure's sake in a take-home with
an in-memory store and no real dependencies to monitor.

**Graceful shutdown uses a fresh context, not the cancelled one.** The original template
called `httpServer.Shutdown(ctx)` with the same `ctx` that had just been cancelled by the
interrupt handler. `Shutdown` given an already-done context can abort immediately instead of
draining in-flight requests, so shutdown now gets its own 10-second timeout context.

## What I'd do with more time

- **A real datastore**, with the repository interface unchanged since the service layer
  doesn't know or care that storage is in-memory today. The same goes for the idempotency
  store, which would also need a TTL/eviction policy rather than keys living forever.
- **Retry with backoff, or a circuit breaker**, around the bank call, instead of failing fast
  on the first error.
- **AuthN/AuthZ**, e.g. per-merchant API keys. There is currently none, matching the spec's
  scope.
- **Distributed tracing** (OpenTelemetry), to follow a request across the gateway and the
  simulated bank, complementing the metrics and structured logs that already exist.
