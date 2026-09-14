import { describe, it, expect, beforeEach, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { api, clearSession, login, setActiveTenant, ApiError } from '../lib/api'
import type { User } from '../lib/api'
import Login from '../pages/Login'

const JSON_CONTENT = { 'Content-Type': 'application/json' }
const ADMIN: User = {
  id: 1,
  tenant_id: null,
  email: 'admin@fihos.dev',
  full_name: 'Admin',
  role: 'admin',
  is_active: true,
}
const OWNER: User = {
  id: 2,
  tenant_id: 3,
  email: 'owner@demo.dev',
  full_name: 'Owner',
  role: 'owner',
  is_active: true,
}

function seedSession(user: User) {
  localStorage.setItem(
    'fihos.session',
    JSON.stringify({ access: 'tok', refresh: 'ref', expiresAt: Date.now() + 3600_000, user }),
  )
}

beforeEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('api() tenant headers', () => {
  it('admin (tenant_id null) sends X-Tenant-Id from active tenant', async () => {
    seedSession(ADMIN)
    setActiveTenant(7)
    const res = { ok: true, status: 200, json: async () => ({ ok: 1 }) } as Response
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(res)
    await api('GET', '/routers')
    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toMatch(/\/api\/v1\/routers$/)
    const headers = init?.headers as Record<string, string>
    expect(headers['X-Tenant-Id']).toBe('7')
    expect(headers['Authorization']).toBe('Bearer tok')
  })

  it('admin without active tenant sends no X-Tenant-Id', async () => {
    seedSession(ADMIN)
    const res = { ok: true, status: 200, json: async () => ({ ok: 1 }) } as Response
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(res)
    await api('GET', '/routers')
    const headers = fetchMock.mock.calls[0][1]?.headers as Record<string, string>
    expect(headers['X-Tenant-Id']).toBeUndefined()
  })

  it('tenant-scoped user never sends X-Tenant-Id', async () => {
    seedSession(OWNER)
    setActiveTenant(7)
    const res = { ok: true, status: 200, json: async () => ({ ok: 1 }) } as Response
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(res)
    await api('GET', '/routers')
    const headers = fetchMock.mock.calls[0][1]?.headers as Record<string, string>
    expect(headers['X-Tenant-Id']).toBeUndefined()
  })

  it('sends Content-Type only when a body is present', async () => {
    seedSession(OWNER)
    const res = { ok: true, status: 200, json: async () => ({ ok: 1 }) } as Response
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(res)
    await api('POST', '/routers', { name: 'x' })
    const headers = fetchMock.mock.calls[0][1]?.headers as Record<string, string>
    expect(headers['Content-Type']).toBe(JSON_CONTENT['Content-Type'])
  })

  it('unwraps error payload into ApiError', async () => {
    seedSession(OWNER)
    const res = {
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      json: async () => ({ error: { code: 'forbidden', message: 'no access' } }),
    } as Response
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(res)
    await expect(api('GET', '/routers')).rejects.toMatchObject({ status: 403, code: 'forbidden', message: 'no access' })
  })

  it('rethrows original ApiError after failed refresh', async () => {
    seedSession(OWNER)
    const fail = { ok: false, status: 401, statusText: 'Unauthorized', json: async () => ({}) } as Response
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(fail)
    await expect(api('GET', '/routers')).rejects.toBeInstanceOf(ApiError)
  })
})

describe('login + clearSession', () => {
  it('stores session and calls api with credentials', async () => {
    const body = { access_token: 'a', refresh_token: 'r', expires_at: new Date(Date.now() + 3600_000).toISOString(), user: OWNER }
    const res = { ok: true, status: 201, json: async () => body } as Response
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(res)
    await login('owner@demo.dev', 'pw')
    const caller = vi.mocked(globalThis.fetch).mock.calls[0]
    expect(String(caller[0])).toMatch(/\/auth\/login$/)
    expect(JSON.parse(String(caller[1]?.body))).toMatchObject({ email: 'owner@demo.dev' })
    expect(localStorage.getItem('fihos.session')).toContain('owner@demo.dev')
  })

  it('clearSession drops session and active tenant', () => {
    seedSession(OWNER)
    setActiveTenant(7)
    clearSession()
    expect(localStorage.getItem('fihos.session')).toBeNull()
    expect(localStorage.getItem('fihos.activeTenant')).toBeNull()
  })
})

describe('Login page', () => {
  it('renders the form and submits', async () => {
    const body = { access_token: 'a', refresh_token: 'r', expires_at: new Date(Date.now() + 3600_000).toISOString(), user: OWNER }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true, status: 201, json: async () => body } as Response)
    render(<Login onLogin={() => {}} />)
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: 'owner@demo.dev' } })
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'pw' } })
    fireEvent.click(screen.getByRole('button', { name: /sign in|login|masuk/i }))
    await vi.waitFor(() => expect(localStorage.getItem('fihos.session')).toContain('owner@demo.dev'))
  })
})