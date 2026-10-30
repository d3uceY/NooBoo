import { useEffect, useMemo, useRef, useState } from 'react'
import type { Mod, TidyResult } from '../lib/api'
import { bytes, when } from '../lib/format'
import { revealRows } from '../lib/motion'
import { Button, Dialog, Field, Section, Switch } from '../ui'

interface Props {
  mods: Mod[]
  onToggle: (ids: string[], enabled: boolean) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onTidy: () => Promise<TidyResult>
  onReveal: (path: string) => void
}

export default function Library({ mods, onToggle, onDelete, onTidy, onReveal }: Props) {
  const [query, setQuery] = useState('')
  const [busy, setBusy] = useState('')
  const [removing, setRemoving] = useState<Mod | null>(null)
  const [detail, setDetail] = useState<Mod | null>(null)
  const [tidying, setTidying] = useState(false)
  const [tidyDone, setTidyDone] = useState<TidyResult | null>(null)
  const scope = useRef<HTMLDivElement>(null)

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return mods
    return mods.filter((m) => `${m.name} ${m.creator}`.toLowerCase().includes(q))
  }, [mods, query])

  useEffect(() => {
    revealRows(scope.current)
  }, [filtered.length])

  const plain = filtered.filter((m) => m.kind === 'mod')
  const cc = filtered.filter((m) => m.kind === 'cc')
  // A loose file is its own id and still carries a mod extension.
  const loose = mods.filter((m) => /\.(package|ts4script)(\.off)?$/i.test(m.id))

  const runTidy = async () => {
    setTidying(false)
    setBusy('tidy')
    try {
      setTidyDone(await onTidy())
    } catch {
      // App puts the reason in the banner above.
    } finally {
      setBusy('')
    }
  }

  const flip = async (mod: Mod, enabled: boolean) => {
    setBusy(mod.id)
    try {
      await onToggle([mod.id], enabled)
    } finally {
      setBusy('')
    }
  }

  const confirmRemove = async () => {
    if (!removing) return
    const target = removing
    setRemoving(null)
    setBusy(target.id)
    try {
      await onDelete(target.id)
    } finally {
      setBusy('')
    }
  }

  if (!mods.length) {
    return <p className="mt-16 text-center text-sm text-ink-soft">No mods yet. Add one from Install.</p>
  }

  const shared = {
    onFlip: flip,
    onReveal: (m: Mod) => onReveal(m.path),
    onRemove: setRemoving,
    onDetail: setDetail,
  }

  return (
    <div ref={scope} className="max-w-[760px]">
      <div className="mb-4 flex items-end justify-between gap-4">
        <div className="w-full max-w-[320px]">
          <Field label="Search" value={query} onChange={setQuery} placeholder="Name or creator" />
        </div>
        <div className="flex shrink-0 items-center gap-3 pb-1">
          {query.trim() !== '' && (
            <p className="text-[11px] text-ink-soft">
              {filtered.length} of {mods.length}
            </p>
          )}
          {loose.length > 0 && (
            <Button size="sm" variant="plain" disabled={busy !== ''} onClick={() => setTidying(true)}>
              Tidy up {loose.length}
            </Button>
          )}
        </div>
      </div>

      {filtered.length === 0 && <p className="mt-10 text-center text-sm text-ink-soft">No matches.</p>}

      {plain.length > 0 && (
        <Section title="Mods" count={plain.length} art="mod">
          {plain.map((mod) => (
            <Row key={mod.id} mod={mod} busy={busy === mod.id} {...shared} />
          ))}
        </Section>
      )}

      {cc.length > 0 && (
        <Section title="Custom content" count={cc.length} art="cc">
          {cc.map((mod) => (
            <Row key={mod.id} mod={mod} busy={busy === mod.id} {...shared} />
          ))}
        </Section>
      )}

      <Dialog
        open={removing !== null}
        onClose={() => setRemoving(null)}
        title="Remove mod?"
        footer={
          <>
            <Button variant="plain" onClick={() => setRemoving(null)}>
              Cancel
            </Button>
            <Button variant="danger" onClick={confirmRemove}>
              Remove
            </Button>
          </>
        }
      >
        <p className="text-sm">
          <span className="font-semibold">{removing?.name}</span> leaves your Mods folder and is kept
          under quarantine, so you can put it back.
        </p>
      </Dialog>

      <Dialog
        open={tidying}
        onClose={() => setTidying(false)}
        title="Group loose files?"
        footer={
          <>
            <Button variant="plain" onClick={() => setTidying(false)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={runTidy}>
              Tidy up
            </Button>
          </>
        }
      >
        <p className="text-sm">
          {loose.length} files sit directly in Mods. Each gets a folder named after it, so a package
          and its script switch together instead of separately. Names are left exactly as they are.
        </p>
      </Dialog>

      <Dialog
        open={tidyDone !== null}
        onClose={() => setTidyDone(null)}
        title="Tidied"
        footer={
          <Button variant="primary" onClick={() => setTidyDone(null)}>
            Close
          </Button>
        }
      >
        {tidyDone && (
          <>
            <p className="text-sm">
              {tidyDone.moved.length === 0
                ? 'Nothing to move.'
                : `${tidyDone.moved.length} files into ${tidyDone.folders} folders.`}
            </p>
            {tidyDone.moved.length > 0 && (
              <ul className="scroll-soft mt-2 max-h-40 overflow-auto rounded-lg bg-panel-dim px-3 py-2 text-[11px] text-ink-soft">
                {tidyDone.moved.map((m) => (
                  <li key={m.file} className="truncate">
                    {m.file} → {m.into}/
                  </li>
                ))}
              </ul>
            )}
            {tidyDone.skipped.length > 0 && (
              <p className="mt-2 text-[11px] text-ink-soft">
                Left alone: {tidyDone.skipped.join(', ')}. Their name is already taken in Mods.
              </p>
            )}
          </>
        )}
      </Dialog>

      <Dialog
        open={detail !== null}
        onClose={() => setDetail(null)}
        title={detail?.name ?? ''}
        footer={
          <>
            <Button variant="plain" onClick={() => setDetail(null)}>
              Close
            </Button>
            <Button variant="blue" onClick={() => detail && onReveal(detail.path)}>
              Folder
            </Button>
          </>
        }
      >
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
          <dt className="text-ink-soft">Type</dt>
          <dd>{detail?.kind === 'cc' ? 'Custom content' : 'Mod'}</dd>
          {detail?.creator && (
            <>
              <dt className="text-ink-soft">Creator</dt>
              <dd className="min-w-0 truncate">{detail.creator}</dd>
            </>
          )}
          <dt className="text-ink-soft">Size</dt>
          <dd>{bytes(detail?.size ?? 0)}</dd>
          <dt className="text-ink-soft">Changed</dt>
          <dd>{when(detail?.modified)}</dd>
          <dt className="text-ink-soft">Status</dt>
          <dd>{detail?.enabled ? 'Enabled' : 'Disabled'}</dd>
          <dt className="text-ink-soft">Files</dt>
          <dd>{detail?.files.length ?? 0}</dd>
        </dl>

        <p className="mt-3 rounded-lg bg-panel-dim px-2 py-1.5 font-mono text-[10px] break-all text-ink">
          {detail?.path}
        </p>

        {!!detail?.files.length && (
          <ul className="scroll-soft mt-2 max-h-36 overflow-y-auto rounded-lg border border-[#d8e8f3] bg-white px-2 py-1.5 font-mono text-[11px]">
            {detail.files.map((f) => (
              <li key={f} className="truncate py-0.5 text-ink-soft" title={f}>
                {f}
              </li>
            ))}
          </ul>
        )}
      </Dialog>
    </div>
  )
}

function Row({
  mod,
  busy,
  onFlip,
  onReveal,
  onRemove,
  onDetail,
}: {
  mod: Mod
  busy: boolean
  onFlip: (m: Mod, on: boolean) => void
  onReveal: (m: Mod) => void
  onRemove: (m: Mod) => void
  onDetail: (m: Mod) => void
}) {
  return (
    <div
      data-row
      className={`group flex items-center gap-3 rounded-lg px-2 py-1.5 transition-colors ${
        mod.enabled ? 'hover:bg-[#f4fafe]' : 'bg-[#f2f6f9]'
      }`}
    >
      <span className="min-w-0 flex-1">
        <button
          type="button"
          onClick={() => onDetail(mod)}
          title="Details"
          className="block max-w-full cursor-pointer truncate text-left font-display text-sm leading-tight font-semibold transition-colors hover:text-[#1d7fc4] focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
        >
          {mod.name}
        </button>
        <span className={`block truncate text-[11px] ${mod.enabled ? 'text-ink-soft' : 'text-ink-soft'}`}>
          {mod.creator ? `${mod.creator} · ${bytes(mod.size)}` : bytes(mod.size)}
          {!mod.enabled && ' · off'}
        </span>
      </span>
      <span className="flex shrink-0 items-center gap-1 opacity-50 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
        <Button size="sm" variant="plain" onClick={() => onReveal(mod)} title="Show this folder in Explorer">
          Folder
        </Button>
        <Button size="sm" variant="danger" onClick={() => onRemove(mod)} title="Remove from Mods">
          Remove
        </Button>
      </span>
      <Switch
        on={mod.enabled}
        disabled={busy}
        label={`${mod.enabled ? 'Disable' : 'Enable'} ${mod.name}`}
        onChange={(next) => onFlip(mod, next)}
      />
    </div>
  )
}
