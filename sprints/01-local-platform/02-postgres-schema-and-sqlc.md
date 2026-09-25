# Postgres schema, migrations, and sqlc

- Sprint: 01 — Local platform
- Status: todo
- Depends on: 01-go-http-server

## Goal

Apply the locked schema to a local Postgres database. Typed queries via sqlc + pgx. Create a `users` row on first authenticated request.

## Acceptance criteria

- [x] `golang-migrate` (or equivalent) runs from `backend/db/migrations`
- [x] `backend/db/migrations/001_init.sql` creates: `users`, `user_settings`, `materials`, `prices`, `designs`, `design_material_usages`, `patterns`, `optimization_runs` plus the indexes and checks from the locked schema
- [x] `sqlc` generates Go from `backend/db/queries/*.sql` using `pgx/v5`
- [ ] First valid Clerk JWT creates `users` (by `clerk_user_id`) and a default `user_settings` row
- [ ] Subsequent requests reuse the same `users.id`
- [x] `GET /readyz` fails if migrations have not been applied

`store.EnsureUser` creates the user and settings row and returns the same `users.id` on every later call, including 32 concurrent first calls. The two unchecked items need the Clerk JWT middleware from `03`, which calls `EnsureUser`.

## Setup

Postgres must already be running (see `00-install-local-services`). From `backend/`:

```bash
go run ./cmd/migrate   # DATABASE_URL from .env, or -database URL. Prints "up to date" when nothing is pending
sqlc generate          # backend/sqlc.yaml reads db/migrations and db/queries, writes internal/store/internal/q
```

`sqlc.yaml` lives in `backend/`. Do not install a second Postgres in Docker this sprint.

## Notes

Schema contract: inches as `numeric`, money as integer cents, design body as `jsonb`, one pattern per design (`patterns.source_design_id UNIQUE`), soft-delete on `designs.deleted_at` only.

Seed system materials in Sprint 03. This sprint only needs the tables and the user upsert.
