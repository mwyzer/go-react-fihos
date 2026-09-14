import { Fragment, useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'
import { fmtRp, fmtTime } from '../lib/fmt'

type Customer = {
  id: number
  hotspot_id: number | null
  name: string
  phone: string
  address: string
  status: 'active' | 'token' | 'expired' | 'suspended'
  balance: number
  created_at: string
}

type Page<T> = { items: T[]; total: number; page: number; size: number }

type WalletTx = {
  id: number
  type: 'topup' | 'bill_payment' | 'adjustment'
  amount: number
  balance_after: number
  ref_type: string
  ref_id: number
  note: string
  created_at: string
}

type Wallet = {
  customer_id: number
  balance: number
  transactions: Page<WalletTx>
}

const TX_LABEL: Record<WalletTx['type'], string> = {
  topup: 'Top up',
  bill_payment: 'Bayar tagihan',
  adjustment: 'Penyesuaian',
}

const STATUSES = ['active', 'token', 'expired', 'suspended'] as const
const STATUS_LABEL: Record<string, string> = {
  active: 'Aktif',
  token: 'Token',
  expired: 'Expired',
  suspended: 'Suspended',
}

export function Customers() {
  const me = getUser()
  const isOwner = me?.role === 'admin' || me?.role === 'owner'
  const [items, setItems] = useState<Customer[]>([])
  const [filter, setFilter] = useState('')
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [wallet, setWallet] = useState<Wallet | null>(null)
  const [walletId, setWalletId] = useState<number | null>(null)

  async function openWallet(c: Customer) {
    if (walletId === c.id) {
      setWalletId(null)
      return
    }
    setWalletId(c.id)
    try {
      const w = await api<Wallet>('GET', `/customers/${c.id}/wallet`)
      setWallet(w)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load wallet')
    }
  }

  async function refreshWallet(c: Customer) {
    try {
      const w = await api<Wallet>('GET', `/customers/${c.id}/wallet`)
      setWallet(w)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load wallet')
    }
  }

  async function load() {
    try {
      const params = new URLSearchParams({ size: '100' })
      if (filter) params.set('status', filter)
      const [page, ov] = await Promise.all([
        api<Page<Customer>>('GET', `/customers?${params}`),
        api<Record<string, number>>('GET', '/customers/overview'),
      ])
      setItems(page.items ?? [])
      setCounts(ov)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    }
  }

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void load()
  }, [filter])

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    const fd = new FormData(e.currentTarget)
    try {
      await api('POST', '/customers', {
        name: fd.get('name'),
        phone: fd.get('phone'),
        address: fd.get('address'),
        status: fd.get('status'),
      })
      ;(e.currentTarget as HTMLFormElement).reset()
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed')
    } finally {
      setBusy(false)
    }
  }

  async function setStatus(c: Customer, status: string) {
    try {
      await api('PATCH', `/customers/${c.id}/status`, { status })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Update failed')
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <h2>Pelanggan</h2>
        <div className="head-actions">
          {Object.entries(STATUS_LABEL).map(([k, label]) => (
            <button key={k} className={`btn ghost small ${filter === k ? 'active' : ''}`} onClick={() => setFilter(filter === k ? '' : k)}>
              {label}: {counts[k] ?? 0}
            </button>
          ))}
          {filter && (
            <button className="btn ghost small" onClick={() => setFilter('')}>
              x
            </button>
          )}
        </div>
      </header>

      {error && <div className="error-banner">{error}</div>}

      {isOwner && (
        <form className="card grid-form" onSubmit={create}>
          <label>
            Nama
            <input name="name" required placeholder="Nama pelanggan" />
          </label>
          <label>
            No. HP
            <input name="phone" placeholder="08xxxxxxxxxx" />
          </label>
          <label>
            Alamat
            <input name="address" placeholder="Alamat" />
          </label>
          <label>
            Status
            <select name="status" defaultValue="active">
              {STATUSES.map((s) => (
                <option key={s} value={s}>
                  {STATUS_LABEL[s]}
                </option>
              ))}
            </select>
          </label>
          <div className="form-actions">
            <button className="btn primary" type="submit" disabled={busy}>
              {busy ? 'Menyimpan…' : 'Tambah pelanggan'}
            </button>
          </div>
        </form>
      )}

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Nama</th>
              <th>No. HP</th>
              <th>Alamat</th>
              <th>Saldo</th>
              <th>Status</th>
              {isOwner && <th className="right">Aksi</th>}
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <Fragment key={c.id}>
                <tr className={walletId === c.id ? 'row-active' : ''}>
                  <td>{c.name}</td>
                  <td className="muted">{c.phone}</td>
                  <td className="muted">{c.address}</td>
                  <td>
                    <span className={`wallet-balance ${c.balance > 0 ? 'pos' : ''}`}>{fmtRp(c.balance)}</span>
                  </td>
                  <td>
                    <span className={`badge ${c.status === 'active' ? 'ok' : c.status === 'token' ? 'info' : c.status === 'suspended' ? 'warn' : 'dim'}`}>
                      {STATUS_LABEL[c.status]}
                    </span>
                  </td>
                  {isOwner && (
                    <td className="right">
                      <button className="btn mini" onClick={() => void openWallet(c)}>
                        {walletId === c.id ? 'Tutup' : 'Wallet'}
                      </button>
                      <select value={c.status} onChange={(e) => void setStatus(c, e.target.value)}>
                        {STATUSES.map((s) => (
                          <option key={s} value={s}>
                            {STATUS_LABEL[s]}
                          </option>
                        ))}
                      </select>
                    </td>
                  )}
                </tr>
                {isOwner && walletId === c.id && wallet?.customer_id === c.id && (
                  <tr>
                    <td colSpan={6}>
                      <WalletPanel
                        customer={c}
                        wallet={wallet}
                        onChanged={async () => {
                          await refreshWallet(c)
                          await load()
                        }}
                      />
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
            {items.length === 0 && (
              <tr>
                <td colSpan={isOwner ? 6 : 5} className="muted">
                  Belum ada pelanggan.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function WalletPanel({
  customer,
  wallet,
  onChanged,
}: {
  customer: Customer
  wallet: Wallet
  onChanged: () => Promise<void>
}) {
  const [amount, setAmount] = useState('')
  const [adj, setAdj] = useState('')
  const [note, setNote] = useState('')
  const [msg, setMsg] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)

  async function topup(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    setMsg(null)
    try {
      const res = await api<{ payment_url?: string; async: boolean; payment: { status: string; external_ref: string } }>(
        'POST',
        `/customers/${customer.id}/topup`,
        { amount: Number(amount) },
      )
      if (res.async) {
        setMsg({ kind: 'ok', text: `Link pembayaran: ${res.payment_url ?? ''} (ref ${res.payment.external_ref})` })
      } else {
        setMsg({ kind: 'ok', text: `Top up diterima — saldo diperbarui (ref ${res.payment.external_ref})` })
      }
      setAmount('')
      await onChanged()
    } catch (err) {
      setMsg({ kind: 'err', text: err instanceof Error ? err.message : 'Top up gagal' })
    } finally {
      setBusy(false)
    }
  }

  async function adjust(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    setMsg(null)
    try {
      await api('POST', `/customers/${customer.id}/wallet/adjust`, {
        amount: Number(adj),
        note: note || 'Penyesuaian manual',
      })
      setAdj('')
      setNote('')
      setMsg({ kind: 'ok', text: 'Penyesuaian berhasil' })
      await onChanged()
    } catch (err) {
      setMsg({ kind: 'err', text: err instanceof Error ? err.message : 'Penyesuaian gagal' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="wallet-panel">
      <div className="row-space">
        <h3>
          Wallet {customer.name} — <span className="wallet-balance pos">{fmtRp(wallet.balance)}</span>
        </h3>
      </div>
      {msg && <div className={`error-banner ${msg.kind === 'ok' ? 'ok-banner' : ''}`}>{msg.text}</div>}
      <div className="wallet-actions">
        <form className="inline-form" onSubmit={topup}>
          <input
            type="number"
            min="1"
            step="1000"
            placeholder="Jumlah top up"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            required
          />
          <button className="btn primary small" type="submit" disabled={busy}>
            Top up
          </button>
        </form>
        <form className="inline-form" onSubmit={adjust}>
          <input
            type="number"
            placeholder="+/- penyesuaian"
            value={adj}
            onChange={(e) => setAdj(e.target.value)}
            required
          />
          <input placeholder="Catatan" value={note} onChange={(e) => setNote(e.target.value)} />
          <button className="btn ghost small" type="submit" disabled={busy}>
            Sesuaikan
          </button>
        </form>
      </div>
      {wallet.transactions.items.length === 0 ? (
        <p className="muted small">Belum ada transaksi wallet.</p>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Waktu</th>
                <th>Tipe</th>
                <th>Jumlah</th>
                <th>Saldo</th>
                <th>Keterangan</th>
              </tr>
            </thead>
            <tbody>
              {wallet.transactions.items.map((t) => (
                <tr key={t.id}>
                  <td className="muted">{fmtTime(t.created_at)}</td>
                  <td>{TX_LABEL[t.type] ?? t.type}</td>
                  <td className={t.amount >= 0 ? 'pos-amt' : 'neg-amt'}>
                    {t.amount >= 0 ? '+' : ''}
                    {fmtRp(t.amount)}
                  </td>
                  <td className="muted">{fmtRp(t.balance_after)}</td>
                  <td className="muted">{t.note || (t.ref_type ? `${t.ref_type}#${t.ref_id}` : '—')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

export default Customers