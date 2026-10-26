# Data handling and reporting

Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT.

Use HTTPS for real login and medical data. Keep the Go listener on localhost behind the app-specific reverse proxy. The application stores configuration only, but login credentials and medical data still pass through its memory.

Preserve data/identity.key with configuration backups and protect exported snapshots. Never attach actual credentials, reports or unsanitized logs to a public issue. Report a suspected vulnerability privately to the maintainer through a contact channel they have explicitly provided; this repository does not invent an email address or promise a response SLA.

Current boundaries: fixed official HTTPS origin, per-session cookie jars, manual CAPTCHA, same-origin/CSRF-protected writes, request limits, no uploaded-script execution or report persistence. These controls do not make the application a certified medical device or eliminate host-level responsibilities.
