# Definition of Done — status

This tracks spec §16 honestly. **Built** means implemented, tested and in `main`. **Team** means it needs people, accounts or field work that software cannot do.

## Build
| Item | Status |
|---|---|
| Project created in AgentFoundry; collaborators invited; screenshots | **Team** — create the AF project, import this repository, save screenshots to `docs/screenshots/` |
| Missed call / IVR intake end to end | **Built** (Twilio missed call → callback IVR; Exotel Passthru flow). **Team:** buy and verify a number, then test with a real phone. The volunteer line works today. |
| WhatsApp voice-note intake | **Built** (Cloud API webhook, media download, ASR or review). **Team:** Meta business verification and number. |
| Web form intake | **Built** (four languages, GPS, consent, honeypot, rate limit) |
| Hindi + English + local-language transcription and extraction, 50+ real recordings with error rate | **Built:** extraction in 9 language forms; Sarvam ASR; WER/CER computed from volunteer corrections and exported. **Team:** collect 50+ real recordings in the pilot. |
| Location resolved to the correct ward for 95%+; follow-up when unclear | **Built:** GPS, landmarks, registered address, follow-up question, ward-correction metric. **Team:** add the pilot ward's real landmarks. |
| Water-test and illness ingestion (manual + WhatsApp) | **Built:** forms, CSV import, clinic WhatsApp parser |
| Alert engine on schedule with v1 rule and reason text | **Built:** every 15 min and after every signal |
| Escalation: evidence pack, deadline, auto-escalation | **Built**, plus acknowledged-but-unfixed escalation and grievance-number tracking |
| Closure needs evidence + callback; denial reopens and escalates | **Built** |
| Advisories in the household's language with the label | **Built** (en/hi/mr/te; partner sign-off; reviewer interlock). **Team:** doctor review of the text. |
| Dashboard: map, alerts, complaints, timers | **Built** |
| Public impact page with anonymised aggregates | **Built** |
| Automated tests for each level, timeout, closure verification | **Built** — `make check` in CI on every PR, plus a randomised whole-system simulation, property tests and fuzzers (`docs/TESTING.md`) |

## Deploy
| Item | Status |
|---|---|
| Public cloud URL, works on a phone on mobile data | **Team** — `render.yaml` / `fly.toml` are ready (see `docs/DEPLOY.md`); no cloud credentials were available while building |
| Demo login for judges | **Built** — `judge` account in demo mode; set the password at deploy |
| Uptime checked on submission day and after | **Team** — external monitor on `/healthz` |

## Proof
| Item | Status |
|---|---|
| `rule-v1-frozen` created before Indore data collection | **Built** — commit `d005214` pushed before research began (branch `freeze/rule-v1`). **Team:** push the tag with `git push origin rule-v1-frozen`; this environment's git proxy blocked tag pushes. |
| Backtests on Indore + 2 more, with sources, both scenarios and limitations | **Built** — Indore, Shimla, Gandhinagar, Greater Noida; `backtest/REPORT.md`. **Team:** re-open every source link before submitting (spec §16). |
| Pilot every day from Day 2, in 2+ wards with named partners | **Team** — see `docs/PILOT_PLAYBOOK.md`. Software support: volunteer line, manual-log import with original dates, `/flyer` with QR, outreach and baseline logs |
| Section 3.2 targets met or shortfalls reported | **Team** — `pilot/METRICS.md` reports real numbers automatically |
| METRICS.md exported from the database with date | **Built** — `bhagirath export-metrics`; the committed file is a pre-pilot export (all zeros) |
| 2+ testimonials with permission | **Team** |

## Open source
| Item | Status |
|---|---|
| Public repo, Apache-2.0 | **Built** (license). **Team:** make the repository public. |
| README per §15.1 | **Built** — the TODO markers for URL, video, team, AgentFoundry and advisory reviewer need team input |
| No secrets, real numbers or audio in repo/history | **Built** — fictional `+9100000xxxxx` numbers only; backtest names redacted |

## Demo & submission
| Item | Status |
|---|---|
| 2–3 minute video (spec §17) | **Team** — the guided demo (▶ Demo) follows the script step by step |
| Every number traced; issue form filled; submitted by 14 Nov | **Team** — draft in `docs/SUBMISSION.md` |
