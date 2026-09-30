# LumberCalc

LumberCalc is a lumber design app that will calculate the cheapest selection of boards to buy for a given project. The `backend/` API is in Go and exists currently. The `frontend/` app is a Vite and React SPA.

User instructions in chat override this file. The nearest `AGENTS.md` wins when one is added under a subdirectory.

## Architecture

LumberCalc uses a layered architecture. The packages in Layout are the layers. Organize code by responsibility so that each package has a clear purpose. Send dependencies through the correct packages.

## Frontend conventions

Reference `docs/architecture/frontend.md` for detailed conventions.

## Backend conventions

Reference `docs/architecture/backend.md` for detailed conventions.

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
- `frontend/` is the Vite app. It uses pnpm. From `frontend/`, `pnpm dev` serves `http://localhost:5173` and proxies `/v1` to the API on `:8080`.
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

Copy `backend/.env.example` to `backend/.env` for a local API. Required values are `DATABASE_URL`, `CLERK_SECRET_KEY`, and `CLERK_AUTHORIZED_PARTIES`. `HTTP_ADDR` defaults to `:8080`. `LOG_LEVEL` defaults to `info`.

From the repo root:

```bash
cd frontend
pnpm install
pnpm test
pnpm dev
```

`frontend/.env.example` holds `VITE_CLERK_PUBLISHABLE_KEY`. Replace the placeholder with a Clerk publishable key (`pk_test_` or `pk_live_`) from the same Clerk instance as `CLERK_SECRET_KEY`. The placeholder is not a key Clerk accepts.

## Conventions

- Keep code DRY (Don't Repeat Yourself) where possible, especially when multiple areas serve the same function. Do not duplicate logic that you would then change in many places.
- Organize a file around one cohesive responsibility. The file holds related functions, types, components, or routes.
- Split a file when it starts to hold unrelated responsibilities. Put each responsibility in a file or directory with a clear name.
- Name directories and files for the area, feature, or responsibility they contain.
- Add a documenting comment when a function or section is non-trivial and the names do not show its purpose.
- Format Go with `gofmt`. The module path is `lumbercalc/backend`.
- Keep handlers thin. Parse and validate at the HTTP, config, and Clerk boundaries. Trust typed IDs inside the process.
- A comment states an invariant or an important behavior that the signature does not show. Do not restate the next line.
- Commit subjects already in the repo look like `feat(api): require a Clerk session for /v1`. Match that style when asked to commit.
- For the Go API, add or update a test for behavior you change. Call the code the way its caller does and assert the result.
- For the frontend, add a test only when the logic is complex. Do not add a test for a page, a redirect, or a thin request wrapper.

## Boundaries

- `GET /readyz` lists check names and ok flags. It does not include probe error text.
- Schema rules already in SQL: inches are `numeric`, money is integer cents, a design body is `jsonb`, one pattern per design, soft-delete is `designs.deleted_at` only.
- Do not add Docker, Compose, Redis, Terraform, or AWS unless the task asks for them. The current runtime is a local Postgres database and the Go process.
- Do not bypass a package for convenience. `api` may call `store`. Only `store` imports `store/internal/q`. `api`, `server`, `clerkauth`, and `config` must not execute SQL.
- Do not add a new package unless the responsibility cannot belong to an existing package.

## Git

Do not `git add`, `git commit`, `git push`, or open a pull request unless the user asks. Do not commit `.env` files. `backend/.env.example` may contain only placeholders such as `sk_test_replace_me`.
