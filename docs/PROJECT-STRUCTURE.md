# Project Structure

```
go-react-fihos/
├── backend/                  # Go / Gin API + worker
│   ├── cmd/
│   │   ├── api/              # API server entrypoint
│   │   └── worker/           # Worker process entrypoint
│   ├── internal/
│   │   ├── config/           # env/config loading
│   │   ├── server/           # router setup, middleware, routes
│   │   ├── handlers/         # HTTP handlers
│   │   ├── services/         # business logic
│   │   ├── repositories/     # data access (PostgreSQL/pgx or GORM)
│   │   ├── models/           # domain models
│   │   ├── auth/             # token, session, RBAC
│   │   ├── middleware/       # auth, tenant, validation, rate-limit
│   │   ├── ws/               # WebSocket hub
│   │   └── mikrotik/         # RouterOS API client
│   ├── pkg/
│   │   └── pulse/            # shared utilities (errors, pagination, validation)
│   ├── migrations/           # SQL migration files (versioned)
│   └── go.mod
├── frontend/                 # React + TypeScript SPA
│   ├── src/
│   │   ├── app/              # app shell, routing, guards
│   │   ├── pages/            # screen components (login, dashboard, routers…)
│   │   ├── components/       # shared UI components
│   │   ├── features/         # feature modules (vouchers, sessions…)
│   │   ├── api/              # typed API client
│   │   ├── hooks/            # React Query + WebSocket hooks
│   │   ├── styles/           # global styles / theme
│   │   └── types/            # shared TS types
│   ├── portal/               # minimal captive-portal bundle
│   ├── index.html
│   ├── package.json
│   └── vite.config.ts
├── migrations/               # shared SQL migration sources (source of truth)
├── docker/
│   ├── docker-compose.yml    # dev + prod compose files
│   ├── backend.Dockerfile
│   ├── frontend.Dockerfile
│   └── nginx.conf            # reverse proxy / TLS
├── docs/                     # PRD, SRS, architecture, API, DB, plans
└── README.md
```

Notes:
- `backend/migrations` and root `migrations/`: keep a single canonical set (root `migrations/`) and have CI copy/bind them; avoid duplicates.
- The portal bundle lives under `frontend/portal` and is deployed separately for captive-portal use.