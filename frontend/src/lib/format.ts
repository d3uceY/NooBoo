/** Human-readable byte size, short enough for a row subtitle. */
export function bytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB']
  let value = n / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unit]}`
}

/** Snapshot id "2026-10-29_14-32-05" as a readable date. */
export function stamp(id: string): string {
  const [date, time] = id.split('_')
  if (!time) return id
  return `${date} · ${time.replace(/-/g, ':')}`
}

/** RFC3339 timestamp as a short local date, or a dash when unknown. */
export function when(iso: string | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}
