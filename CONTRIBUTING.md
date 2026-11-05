# Contributing

1. Open an issue describing the change.
2. Branch from `main`; never push to `main` directly.
3. Run `make check` (gofmt, vet, staticcheck, race tests against Postgres) before pushing.
4. Open a pull request. CI must be green before it is squash-merged.

Ground rules:
- **Never edit `config/alert_rule_v1.yaml`.** A rule change means `alert_rule_v2.yaml`, a new version and a new backtest.
- Never commit real phone numbers, audio, residents' names or officials' private contacts. Seed data uses `+9100000xxxxx`.
- Resident-facing text lives in `internal/i18n` and must exist in every supported language; the tests enforce this.
- Bhagirath must never present itself as a government body.
