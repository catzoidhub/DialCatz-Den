<p align="center"><img src="admin/src/assets/icon.png" width="128" height="128" alt="DialCatz Den"></p>

<h1 align="center">DialCatz Den</h1>
<p align="center"><em>Where the team's cats keep their calls.</em></p>
<p align="center">
  <a href="https://catzoid.com/dialcatz">Website</a> ·
  <a href="https://catzoid.com/docs/dialcatz-den">Docs</a> ·
  <a href="https://catzoid.com/dialcatz/download">Get DialCatz</a> ·
  <a href="https://catzoid.com/community">Community</a>
</p>

The self-hosted team server for [DialCatz](https://github.com/catzoidhub/DialCatz), the open-source API client. Each team runs its own: it keeps the team's shared
collections and environments, and knows who is who. One small container — Go binary with the admin panel
embedded, SQLite inside (or PostgreSQL), ~30 MB image, distroless, non-root.

Part of [Catzoid](https://catzoid.com) — free, open-source developer tools with nine lives. All cats, no catch: no
account, no Pro tier, nothing locked behind a payment.

**No accounts for members.** The admin creates members in the admin panel and sends each of their devices a
one-time invite link. Opening it in DialCatz exchanges the code for that device's own token (kept in the system's
secret store). Every change is attributed to the member the token belongs to; revoke one device and only that
device is locked out.

**Secrets stay local.** Environments sync variable *names* only; every member fills in values on their own
computer.

**Live.** The app keeps an event stream open per workspace: a teammate's push shows up within a second, and
everyone sees who is looking at which request (presence). A token can be limited to some of its member's
workspaces (choose it when making the invite link or the token).

## Deploy on Dokploy from Git (recommended)

The repository builds itself — no registry, nothing prebuilt. Fork it or use it as it is:

1. Dokploy → your project → **Create Service → Compose**.
2. Provider **Git** (or GitHub): `https://github.com/catzoidhub/DialCatz-Den.git`, branch `main`,
   compose path `./docker-compose.yml` (SQLite) — or `./docker-compose.postgres.yml` (its own PostgreSQL).
3. **Environment**: `DIALCATZ_PUBLIC_URL=https://dialcatz.your-domain.com` (and `POSTGRES_PASSWORD` for the
   PostgreSQL file).
4. **Domains**: host `dialcatz.your-domain.com`, service `dialcatz`, container port **8080**, HTTPS on
   (Let's Encrypt). Behind Cloudflare use SSL *Full (strict)*.
5. **Deploy**, open the domain: the first visit creates the admin account.

**Updating**: press **Deploy** again (or turn on Auto Deploy / the webhook so every push to the branch redeploys).
The image is rebuilt from the new code, database migrations run at start, and your data stays in the
`dialcatz-data` volume. Pin a release by deploying a tag (`v0.2.0`) instead of `main`.

Dokploy's other way, **Application → Git → Build type Dockerfile**, works too: same repository, Dockerfile at the
root, port 8080, and add a volume mounted at `/data`.

**Backups**: SQLite lives in the `dialcatz-data` volume (`/data/dialcatz.db`) — back up that volume. With
PostgreSQL, back up the database (or use a Dokploy PostgreSQL database with its S3 backups and point
`DATABASE_URL` at it from `docker-compose.yml`).

## Other ways to run it

```sh
# Any machine with Docker, from a clone:
cp .env.example .env    # set DIALCATZ_PUBLIC_URL
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build

# The ready image (published by .github/workflows/image.yml for each v* tag):
docker run -d -p 8080:8080 -v dialcatz:/data -e DIALCATZ_PUBLIC_URL=https://dialcatz.example.com ghcr.io/catzoidhub/dialcatz-den
```

Put it behind HTTPS: the DialCatz app refuses plain `http://` except for local addresses.

## Configuration

| Variable | Default | |
| --- | --- | --- |
| `DIALCATZ_PUBLIC_URL` | `http://localhost:8080` | The address members use; goes into invite links. `https://` turns on secure cookies. |
| `DATABASE_URL` | SQLite at `/data/dialcatz.db` | Or `postgres://user:pass@host:5432/db` for PostgreSQL. |
| `DIALCATZ_SECRET_KEY` | generated into `/data/secret.key` | Encrypts the admins' two-step secrets. Keep it: changing it means setting up two-step sign-in again. |
| `DIALCATZ_DATA_DIR` | `/data` | SQLite file and the generated key. Keep it on a volume. |
| `DIALCATZ_ADDR` | `:8080` | Listen address inside the container. |
| `DIALCATZ_GOOGLE_CLIENT_ID` / `_SECRET` | | Optional "Sign in with Google" for admins (see below). |
| `DIALCATZ_GITHUB_CLIENT_ID` / `_SECRET` | | Optional "Sign in with GitHub" for admins. |

## Admin sign-in

**More than one admin**: the first visit creates the first admin; after that, *Admins → Invite an admin* (name +
email) makes a one-time link valid for 48 hours. The invited person opens it and chooses their own password — no
shared sign-in, no password sent by chat. Every admin can manage everything, other admins included; an admin cannot
remove themselves, and the last admin can never be removed. Removing an admin signs them out everywhere and deletes
their passkeys and two-step codes.

**Two-step sign-in**: *Your account → Two-step sign-in* shows a QR code for any authenticator app (TOTP, 6 digits,
30 s). After the first code it shows ten one-time recovery codes — save them; each works once in place of a code.
Turning it off needs the password and a code.

**Passkeys**: *Your account → Passkeys → Add a passkey* registers Touch ID / Face ID, Windows Hello, a phone or a
security key; the sign-in page then shows **Sign in with a passkey** (no email to type). A passkey sign-in is
complete on its own — it cannot be phished and the device checks the person — so no two-step code follows it.
Add one passkey per device and keep the password (and recovery codes) as the way back in. Passkeys need the server
on an `https://` address with a domain name and are bound to that domain (the host of `DIALCATZ_PUBLIC_URL`): after
moving the server to another domain, register them again.

**Google / GitHub** (optional, with your own OAuth app): set the client id and secret of either and the sign-in
page shows the button. It only signs in an admin who already exists with the same, verified email (GitHub: the
primary verified address); it never creates admins, and two-step sign-in still asks for the code. Register this
redirect URL with the OAuth app:

- Google (Cloud Console → Credentials → OAuth client, type *Web application*):
  `https://dialcatz.your-domain.com/api/admin/oauth/google/callback`
- GitHub (Settings → Developer settings → OAuth Apps): `https://dialcatz.your-domain.com/api/admin/oauth/github/callback`

## Moving to another server (Backup)

*Admin → Backup* exports one encrypted file (`.dcden`) with the members, the workspaces with their collections and
environments, the memberships, the devices and the activity log; the admin chooses a passphrase (Argon2id +
AES-256-GCM — without it the file cannot be opened). Admins are not in it: each server keeps its own.

1. On the old server: *Backup → Export a backup*.
2. Deploy DialCatz Den on the new server (SQLite or PostgreSQL — either way round works), create its admin at the first
   visit, then *Backup → Restore a backup*: choose the file, enter the passphrase, check what is in it, and type the
   server's name to replace its members, workspaces, devices and activity.
3. Point the address at the new server. With the same address every device keeps working (the token hashes moved
   with the backup); with a new address, members change the server address in DialCatz → Team.

Open invite links do not move (they carry the old address): make new ones. A backup from a newer DialCatz Den is
refused by an older one — update the new server first.

The panel's lists (members, workspaces, devices, admins, activity) have a filter, a search and pages; activity is
filtered and searched on the server, so it stays quick however long the log grows.

## Live updates behind a proxy

The event stream (`/api/v1/workspaces/{id}/events`) is a long-lived `text/event-stream` response; the server sends
`X-Accel-Buffering: no` and a ping every 25 s, which is enough for Traefik (Dokploy), Caddy and nginx. Live state
is held in memory, so run one instance (SQLite, or a single instance on PostgreSQL).

## Releasing

Bump `VERSION`, commit, tag `v<version>` and push the tag: teams deploying from Git redeploy the tag; the workflow
also publishes the multi-arch image to GHCR.

## API

[`openapi.yaml`](openapi.yaml) describes the sync API the app uses (`/api/health`, `/api/v1/connect`, `/api/v1/me`,
`/api/v1/workspaces/{id}/changes|push|events|presence`). The admin API under `/api/admin` is the panel's own.

## Development

```sh
cd admin && bun install && bun run build   # the panel → internal/web/dist (embedded)
cd .. && DIALCATZ_DATA_DIR=./tmp DIALCATZ_PUBLIC_URL=http://localhost:8080 go run ./cmd/dialcatz-den
```

`bun run dev` in `admin/` serves the panel with hot reload and proxies `/api` to `:8080`.

Layout: `cmd/dialcatz-den` (main), `internal/config` (env), `internal/db` (SQLite / PostgreSQL + goose
migrations), `internal/store` (all queries, portable SQL), `internal/auth` (argon2id passwords, SHA-256 hashed
tokens, TOTP, AES-GCM sealing), `internal/live` (in-memory event / presence hub), `internal/api` (Gin routes), `internal/web` (embedded panel), `admin/` (Vue + Tailwind panel).

## Licence

DialCatz Den is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General
Public License as published by the Free Software Foundation, version 3. It comes with no warranty. See
[`LICENSE`](LICENSE). If you run a modified Den for other people over a network, the AGPL asks you to offer them its
source — the admin panel links to it at the foot of the sidebar.

The names Catzoid, DialCatz and DialCatz Den and the Catzoid cat are trademarks and are **not** covered by the
licence: a fork must use its own name and icon. See [`TRADEMARKS.md`](TRADEMARKS.md).

Contributions are welcome — see [`CONTRIBUTING.md`](CONTRIBUTING.md). Before your first pull request is merged you
sign the [contributor licence agreement](CLA.md) through the CLA check on the pull request — you keep the copyright of
your work. Questions and bug reports: [`SUPPORT.md`](SUPPORT.md); security problems, privately: [`SECURITY.md`](SECURITY.md).

Copyright © 2026 Mahryan Rakhmatullah
