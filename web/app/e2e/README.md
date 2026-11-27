# Playwright acceptance tests

These tests drive the reference web workspace against a running demo stack.

## Prerequisites

```bash
make demo
pnpm --dir web/app exec playwright install chromium
```

The frontend is expected at `http://localhost:3001` and the API at
`http://localhost:8000`. Override with `E2E_BASE_URL` and `E2E_API_URL`.

## Run

```bash
pnpm --dir web/app test:e2e
```

The suite creates a fresh shared user for shell/empty-state tests and two
throwaway users for the auth flow. It covers:

- register / login / logout and session restore
- rail navigation and empty states
- draggable sidebar (pointer, keyboard, reload persistence)
- mobile single-pane layout
- light/dark theme persistence
- operator-console access boundary

## Operator-console test

Set `E2E_ADMIN_USERNAME` and `E2E_ADMIN_PASSWORD` for an existing platform
admin. Bootstrap one by registering a user and recreating the gateway with
`AGENT_ADMIN_USER_IDS=<user-id>`:

```bash
AGENT_ADMIN_USER_IDS=<id> docker compose -f docker-compose.yml up -d \
  --no-deps --force-recreate nonoka-im
E2E_ADMIN_USERNAME=<username> E2E_ADMIN_PASSWORD=<password> \
  pnpm --dir web/app test:e2e
```

Without an admin user the admin spec is skipped. Live model, streaming, tool
and approval flows are verified manually against the demo stack rather than in
this suite.
