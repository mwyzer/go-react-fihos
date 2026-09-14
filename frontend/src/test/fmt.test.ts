import { describe, it, expect } from 'vitest'
import { fmtBytes, fmtDur, toISO } from '../lib/fmt'

describe('fmtBytes', () => {
  it('formats zero and negative as 0 B', () => {
    expect(fmtBytes(0)).toBe('0 B')
    expect(fmtBytes(null)).toBe('0 B')
    expect(fmtBytes(undefined)).toBe('0 B')
    expect(fmtBytes(-5)).toBe('0 B')
  })
  it('scales through units', () => {
    expect(fmtBytes(512)).toBe('512 B')
    expect(fmtBytes(2048)).toBe('2.0 KB')
    expect(fmtBytes(1_048_576)).toBe('1.0 MB')
    expect(fmtBytes(1073741824 * 2)).toBe('2.0 GB')
  })
  it('drops decimals for large numbers', () => {
    expect(fmtBytes(104857600)).toBe('100 MB')
  })
})

describe('fmtDur', () => {
  it('handles minutes and hours', () => {
    expect(fmtDur(45)).toBe('45 min')
    expect(fmtDur(120)).toBe('2h')
    expect(fmtDur(150)).toBe('2h 30m')
    expect(fmtDur(null)).toBe('—')
  })
})

describe('toISO', () => {
  it('converts local input to ISO or empty', () => {
    expect(toISO('')).toBe('')
    expect(toISO('2026-09-14T10:00')).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/)
  })
})