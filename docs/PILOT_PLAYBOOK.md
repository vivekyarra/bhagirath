# Pilot playbook

The pilot is what wins: real residents, real complaints and real verified fixes. Everything below is logged in Bhagirath, so `METRICS.md` is exported from data rather than counted by hand.

## Day 1–3: partner and setup

1. **Partner.** In order of preference: a resident group, councillor or NGO in an affected ward; a ward with known complaints in your own city; or a village where a water NGO already works. Outreach message (adapt it):
   > "We're building Bhagirath, a free open-source tool that lets residents report dirty water by missed call or voice note, connects those complaints with water tests and illness reports, and follows up with officials until the problem is verifiably fixed. After the Indore tragedy, we want to test it in one ward for two weeks with your help. You keep all data and control what gets sent. Could we speak for 15 minutes?"
2. **Ward file.** Copy `config/ladder.example.yaml` to `config/ladder.local.yaml` (git-ignored). Then:
   - Draw the ward boundary: four or more `[lat, lon]` points, taken from municipal open data or walked with the partner.
   - Add 6–12 **landmarks** residents actually say (temple, school, ration shop, water tank) with Hindi and local-language aliases. Spoken locations resolve through these.
   - Add officials **only from public sources or the partner**. If there is no direct contact, use `grievance_channel: "CM Helpline 181"`.
3. **Go live on Day 2 even before telephony is approved.** Use the web form plus the **volunteer phone line** (dashboard → Volunteer line). Every call a volunteer logs counts as a non-smartphone complaint.
4. **Advisory review.** Show the four advisory texts (`internal/i18n/i18n.go`, key `advisory`) to a doctor or public-health professional, then set `BHAGIRATH_ADVISORY_REVIEWED_BY="Dr …, …"`. Until that is done, pilot mode refuses to send advisories.

## Baseline (before switching on)

Ask 10 residents and the partner, and record each answer under **Pilot → Baseline**:
- "The last time you complained about water, where did you complain?"
- "How many days until it was fixed?" (If it never was, record the days waited and choose *No, still waiting*.)
- If helpline data is available, enter the typical resolution time with source *helpline_data*.

The median of these is the "before" on the impact page.

## Every day

- **Outreach:** after each door-to-door round, flyer drop or WhatsApp-group post, log the households reached (Pilot → Log outreach).
- **Clinics, chemists and ASHAs:** enrol at least 5 (Tests & illness → Enrol). Each sends one WhatsApp a day, for example `D 2 V 0 ORS 14` — counts only, never names.
- **Review queue:** transcribe or correct every voice note. This drives the published speech-recognition error rate (target: 50+ recordings).
- **Water tests:** enter field-kit or lab results, and import the JJM WQMIS village results as CSV if the ward is rural.
- **Escalations:** check the timers. When an official answers by phone, record the acknowledgement; when you file on a grievance portal, record the number.
- **Closures:** if a resident cannot be called (no consent), visit and record the result.

## Consent script (volunteer line)

> "Bhagirath is a free citizen service, not the government. May we call you back later only to check whether the water problem was really fixed? Your number stays private and is never shown to officials. Yes or no?"

## Evidence for the submission

- Dated photos or screenshots of the flyer, the volunteer line and the dashboard (with consent; no faces without permission).
- Two or more testimonials (the partner, plus an official or doctor), with written permission to quote name and role.
- On submission day, run `bhagirath export-metrics -o pilot/METRICS.md` against the pilot database and commit the file.

## Printable flyer

Open `/flyer?ward=<ward-id>` on the deployment and print it, or save it as a PDF. It is a bilingual A5 flyer with the missed-call number (`BHAGIRATH_MISSED_CALL_NUMBER`) and a QR code that opens the report form with the ward already selected. A static version is in [`docs/flyer.html`](flyer.html).
