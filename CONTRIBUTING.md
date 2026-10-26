# Contributing to DialCatz Den

Thank you for wanting to help — DialCatz Den is made by one person, and every fix, idea and bug report counts. This page says
how to get a change merged without wasted work.

## Before you start

- **Small fixes** (a typo, an obvious bug, a clearer message): open a pull request straight away.
- **Anything bigger** (a new feature, a new dependency, a change in how something works): open an issue first and
  wait for a yes, so nobody builds something that will not be merged.
- **Security problems** never go in a public issue or pull request — see [`SECURITY.md`](SECURITY.md).
- Questions and where to ask them: [`SUPPORT.md`](SUPPORT.md).

## Running it from source

You need Go (the version in `go.mod`), [Bun](https://bun.sh) for the admin panel, and optionally Docker.

```sh
cd admin && bun install && bun run build   # the panel → internal/web/dist (embedded)
cd .. && DIALCATZ_DATA_DIR=./tmp DIALCATZ_PUBLIC_URL=http://localhost:8080 go run ./cmd/dialcatz-den
```

`bun run dev` in `admin/` serves the panel with hot reload. Before a pull request: `go vet ./...`, `gofmt -l .`
(prints nothing), `go test ./...` and `bun run typecheck` in `admin/`. Queries stay portable — every change must work
on both SQLite and PostgreSQL — and a schema change is a new goose migration, never an edit to an old one. If you
change the sync API, update [`openapi.yaml`](openapi.yaml) in the same pull request.

## Making the change

- **Branches.** Work on a branch in your fork (`fix/…` or `feature/…`) and open the pull request against `main`.
  `main` is what the next release is built from; releases are tags (`v1.2.0`).
- **Match the code around you** — its naming, comment style and structure. No reformatting of lines you did not
  change, and no new dependency without asking first.
- **One change per pull request**, with a title that says what it does ("Fix the port check when 80 is taken"). Say
  why in the description and link the issue (`Fixes #123`).
- **Words in the app** are English and short; the cats speak plainly and kindly.
- **Never commit secrets** — keys, tokens, passwords, real customer or company names in test data.

## Before you open the pull request

- `bun run typecheck` passes (and the Go checks above).
- Try it against SQLite and PostgreSQL if you touched the database.
- Screenshots for anything you can see — before and after.

## The contributor licence agreement

The first time you open a pull request, the CLA check asks you to sign the
[contributor licence agreement](CLA.md) by leaving a comment. You keep the copyright of your work; the agreement lets
the project ship it under its licence. You sign once for all your pull requests.

## Names and logos

DialCatz Den's code is free software ([`LICENSE`](LICENSE)); its name, the Catzoid name and the cat logos are not
([`TRADEMARKS.md`](TRADEMARKS.md)). Please do not change them in a pull request — and a fork that is shared with
others needs its own name and icon.
