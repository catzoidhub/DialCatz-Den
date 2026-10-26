# Security

DialCatz Den holds a team's shared API collections, who its members are and the tokens of their devices, on a server
the team runs itself. A security problem in it is taken seriously.

## Reporting a vulnerability

**Please do not open a public issue.** Report it privately instead:

1. Open the repository's **Security** tab → **Report a vulnerability**
   ([direct link](https://github.com/catzoidhub/DialCatz-Den/security/advisories/new)). Only the maintainer sees it.
2. Say what is affected, how to reproduce it, and what an attacker could do with it. A proof of concept helps.

Security reports are read before anything else, as soon as the maintainer can — there is no fixed response time yet.
Together we agree on a fix and when to publish it; you are credited in the advisory and the release notes unless you
would rather not be.

## What is in scope

- The sync API (`/api/v1/…`) — anything that lets a token reach a workspace it is not allowed, act as another member,
  or survive being revoked.
- Invite links and the code exchange — reusing a code, guessing one, or getting a token without one.
- The admin panel and its API (`/api/admin/…`) — signing in without the password or the second factor, or doing
  admin work as a member.
- What is stored: passwords (argon2id), tokens (only their SHA-256 hash), sealed secrets (AES-GCM) — anything that
  leaks them in clear, in logs or in a backup.
- The image `ghcr.io/catzoidhub/dialcatz-den` as published.

Out of scope: a server that is already compromised, a Den served over plain HTTP on a public network, and
vulnerabilities in third-party software (Docker, PostgreSQL, your proxy — report those upstream; we are happy to help).
Problems in the DialCatz app go to [DialCatz](https://github.com/catzoidhub/DialCatz/security/advisories/new).

## Supported versions

Only the **latest release** gets security fixes. Redeploy the newest `v*` tag (or pull the newest image) to stay on
it.
