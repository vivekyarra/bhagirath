# Security policy

Please report vulnerabilities privately through GitHub Security Advisories on this repository, not in public issues. We aim to respond within 72 hours.

In scope: anything that could expose residents' phone numbers or recordings, forge an official's acknowledgement or fix claim, bypass the resident confirmation that a fix requires, send messages as Bhagirath, or alter alert decisions.

Design notes: phone numbers are AES-256-GCM encrypted with an HMAC for de-duplication; official links are 256-bit bearer tokens stored only as SHA-256 hashes; webhooks are signature-verified; every state change is written to an append-only audit log in the same transaction.
