import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { Quarantine } from '../lib/api'
import { bytes, stamp } from '../lib/format'
import { Art, Button } from '../ui'

/**
 * Set-aside window: what restores and mod removals parked, with Put back.
 */
export default function QuarantineWindow() {
  const [items, setItems] = useState<Quarantine[]>([])
  const [busy, setBusy] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      setItems((await api.quarantine()) ?? [])
    } catch (e) {
      setError(text(e))
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const putBack = async (id: string) => {
    setBusy(id)
    setNote('')
    setError('')
    try {
      await api.restoreQuarantine(id)
      setNote('Put back. The copy it replaced is now set aside.')
      await load()
    } catch (e) {
      setError(text(e))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="flex h-full flex-col px-5 pt-4 pb-3">
      <h1 className="font-display text-lg leading-none font-bold">Set aside</h1>
      <p className="mt-1 text-[11px] text-ink-soft">Parked by restores and mod removals. The game does not load these.</p>

      {error && <p className="mt-2 text-xs text-danger">{error}</p>}
      {note && <p className="mt-2 text-xs text-plumbob-dark">{note}</p>}

      <div className="scroll-soft mt-3 min-h-0 flex-1 overflow-y-auto">
        {items.length === 0 ? (
          <p className="mt-10 text-center text-sm text-ink-soft">Nothing set aside.</p>
        ) : (
          items.map((q) => (
            <div
              key={q.id}
              className="flex items-center gap-3 border-b border-[#dceaf4] py-2 last:border-0"
            >
              <Art name="backups" size={20} />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-xs font-semibold text-ink">{stamp(q.id)}</span>
                <span className="block truncate text-[11px] text-ink-soft">
                  {q.parts.join(', ') || 'empty'} · {bytes(q.size)}
                </span>
              </span>
              <Button
                size="sm"
                variant="plain"
                disabled={busy !== ''}
                onClick={() => void putBack(q.id)}
              >
                {busy === q.id ? 'Putting back…' : 'Put back'}
              </Button>
            </div>
          ))
        )}
      </div>

      <div className="mt-3 flex items-center justify-between gap-2 border-t border-[#dceaf4] pt-3">
        <Button size="sm" variant="plain" onClick={() => void api.revealQuarantine()}>
          Open folder
        </Button>
        <span className="text-[11px] text-ink-soft">Empty from Backups.</span>
      </div>
    </div>
  )
}

function text(e: unknown): string {
  const raw = e instanceof Error ? e.message : String(e)
  return raw.replace(/^Error:\s*/, '')
}
