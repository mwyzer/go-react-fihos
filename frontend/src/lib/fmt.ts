export function fmtBytes(n: number | null | undefined): string {
  if (!n || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 ? 0 : 1)} ${units[i]}`
}

export function fmtDur(min: number | null | undefined): string {
  if (!min || min <= 0) return '—'
  if (min < 60) return `${min} min`
  const h = Math.floor(min / 60)
  const m = Math.round(min % 60)
  return m ? `${h}h ${m}m` : `${h}h`
}

export function fmtTime(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '—' : d.toLocaleString()
}

export function fmtDate(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '—' : d.toLocaleDateString()
}

export function relTime(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso).getTime()
  const gap = Date.now() - d
  if (gap < 60_000) return 'just now'
  if (gap < 3_600_000) return `${Math.round(gap / 60_000)}m ago`
  if (gap < 86_400_000) return `${Math.round(gap / 3_600_000)}h ago`
  return `${Math.round(gap / 86_400_000)}d ago`
}

export function toISO(v: string): string {
  return v ? new Date(v).toISOString() : ''
}