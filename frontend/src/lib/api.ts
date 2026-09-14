export type User = {
  id: number
  tenant_id: number | null
  email: string
  full_name: string
  role: 'admin' | 'owner' | 'staff'
  is_active: boolean
}

export type Session = {
  access: string
  refresh: string
  expiresAt: number
  user: User
}

const BASE = '/api/v1'
const KEY = 'fihos.session'
const TENANT_KEY = 'fihos.activeTenant'

const EXPIRY_BUFFER_MS = 30_000

let cached: Session | null = null

export function getActiveTenant(): number | null {
  try {
    const raw = localStorage.getItem(TENANT_KEY)
    if (raw) {
      const id = Number(raw)
      return Number.isFinite(id) && id > 0 ? id : null
    }
  } catch {
    /* ignore */
  }
  return null
}

export function setActiveTenant(id: number | null) {
  try {
    if (id) localStorage.setItem(TENANT_KEY, String(id))
    else localStorage.removeItem(TENANT_KEY)
  } catch {
    /* ignore */
  }
}

function loadSession(): Session | null {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) {
      cached = null
      return cached
    }
    cached = JSON.parse(raw) as Session
    return cached
  } catch {
    cached = null
    return null
  }
}

export function getUser(): User | null {
  return loadSession()?.user ?? null
}

function saveSession(t: {
  access_token: string
  refresh_token: string
  expires_at: string
  user: User
}) {
  cached = {
    access: t.access_token,
    refresh: t.refresh_token,
    expiresAt: Date.parse(t.expires_at),
    user: t.user,
  }
  localStorage.setItem(KEY, JSON.stringify(cached))
}

export function clearSession() {
  cached = null
  localStorage.removeItem(KEY)
  setActiveTenant(null)
}

export function logout() {
  clearSession()
  window.location.hash = '/login'
}

let refreshing: Promise<boolean> | null = null

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message)
  }
}

async function refreshNow(): Promise<boolean> {
  const s = loadSession()
  if (!s?.refresh) return false
  try {
    const res = await fetch(`${BASE}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: s.refresh }),
    })
    if (!res.ok) {
      clearSession()
      return false
    }
    saveSession((await res.json()) as Session & { access_token: string; refresh_token: string; expires_at: string })
    return true
  } catch {
    clearSession()
    return false
  }
}

export async function api<T = unknown>(method: string, path: string, body?: unknown): Promise<T> {
  let s = loadSession()
  const authless = path === '/auth/login' || path === '/auth/refresh'
  if (!authless && s?.refresh && s.expiresAt - EXPIRY_BUFFER_MS <= Date.now()) {
    refreshing = refreshing ?? refreshNow().finally(() => (refreshing = null))
    if (await refreshing) s = loadSession()
  }
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (s?.access) headers['Authorization'] = `Bearer ${s.access}`
  if (s?.user?.tenant_id == null) {
    const tid = getActiveTenant()
    if (tid) headers['X-Tenant-Id'] = String(tid)
  }

  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (res.status === 401 && s?.refresh) {
    refreshing = refreshing ?? refreshNow().finally(() => (refreshing = null))
    if (await refreshing) return api<T>(method, path, body)
    throw new ApiError(401, 'unauthorized', 'Session expired')
  }
  if (!res.ok) {
    let code = 'error'
    let msg = res.statusText
    try {
      const e = (await res.json()) as { error?: { code?: string; message?: string } }
      code = e.error?.code ?? code
      msg = e.error?.message ?? msg
    } catch {
      /* empty body */
    }
    throw new ApiError(res.status, code, msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export async function login(email: string, password: string): Promise<void> {
  const t = await api<{ access_token: string; refresh_token: string; expires_at: string; user: User }>(
    'POST',
    '/auth/login',
    { email, password },
  )
  saveSession(t)
}