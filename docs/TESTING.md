# How Bhagirath is tested

A system that tells officials a ward is in danger, and tells residents a fix is real, has to be right. CI runs everything below on every pull request (`make check` runs the same locally). Nothing merges to `main` unless it is green.

| Layer | What it proves | Where |
|---|---|---|
| **Rule unit tests** | Every level (Watch, Warning, Critical, and every route to Critical); both complaint sub-conditions; window edges to the hour; retest clearing; no look-ahead; anonymous and same-household gaming; ORS baselines including zero and thin history | `internal/rule/rule_test.go` |
| **Rule properties** (thousands of random wards) | Cluster existence matches an O(n³) brute force; adding a failed test, an illness case or a complaint never lowers the level; verifying a fix never raises it; shifting all times together never changes the verdict; input order never matters | `internal/rule/property_test.go`, `rule_test.go` |
| **Frozen-rule guard** | `alert_rule_v1.yaml` matches its pinned SHA-256; an edited v1 fails | `internal/rule/frozen_test.go` |
| **Whole-loop scenario** | Complaint → Watch → Warning → advisories wait for sign-off → timeouts climb the ladder → fake fix caught → real fix verified → Critical | `internal/agents/loop_test.go` |
| **Edge scenarios** | No answer after three calls reopens the complaint; no consent requires a field visit; follow-up location questions; pins sent before or after the text; same-household de-duplication; alert resolution closes escalations; an acknowledged complaint that is never fixed escalates; the advisory reviewer interlock | `internal/agents/edge_test.go` |
| **Randomised whole-system simulation** | Random sequences of complaints, replies, tests, clinic counts, acknowledgements, fix claims, callbacks, approvals and clock jumps, with **17 database invariants and rule agreement checked after every step**. For example: no complaint is verified without a confirmed callback; reopen counts equal rejected claims; no escalation stays open on a fixed complaint; no advisory goes to a household without consent; no phone number appears in plain text; the active alert always equals what the frozen rule says now. | `internal/agents/sim_test.go` |
| **Concurrency** | 40 parallel intakes into one ward under `-race` produce exactly one alert and no lost writes | `TestConcurrentIntakeStress` |
| **HTTP and security** | Login, roles and CSRF; a forged or expired form is refused; the rate limiter cannot be bypassed with spoofed `X-Forwarded-For`; WhatsApp signatures; Twilio's documented signature vector; the SSRF guard on recordings; official links (forged tokens, no claim without evidence); recordings restricted by role; mislabelled uploads never rendered inline; security headers; the simulator locked out in pilot mode | `internal/web/web_test.go`, `internal/telephony`, `internal/whatsapp` |
| **Fuzzing** (separate CI job) | The WhatsApp webhook parser, the clinic-count parser, location resolution, the backtest CSV reader, the rule YAML parser and the multilingual extractor never panic and never accept invalid output | `Fuzz*` functions |
| **Language safety** | Every resident message exists in all four languages with the same placeholders; every advisory has boil/ORS/danger-sign content and the "not a government order" label | `internal/i18n` |
| **Backtest reproducibility** | `backtest/RESULTS.md` is regenerated from the data and the frozen rule, and must match byte for byte | `internal/backtest` |
| **Static analysis** | gofmt, `go vet`, staticcheck, govulncheck | CI |
| **Container** | The image builds, starts against a fresh Postgres, migrates and passes `/healthz` | CI `docker` job |

## Results recorded during development

- **Randomised simulation:** 3 seeds × 400 steps in CI; a longer local run of 6 seeds × 3,000 steps (18,000 random actions) passed with every invariant holding. The simulation found one real bug: a verified fix did not re-evaluate the ward until the next 15-minute tick, so a stale alert stayed up. It is fixed, and the invariant now guards it.
- **Load test** (`go run ./tools/loadtest`, a single container with local Postgres): 2,400 mixed requests from 40 concurrent senders, ~5,500 requests/s, zero errors, p99 under 25 ms. Every accepted WhatsApp message produced exactly one record, and all 771 pinned complaints were located by GPS. The load test found three issues, all fixed: a pin racing its text, a stray complaint created from a pin, and slow public-page aggregation.
- **Browser walkthrough:** a headless Chromium run of the seven-step guided story and every dashboard page, with zero console errors.

Reproduce the long simulation:

```bash
BHAGIRATH_SIM_STEPS=3000 BHAGIRATH_SIM_SEEDS=6 BHAGIRATH_TEST_DATABASE_URL=postgres://… go test -run Randomized -timeout 30m ./internal/agents/
```
