# Security Policy

## Reporting a vulnerability in SENTRIX
Please report privately to the developer through the contact options on
[![Telegram](https://img.shields.io/badge/Telegram-Contact-blue?style=flat-square&logo=telegram)](http://t.me/monstr_m1nd)
[![Instagram](https://img.shields.io/badge/Instagram-Follow-purple?style=flat-square&logo=instagram)](https://www.instagram.com/httpx.mrmonsif/) rather than opening a public issue. Include
the version (`sentrix version`), your OS, and steps to reproduce.

## Design notes
- SENTRIX is passive: it queries third-party public datasets and never sends
  requests to the target.
- API keys are read from environment variables or the config file (mode 0600)
  and are masked in all output.
- Project files carry a SHA-256 checksum that detects accidental corruption and
  casual edits. It is not a signature and does not protect against a malicious
  local user who can rewrite both data and checksum.
- Data from third-party sources is untrusted: HTML reports escape it, and URLs
  are parsed defensively. Report any case where hostile data breaks this.
