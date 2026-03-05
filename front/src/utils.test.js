import { describe, it, expect } from 'vitest'
import { formatBytes, formatMs, parsePipeline } from './utils.js'

// Simule l'interface Headers de fetch (seul .get() est utilisé par parsePipeline)
const makeHeaders = (map) => ({ get: (k) => map[k] ?? null })

// ── formatBytes ───────────────────────────────────────────────────────────────

describe('formatBytes', () => {
  it('affiche les octets bruts sous 1 Ko', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(1)).toBe('1 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1023)).toBe('1023 B')
  })

  it('affiche en Ko entre 1024 et 1 Mo', () => {
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(2048)).toBe('2.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
  })

  it('affiche en Mo à partir de 1 Mo', () => {
    expect(formatBytes(1024 * 1024)).toBe('1.00 MB')
    expect(formatBytes(2 * 1024 * 1024)).toBe('2.00 MB')
    expect(formatBytes(1.5 * 1024 * 1024)).toBe('1.50 MB')
  })
})

// ── formatMs ──────────────────────────────────────────────────────────────────

describe('formatMs', () => {
  it('retourne null si la valeur est absente', () => {
    expect(formatMs(null)).toBeNull()
    expect(formatMs(undefined)).toBeNull()
  })

  it('affiche en ms avec 1 décimale entre 1 et 999 ms', () => {
    expect(formatMs('1')).toBe('1.0 ms')
    expect(formatMs('12.5')).toBe('12.5 ms')
    expect(formatMs('999.9')).toBe('999.9 ms')
  })

  it('affiche en secondes à partir de 1000 ms', () => {
    expect(formatMs('1000')).toBe('1.00 s')
    expect(formatMs('2500')).toBe('2.50 s')
  })

  it('affiche avec 2 décimales sous 1 ms', () => {
    expect(formatMs('0.5')).toBe('0.50 ms')
    expect(formatMs('0.12')).toBe('0.12 ms')
    expect(formatMs('0')).toBe('0.00 ms')
  })

  it('accepte des nombres en plus des chaînes', () => {
    expect(formatMs(1.5)).toBe('1.5 ms')
    expect(formatMs(2000)).toBe('2.00 s')
  })
})

// ── parsePipeline ─────────────────────────────────────────────────────────────

describe('parsePipeline', () => {
  it('retourne null si aucun header de timing', () => {
    const headers = makeHeaders({})
    expect(parsePipeline(headers)).toBeNull()
  })

  it('inclut uniquement les étapes dont le header est présent', () => {
    const headers = makeHeaders({ 'X-T-Read': '5.2', 'X-T-Optimizer': '120.5' })
    const steps = parsePipeline(headers)
    expect(steps).toHaveLength(2)
    expect(steps[0].key).toBe('Read')
    expect(steps[1].key).toBe('Optimizer')
  })

  it('marque Redis comme hit quand X-Cache = HIT', () => {
    const headers = makeHeaders({ 'X-T-Redis': '0.8', 'X-Cache': 'HIT' })
    const steps = parsePipeline(headers)
    const redis = steps.find((s) => s.key === 'Redis')
    expect(redis?.status).toBe('hit')
  })

  it('marque Redis comme miss quand X-Cache ≠ HIT', () => {
    const headers = makeHeaders({ 'X-T-Redis': '1.2', 'X-Cache': 'MISS' })
    const steps = parsePipeline(headers)
    const redis = steps.find((s) => s.key === 'Redis')
    expect(redis?.status).toBe('miss')
  })

  it('marque Redis comme miss quand X-Cache est absent', () => {
    const headers = makeHeaders({ 'X-T-Redis': '1.2' })
    const steps = parsePipeline(headers)
    const redis = steps.find((s) => s.key === 'Redis')
    expect(redis?.status).toBe('miss')
  })

  it('marque RabbitMQ avec le statut rabbit', () => {
    const headers = makeHeaders({ 'X-T-Rabbit': '50.0' })
    const steps = parsePipeline(headers)
    expect(steps[0].key).toBe('RabbitMQ')
    expect(steps[0].status).toBe('rabbit')
  })

  it("préserve l'ordre du pipeline complet", () => {
    const headers = makeHeaders({
      'X-T-Read':      '5',
      'X-T-Hash':      '1',
      'X-T-Redis':     '2',
      'X-Cache':       'MISS',
      'X-T-Minio':     '10',
      'X-T-Optimizer': '120',
      'X-T-Store':     '8',
    })
    const keys = parsePipeline(headers).map((s) => s.key)
    expect(keys).toEqual(['Read', 'Hash', 'Redis', 'MinIO', 'Optimizer', 'Store'])
  })

  it('propage la valeur ms depuis le header', () => {
    const headers = makeHeaders({ 'X-T-Read': '42.5' })
    const steps = parsePipeline(headers)
    expect(steps[0].ms).toBe('42.5')
  })

  it('retourne null si tous les headers de timing sont absents', () => {
    const headers = makeHeaders({ 'X-Cache': 'HIT' })
    expect(parsePipeline(headers)).toBeNull()
  })
})
