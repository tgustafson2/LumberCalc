# LumberCalc

LumberCalc is a lumber design app that will calculate the cheapest selection of boards to buy for a given project. The `backend/` API is in Go and exists currently. A Vite/React frontend is planned for the future.

User instructions in chat override this file. The nearest `AGENTS.md` wins when one is added under a subdirectory.

## Layout

- `backend/cmd/api` serves HTTP.
- `backend/cmd/migrate` applies embedded SQL migrations.
- `backend/internal/config` loads `.env` and the process environment. Process values win. A missing `.env` is not an error.
- `backend/internal/server` is the process HTTP stack. `GET /healthz` and `GET /readyz` are public.
- `backend/internal/api` is `/v1`. Every path under `/v1` requires a Clerk session, including unknown paths.
- `backend/internal/clerkauth` verifies Clerk session JWTs and loads display names.
- `backend/internal/store` is the database boundary. `UserID` and `ClerkUserID` are constructed only there.
- `backend/db/migrations` and `backend/db/queries` are the schema and sqlc inputs.
- `backend/internal/store/internal/q` is generated. Do not edit it.
- `sprints/` is local planning and is gitignored. Read it for intent. Do not commit it.

## Commands

Run these from `backend/`.

```bash
go test ./...
go run ./cmd/api
go run ./cmd/migrate
sqlc generate
gofmt -w .
```

`go test ./...` skips Postgres integration tests when `TEST_DATABASE_URL` is unset. A package that finishes in a few milliseconds did not hit the database. Set `TEST_DATABASE_URL` to a disposable local database before treating a store or migration change as verified.

Copy `backend/.env.example` to `backend/.env` for a local API. Required values are `DATABASE_URL` and `CLERK_SECRET_KEY`. `HTTP_ADDR` defaults to `:8080`. `LOG_LEVEL` defaults to `info`. `CLERK_AUTHORIZED_PARTIES` defaults to `http://localhost:5173`.

## Conventions

- Keep to DRY(Don't Repeat Yourself) where possible especially when areas are serving the same function that way it isn't necessary to make changes in many different places
- Add documenting comments when the function or section of code is non trivial and not explained
through function or variable names
- Format Go with `gofmt`. Module path is `lumbercalc/backend`.
- Keep handlers thin. Parse and validate at the HTTP, config, and Clerk boundaries. Trust typed ids inside the process.
- Comments state an invariant the signature does not show. Do not restate what the next line does.
- Commit subjects already in the repo look like `feat(api): require a Clerk session for /v1`. Match that when asked to commit.
- Add or update a test for behavior you change. Call the code the way its caller does and assert the result.

## Boundaries

- `GET /readyz` lists check names and ok flags. It does not include probe error text.
- Schema rules already in SQL: inches are `numeric`, money is integer cents, a design body is `jsonb`, one pattern per design, soft-delete is `designs.deleted_at` only.
- Do not add Docker, Compose, Redis, Terraform, or AWS unless the task asks for them. Current runtime is a local Postgres and the Go process.

## Git

Do not `git add`, `git commit`, `git push`, or open a pull request unless the user asks. Do not commit `.env` files. `backend/.env.example` may contain only placeholders such as `sk_test_replace_me`.
