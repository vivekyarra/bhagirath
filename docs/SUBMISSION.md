# Submission draft (Project Submission issue form)

Fill every `[…]` with real values. Every number must trace to `pilot/METRICS.md`, `backtest/RESULTS.md` or a source link that has been re-opened. If a pilot number is small, report it as it is.

**Project name:** Bhagirath (भगीरथ)
**Track:** 05 — Clean Water
**Repository:** https://github.com/vivekyarra/bhagirath (Apache-2.0)
**Live demo:** [URL] — login `judge` / `[password]` (fictional data only; open ▶ Demo for the seven-click story)
**Demo video:** [link, checked in an incognito window]

**One-line summary:** Bhagirath connects dirty-water complaints (missed call, voice note, WhatsApp), water tests and clinic illness counts, alerts by a frozen published rule, escalates to officials with deadlines, and counts a fix only when the resident confirms it.

**Problem:** In Bhagirathpura, Indore, residents complained about dirty water for months before an outbreak in December 2025 killed at least 16 people. About 700 Indore complaints sat "under resolution", and some were closed without repair. Complaints, tests and illness lived in three disconnected systems, and no one had to prove a fix.

**What is agentic:** five agents — Intake (ASR, extraction, location, follow-up questions), Fusion (deterministic rule), Escalation (ladder, evidence pack, timers), Closure Verifier (evidence plus resident callback; reopens fake closures), Advisory (precautionary messages in the household's language). An LLM assists with language; alert decisions are deterministic and backtested.

**Deployment & impact data:**
- Pilot: [N] wards/villages with [partner names], running since [date] ([N] active days).
- From `pilot/METRICS.md` (exported [date]): households informed [N] · complaints [N] ([N] from non-smartphone channels) · distinct reporters [N] · escalations [N], acknowledged [N%] · verified fixes [N] · false closures caught [N] · median days to verified fix [x] vs baseline [y] · clinics/chemists/ASHAs reporting [N] · speech recognition WER [x%] on [N] recordings.
- Backtest: the rule was frozen in commit d005214 before data collection. On Indore, using only the three individually dated helpline complaints at their latest plausible dates, it raises a Watch on 27 Nov 2025 and reaches the commissioner by 3 Dec 2025, 23 days before the first claimed death. Multi-signal alerts fire only once the outbreak began, and for three other outbreaks the public data would not have given an early warning (see backtest/REPORT.md).
- Testimonials: [partner name, role]; [official or doctor name, role] (with permission).

**Built with:** Go, PostgreSQL, WhatsApp Cloud API, Twilio/Exotel, Sarvam ASR, Leaflet/OpenStreetMap; built in AgentFoundry [details].
