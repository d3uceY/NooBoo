import { useCallback, useEffect, useState } from 'react'
import type { Exception, Mod } from '../lib/api'
import { api } from '../lib/api'
import { when } from '../lib/format'
import { Button, Panel, PanelHeader } from '../ui'

interface Props {
  mods: Mod[]
  onToggle: (ids: string[], enabled: boolean) => Promise<void>
}

/**
 * The game's own error log, read and matched to mods. Shows nothing at all when
 * the game has not logged an error, because usually it has not.
 */
export function LastError({ mods, onToggle }: Props) {
  const [error, setError] = useState<Exception | null>(null)
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState('')
  const [problem, setProblem] = useState('')

  const load = useCallback(async () => {
    try {
      setError(await api.lastException())
      setProblem('')
    } catch (e) {
      setProblem(clean(e))
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const disable = async (id: string) => {
    setBusy(id)
    try {
      await onToggle([id], false)
      await load()
    } catch (e) {
      setProblem(clean(e))
    } finally {
      setBusy('')
    }
  }

  if (!error?.found) {
    return problem ? <p className="mb-4 text-xs text-danger">{problem}</p> : null
  }

  const gameVersion = error.build.match(/\d+\.\d+\.\d+\.\d+/)?.[0]

  return (
    <div className="mb-4">
      <Panel>
        <PanelHeader
          title="Last error"
          hint={`${when(error.when)}${gameVersion ? ` · game ${gameVersion}` : ''}`}
        >
          <div className="flex shrink-0 gap-1.5">
            <Button size="sm" onClick={() => void load()}>
              Recheck
            </Button>
            <Button size="sm" onClick={() => void api.openException()}>
              Open log
            </Button>
          </div>
        </PanelHeader>

        <div className="px-4 py-3">
          <p className="text-sm font-semibold break-words">{error.message}</p>

          {error.suspects.length === 0 ? (
            <p className="mt-1 text-[11px] text-ink-soft">
              Nothing in Mods appears in the trace, so this one looks like game code.
            </p>
          ) : (
            <ul className="mt-2 divide-y divide-[#e4f0f8]">
              {error.suspects.map((s) => {
                const mod = target(s.id, s.label, mods)
                return (
                  <li key={`${s.label}-${s.path}`} className="flex items-center gap-3 py-1.5">
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-display text-sm font-semibold">
                        {mod?.name ?? s.label}
                      </span>
                      <span className="block truncate text-[11px] text-ink-soft">
                        {s.why}
                        {s.line > 0 ? ` · line ${s.line}` : ''}
                        {mod ? '' : ' · not installed any more'}
                        {mod && !mod.enabled ? ' · already off' : ''}
                      </span>
                    </span>
                    {mod?.enabled && (
                      <Button
                        size="sm"
                        variant="danger"
                        disabled={busy !== ''}
                        onClick={() => void disable(mod.id)}
                      >
                        Disable
                      </Button>
                    )}
                  </li>
                )
              })}
            </ul>
          )}

          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            className="mt-2 cursor-pointer text-[11px] font-semibold text-[#1d7fc4] underline decoration-dotted underline-offset-2"
          >
            {open ? 'Hide the trace' : `Show the trace (${error.trace.length})`}
          </button>

          {open && (
            <pre className="scroll-soft mt-2 max-h-40 overflow-auto rounded-lg bg-panel-dim px-3 py-2 font-mono text-[10px] leading-relaxed whitespace-pre-wrap text-ink-soft">
              {error.trace.join('\n')}
            </pre>
          )}

          {problem && <p className="mt-2 text-xs text-danger">{problem}</p>}
        </div>
      </Panel>
    </div>
  )
}

/** Finds the installed mod a suspect points at, by id or by name. */
function target(id: string, label: string, mods: Mod[]): Mod | undefined {
  if (!id && !label) return undefined
  return mods.find(
    (m) =>
      (id !== '' && (m.id === id || m.id.split('/')[0] === id)) ||
      (label !== '' && m.name.toLowerCase() === label.toLowerCase()),
  )
}

function clean(e: unknown): string {
  return (e instanceof Error ? e.message : String(e)).replace(/^Error:\s*/, '')
}
