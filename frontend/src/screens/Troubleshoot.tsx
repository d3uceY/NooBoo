import { useMemo, useState } from 'react'
import type { Mod } from '../lib/api'
import { bytes } from '../lib/format'
import { Button, Panel, PanelHeader } from '../ui'
import { LastError } from './LastError'

interface Props {
  mods: Mod[]
  onToggle: (ids: string[], enabled: boolean) => Promise<void>
}

export default function Troubleshoot({ mods, onToggle }: Props) {
  const [picked, setPicked] = useState<string[]>([])
  const [busy, setBusy] = useState(false)

  const enabled = useMemo(() => mods.filter((m) => m.enabled), [mods])
  const disabled = useMemo(() => mods.filter((m) => !m.enabled), [mods])

  const togglePick = (id: string) =>
    setPicked((prev) => (prev.includes(id) ? prev.filter((p) => p !== id) : [...prev, id]))

  const disablePicked = async () => {
    setBusy(true)
    try {
      await onToggle(picked, false)
      setPicked([])
    } finally {
      setBusy(false)
    }
  }

  const enableEverything = async () => {
    setBusy(true)
    try {
      await onToggle(
        disabled.map((m) => m.id),
        true,
      )
    } finally {
      setBusy(false)
    }
  }

  if (!mods.length) {
    return (
      <div className="w-[min(720px,100%)]">
        <LastError mods={mods} onToggle={onToggle} />
        <p className="mt-10 text-center text-sm text-ink-soft">No mods yet.</p>
      </div>
    )
  }

  return (
    <div className="w-[min(720px,100%)]">
      <LastError mods={mods} onToggle={onToggle} />

      <div className="mb-4 rounded-xl border border-[#bcdcef] bg-panel-dim px-4 py-3">
        <p className="text-sm">
          Switch off half your mods, launch the game, and keep halving until it works. Disabled mods
          are renamed, not moved, so nothing is lost.
        </p>
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Button variant="danger" onClick={disablePicked} disabled={busy || picked.length === 0}>
          Disable {picked.length > 0 ? picked.length : 'selected'}
        </Button>
        <Button variant="primary" onClick={enableEverything} disabled={busy || disabled.length === 0}>
          Enable all {disabled.length > 0 ? `(${disabled.length})` : ''}
        </Button>
      </div>

      <Panel>
        <PanelHeader title="Enabled" hint={`${enabled.length} on`} />
        <div className="divide-y divide-[#e4f0f8]">
          {enabled.map((mod) => {
            const active = picked.includes(mod.id)
            return (
              <label
                key={mod.id}
                data-row
                className={`flex cursor-pointer items-center gap-3 px-4 py-2 transition-colors ${
                  active ? 'bg-plumbob-soft' : 'hover:bg-[#f4fafe]'
                }`}
              >
                <input
                  type="checkbox"
                  checked={active}
                  onChange={() => togglePick(mod.id)}
                  className="h-4 w-4 accent-plumbob"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-display text-sm leading-tight font-semibold">
                    {mod.name}
                  </span>
                  <span className="block truncate text-[11px] text-ink-soft">
                    {mod.creator ? `${mod.creator} \u00b7 ${bytes(mod.size)}` : bytes(mod.size)}
                  </span>
                </span>
              </label>
            )
          })}
        </div>
      </Panel>

      {enabled.length === 0 && (
        <p className="mt-6 text-sm text-ink-soft">Everything is off. Enable all to start again.</p>
      )}
    </div>
  )
}
