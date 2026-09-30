# Frontend architecture

`frontend/` is the Vite and React SPA. It uses pnpm and TypeScript. The files below are the current layers. Put new code in one of these files until that file holds an unrelated responsibility. Then split the file into a directory named for the feature. Do not add a new directory when the related code can stay in an existing file.

## Files

- `frontend/src/client-config.ts` reads `VITE_CLERK_PUBLISHABLE_KEY`. The value must be a Clerk publishable key (`pk_test_` or `pk_live_`). A secret key is rejected. A key Clerk would reject is rejected here, and the page shows the reason.
- `frontend/src/main.tsx` mounts Clerk when that read succeeds.
- `frontend/src/gate.ts` maps a known session kind to `/` or `/designs`.
- `frontend/src/app.tsx` classifies the Clerk session and owns the routes.
- `frontend/src/api.ts` is the `/v1` client. It parses JSON at the network boundary.
- `frontend/src/api.gen.ts` is generated from `backend/openapi.yaml`. Do not edit it.
- `frontend/vite.config.ts` serves `http://localhost:5173` and proxies `/v1` to the API on `:8080`.

`openapi-typescript` writes `frontend/src/api.gen.ts` from `backend/openapi.yaml`. `frontend/src/api.ts` maps the snake_case response to the camelCase `Me` type. `toMe` is the only mapper.

## Rules for new code

Name a file or a directory for the area, feature, or responsibility it contains.

Keep related functions, types, and components in one file. Split the file when it holds an unrelated responsibility.

Keep a helper next to the only feature that uses it. Put shared code in a directory named for that shared responsibility. Do not copy the same code across features.

Add a test only when the logic is complex. Do not add a test for a page, a redirect, or a thin request wrapper.

Add a comment when a function or a section is non-trivial and the names do not show its purpose. A comment states an invariant or an important behavior that the signature does not show. Do not restate the next line.
