# Connecting real channels

Bhagirath runs without any provider: the web form and the **volunteer phone line** (dashboard → Volunteer line) always work. That is the spec's fallback if KYC for a telephony or WhatsApp provider is blocked. Each provider below is optional and independent. **Settings** in the dashboard shows which ones are configured.

All webhook URLs below use your public `BHAGIRATH_BASE_URL`.

## WhatsApp Cloud API (Meta)

1. Create a Meta app with the WhatsApp product and register a business phone number.
2. Set `WHATSAPP_TOKEN` (a permanent system-user token), `WHATSAPP_PHONE_NUMBER_ID`, `WHATSAPP_APP_SECRET` and a random `WHATSAPP_VERIFY_TOKEN`.
3. Webhook URL: `https://<host>/webhooks/whatsapp`, with the same verify token. Subscribe to the **messages** field.
4. Messages sent outside the 24-hour customer-service window need approved templates. Create one for advisories and one for callbacks, with the same wording as `internal/i18n`, and set `WHATSAPP_ADVISORY_TEMPLATE` and `WHATSAPP_CALLBACK_TEMPLATE`.

What arrives: text, voice notes (downloaded, then transcribed or queued for review), location pins, and button replies (`cb:<closure>:yes|no`). Registered clinics, chemists and ASHAs send counts such as `D 3 V 1 ORS 12` or `दस्त 3 उल्टी 1`.

Every webhook body is checked against `X-Hub-Signature-256`; unsigned requests get a 401. Delivery receipts mark advisories as delivered.

**Officials on WhatsApp.** If the ladder file gives an official a `phone` and `preferred_channel: whatsapp`, their evidence packs arrive on WhatsApp with an **Acknowledge** button. They can tap it, or reply `ACK ESC-000123`, and the escalation is acknowledged without opening a link. Their number is matched by keyed hash, and an official can acknowledge only escalations addressed to them. Messages from officials are never treated as resident complaints.

## Twilio (missed call → free IVR callback)

1. Buy a number that can receive calls. For Indian numbers, complete Twilio's regulatory bundle; if that is blocked, use Exotel below.
2. Set `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN` and `TWILIO_FROM_NUMBER`.
3. Number → Voice → "A call comes in": **Webhook** `POST https://<host>/voice/twilio/incoming`.

Flow: the resident rings → Bhagirath **rejects** the call, so the caller pays nothing → calls back within seconds → language menu (1 Hindi, 2 English, 3 Marathi, 4 Telugu) → consent (1 = allow callbacks) → records up to 2 minutes → thanks. The recording is downloaded only from `api.twilio.com`, then transcribed or queued for review. One callback per number every 10 minutes.

Verification calls use `<Gather>`: 1 means fixed, 2 means not fixed. The answer counts only when it comes from the complainant's own number. Every request is checked against `X-Twilio-Signature`.

## Exotel (Indian ExoPhone)

Set `EXOTEL_SID`, `EXOTEL_API_KEY`, `EXOTEL_API_TOKEN`, `EXOTEL_SUBDOMAIN`, `EXOTEL_CALLER_ID` (the ExoPhone), `EXOTEL_APP_ID` (the callback flow) and a random `EXOTEL_WEBHOOK_KEY`. Then build two flows in the Exotel App Bazaar:

- **Inbound flow on the ExoPhone:** Passthru → `https://<host>/voice/exotel/missed?key=<KEY>`, then Hangup. This is the free missed call; Bhagirath calls back with the callback flow.
- **Callback flow (`EXOTEL_APP_ID`):** Greeting (consent prompt) → Gather (1 digit) → Passthru `/voice/exotel/consent?key=<KEY>` → Greeting (record prompt) → Record → Passthru `/voice/exotel/recording?key=<KEY>` → Greeting (thanks).
- **Verification calls** use the same app with `CustomField=closure:<id>`: Greeting → Gather → Passthru `/voice/exotel/callback?key=<KEY>`.

Recordings are downloaded only from Exotel or AWS hosts.

## Speech-to-text (Sarvam AI)

Set `SARVAM_API_KEY` (model `saarika:v2.5` by default; the language is detected automatically). Without a key, or if a request fails, recordings go to **Review**. A volunteer types the transcript there, and if a machine transcript existed, the word and character error rates are logged. The review page publishes WER/CER by language — the spec's "50+ recordings, error rate measured".

## Optional LLM (extraction and translation only)

Set `LLM_PROVIDER=anthropic` (`LLM_MODEL` defaults to `claude-haiku-5-5`), or `LLM_PROVIDER=openai` with `LLM_BASE_URL` pointing at any OpenAI-compatible endpoint, plus `LLM_API_KEY`. Its output is validated, and any failure falls back to the deterministic extractor. **It never influences alert levels.**

## Email to officials

Set `SMTP_HOST`, `SMTP_PORT` (587 for STARTTLS, 465 for TLS), `SMTP_USER`, `SMTP_PASSWORD` and `SMTP_FROM`. Officials receive the evidence summary and their personal link to acknowledge, upload a repair photo or a retest, and see the deadline.

Officials without a direct contact get `preferred_channel: grievance` in the ladder file. The partner then files the complaint on the CM Helpline, CPGRAMS or the municipal portal and records the grievance number on the escalation page. Bhagirath tracks it like any other escalation.
