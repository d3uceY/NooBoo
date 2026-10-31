import { useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { api } from '../lib/api'
import type { DuplicateSet, Mod, Pack, TidyResult, TrayItem } from '../lib/api'
import { bytes, when } from '../lib/format'
import { revealRows } from '../lib/motion'
import { Art, Button, Dialog, Field, Section, Switch } from '../ui'

interface Props {
  mods: Mod[]
  onToggle: (ids: string[], enabled: boolean) => Promise<void>
  onDelete: (id: string) => Promise<void>
  onTidy: () => Promise<TidyResult>
  onReveal: (path: string) => void
  onRefresh: () => Promise<void>
}

type Sort = 'name' | 'creator' | 'size' | 'newest'
type View = 'list' | 'grid'

const kindLabel: Record<string, string> = {
  household: 'Household',
  lot: 'Lot',
  room: 'Room',
}

const packKindLabel: Record<string, string> = {
  expansion: 'Expansion',
  'game pack': 'Game pack',
  'stuff pack': 'Stuff pack',
  kit: 'Kit',
  free: 'Free',
}

const advisoryLabel: Record<string, string> = {
  broken: 'Broken',
  outdated: 'Outdated',
  caution: 'Caution',
}

export default function Library({ mods, onToggle, onDelete, onTidy, onReveal, onRefresh }: Props) {
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<Sort>('name')
  const [view, setView] = useState<View>(() =>
    localStorage.getItem('nooboo.library.view') === 'grid' ? 'grid' : 'list',
  )
  const [kind, setKind] = useState('')
  const [tag, setTag] = useState('')
  const [busy, setBusy] = useState('')
  const [removing, setRemoving] = useState<Mod | null>(null)
  const [detailId, setDetailId] = useState<string | null>(null)
  const [tidying, setTidying] = useState(false)
  const [tidyDone, setTidyDone] = useState<TidyResult | null>(null)
  const [dupes, setDupes] = useState<DuplicateSet[] | null>(null)
  const [finding, setFinding] = useState(false)
  const [tray, setTray] = useState<TrayItem[]>([])
  const [packs, setPacks] = useState<Pack[]>([])
  const [packRoot, setPackRoot] = useState('')
  const [packBusy, setPackBusy] = useState('')
  const [note, setNote] = useState('')
  const [tagDraft, setTagDraft] = useState('')
  const scope = useRef<HTMLDivElement>(null)

  const detail = useMemo(() => mods.find((m) => m.id === detailId) ?? null, [mods, detailId])

  const kinds = useMemo(
    () => [...new Set(mods.map((m) => m.category).filter(Boolean))].sort(),
    [mods],
  )

  const tags = useMemo(() => {
    const set = new Set<string>()
    for (const mod of mods) for (const t of mod.tags ?? []) set.add(t)
    return [...set].sort((a, b) => a.localeCompare(b))
  }, [mods])

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = mods.filter((m) => {
      if (q && !`${m.name} ${m.creator}`.toLowerCase().includes(q)) return false
      if (kind && m.category !== kind) return false
      if (tag && !(m.tags ?? []).includes(tag)) return false
      return true
    })
    return [...list].sort((a, b) => {
      if (a.enabled !== b.enabled) return a.enabled ? -1 : 1
      switch (sort) {
        case 'size':
          return b.size - a.size
        case 'newest':
          return (b.modified ?? '').localeCompare(a.modified ?? '')
        case 'creator':
          return `${a.creator}\u0000${a.name}`.localeCompare(`${b.creator}\u0000${b.name}`)
        default:
          return a.name.localeCompare(b.name)
      }
    })
  }, [mods, query, kind, tag, sort])

  useEffect(() => {
    void api.trayItems().then(setTray).catch(() => undefined)
    void api.packRoot().then(setPackRoot).catch(() => undefined)
    void api.packs().then(setPacks).catch(() => undefined)
  }, [])

  useEffect(() => {
    revealRows(scope.current)
  }, [visible.length])

  useEffect(() => {
    localStorage.setItem('nooboo.library.view', view)
  }, [view])

  const plain = visible.filter((m) => m.kind === 'mod')
  const cc = visible.filter((m) => m.kind === 'cc')
  // A loose file is its own id and still carries a mod extension.
  const loose = mods.filter((m) => /\.(package|ts4script)(\.off|\.disabled)?$/i.test(m.id))
  const on = mods.filter((m) => m.enabled).length
  const clashing = mods.filter((m) => m.conflicts.length > 0).length

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

  const findCopies = async () => {
    setFinding(true)
    setNote('')
    try {
      setDupes(await api.findDuplicates())
    } catch (e) {
      setNote(message(e))
    } finally {
      setFinding(false)
    }
  }

  const setAside = async (set: DuplicateSet) => {
    setNote('')
    try {
      for (const f of set.files.slice(1)) await api.setAsideFile(f.file)
      setDupes(null)
      await onRefresh()
    } catch (e) {
      setNote(message(e))
    }
  }

  const ignore = async (other: string) => {
    if (!detail) return
    setNote('')
    try {
      await api.ignoreConflict(detail.id, other)
      await onRefresh()
    } catch (e) {
      setNote(message(e))
    }
  }

  const addTag = async () => {
    if (!detail || !tagDraft.trim()) return
    try {
      await api.addTag(detail.id, tagDraft)
      setTagDraft('')
      await onRefresh()
    } catch (e) {
      setNote(message(e))
    }
  }

  const removeTag = async (t: string) => {
    if (!detail) return
    try {
      await api.removeTag(detail.id, t)
      await onRefresh()
    } catch (e) {
      setNote(message(e))
    }
  }

  const flipPack = async (pack: Pack, enabled: boolean) => {
    setPackBusy(pack.code)
    setNote('')
    try {
      await api.setPack(pack.code, enabled)
      setPacks(await api.packs())
    } catch (e) {
      setNote(message(e))
    } finally {
      setPackBusy('')
    }
  }

  const findInstall = async () => {
    setNote('')
    try {
      await api.pickInstallFolder()
      setPackRoot(await api.packRoot())
      setPacks(await api.packs())
    } catch (e) {
      setNote(message(e))
    }
  }

  const shared = {
    onFlip: flip,
    onReveal: (m: Mod) => onReveal(m.path),
    onRemove: setRemoving,
    onDetail: (m: Mod) => setDetailId(m.id),
  }

  return (
    <div ref={scope} className="max-w-[760px]">
      <div className="mb-2 flex items-end justify-between gap-4">
        <div className="w-full max-w-[320px]">
          <Field label="Search" value={query} onChange={setQuery} placeholder="Name or creator" />
        </div>
        <div className="flex shrink-0 items-center gap-3 pb-1">
          {query.trim() !== '' && (
            <p className="text-[11px] text-ink-soft">
              {visible.length} of {mods.length}
            </p>
          )}
          {loose.length > 0 && (
            <Button size="sm" variant="plain" disabled={busy !== ''} onClick={() => setTidying(true)}>
              Tidy up {loose.length}
            </Button>
          )}
        </div>
      </div>

      <div className="mb-3 flex flex-wrap items-center gap-2 text-[11px] text-ink-soft">
        <span>
          {mods.length} mods · {on} on · {mods.length - on} off
          {clashing > 0 && ` · ${clashing} clashing`}
        </span>
        <div className="ml-auto flex items-center gap-1.5">
          <select
            value={sort}
            onChange={(e) => setSort(e.target.value as Sort)}
            aria-label="Sort"
            className="cursor-pointer rounded-lg border border-[#c3d8e6] bg-white px-2 py-1 text-[11px] font-semibold text-ink focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
          >
            <option value="name">Name</option>
            <option value="creator">Creator</option>
            <option value="size">Size</option>
            <option value="newest">Newest</option>
          </select>
          <div className="flex items-center gap-0.5 rounded-lg border border-[#c3d8e6] bg-white p-0.5">
            <ViewButton
              active={view === 'list'}
              art="viewList"
              label="List view"
              onClick={() => setView('list')}
            />
            <ViewButton
              active={view === 'grid'}
              art="viewGrid"
              label="Grid view"
              onClick={() => setView('grid')}
            />
          </div>
        </div>
        <Button size="sm" variant="plain" onClick={findCopies} disabled={finding}>
          {finding ? 'Checking…' : 'Find copies'}
        </Button>
      </div>

      {(kinds.length > 1 || tags.length > 0) && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          <Chip
            active={!kind && !tag}
            onClick={() => {
              setKind('')
              setTag('')
            }}
          >
            All
          </Chip>
          {kinds.map((k) => (
            <Chip key={k} active={kind === k} onClick={() => setKind(kind === k ? '' : k)}>
              {k}
            </Chip>
          ))}
          {tags.map((t) => (
            <Chip key={t} active={tag === t} onClick={() => setTag(tag === t ? '' : t)}>
              {t}
            </Chip>
          ))}
        </div>
      )}

      {note && <p className="mb-3 text-[11px] text-danger">{note}</p>}

      {dupes && (
        <div className="mb-3 rounded-xl border border-[#bcdcef] bg-panel-dim px-3 py-2">
          {dupes.length === 0 ? (
            <p className="text-xs text-ink-soft">No copies.</p>
          ) : (
            <ul className="space-y-2">
              {dupes.map((set) => (
                <li key={set.digest} className="flex items-center gap-3">
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-mono text-[11px]">{set.files[0].file}</span>
                    <span className="block text-[11px] text-ink-soft">
                      {set.files.length - 1} extra · {bytes(set.wasted)} spare
                    </span>
                  </span>
                  <Button size="sm" variant="danger" onClick={() => setAside(set)}>
                    Set aside extras
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {mods.length === 0 && (
        <p className="mt-6 text-center text-sm text-ink-soft">No mods yet. Add one from Install.</p>
      )}
      {mods.length > 0 && visible.length === 0 && (
        <p className="mt-10 text-center text-sm text-ink-soft">No matches.</p>
      )}

      {plain.length > 0 && (
        <Section title="Mods" count={plain.length} art="mod">
          {view === 'grid' ? (
            <ModGrid mods={plain} busy={busy} onFlip={flip} onDetail={(m) => setDetailId(m.id)} />
          ) : (
            plain.map((mod) => (
              <Row key={mod.id} mod={mod} busy={busy === mod.id} {...shared} />
            ))
          )}
        </Section>
      )}

      {cc.length > 0 && (
        <Section title="Custom content" count={cc.length} art="cc">
          {view === 'grid' ? (
            <ModGrid mods={cc} busy={busy} onFlip={flip} onDetail={(m) => setDetailId(m.id)} />
          ) : (
            cc.map((mod) => (
              <Row key={mod.id} mod={mod} busy={busy === mod.id} {...shared} />
            ))
          )}
        </Section>
      )}

      {tray.length > 0 && (
        <Section title="Creations" count={tray.length} art="backups">
          {tray.map((item) => (
            <TrayRow key={item.id} item={item} onReveal={onReveal} />
          ))}
        </Section>
      )}

      {packRoot === '' && (
        <div className="mb-3 flex items-center gap-2 rounded-xl border border-[#bcdcef] bg-panel-dim px-3 py-2 text-[11px]">
          <span className="min-w-0 flex-1 text-ink-soft">Game install not found.</span>
          <Button size="sm" variant="plain" onClick={findInstall}>
            Find game folder
          </Button>
        </div>
      )}

      {packs.length > 0 && (
        <Section title="Packs" count={packs.length} art="install" defaultOpen={false}>
          <p className="px-2 pt-1 text-[11px] text-ink-soft">Applies next launch.</p>
          {packs.map((pack) => (
            <div key={pack.code} data-row className="flex items-center gap-3 rounded-lg px-2 py-1.5">
              <span className="w-10 shrink-0 rounded bg-panel-dim px-1 py-0.5 text-center font-mono text-[10px] text-ink-soft">
                {pack.code}
              </span>
              <span className="min-w-0 flex-1 truncate font-display text-sm font-semibold">
                {pack.name}
              </span>
              <span className="shrink-0 text-[10px] text-ink-soft">
                {packKindLabel[pack.kind] ?? pack.kind}
              </span>
              <Switch
                on={pack.enabled}
                disabled={packBusy === pack.code}
                label={`${pack.enabled ? 'Disable' : 'Enable'} ${pack.name}`}
                onChange={(next) => flipPack(pack, next)}
              />
            </div>
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
        onClose={() => setDetailId(null)}
        title={detail?.name ?? ''}
        footer={
          <>
            <Button variant="plain" onClick={() => setDetailId(null)}>
              Close
            </Button>
            <Button variant="blue" onClick={() => detail && onReveal(detail.path)}>
              Folder
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                if (!detail) return
                setRemoving(detail)
                setDetailId(null)
              }}
            >
              Remove
            </Button>
          </>
        }
      >
        {detail && (
          <>
            <div className="flex items-start gap-3">
              <Thumb id={detail.id} has={detail.hasThumb} kind={detail.kind} size={64} />
              <dl className="grid flex-1 grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
                <dt className="text-ink-soft">Type</dt>
                <dd>{detail.kind === 'cc' ? 'Custom content' : 'Mod'}</dd>
                <dt className="text-ink-soft">Kind</dt>
                <dd>{detail.category}</dd>
                {detail.creator && (
                  <>
                    <dt className="text-ink-soft">Creator</dt>
                    <dd className="min-w-0 truncate">{detail.creator}</dd>
                  </>
                )}
                <dt className="text-ink-soft">Size</dt>
                <dd>{bytes(detail.size)}</dd>
                <dt className="text-ink-soft">Changed</dt>
                <dd>{when(detail.modified)}</dd>
                <dt className="text-ink-soft">Status</dt>
                <dd>{detail.enabled ? 'Enabled' : 'Disabled'}</dd>
                <dt className="text-ink-soft">Files</dt>
                <dd>{detail.files.length}</dd>
                <dt className="text-ink-soft">Resources</dt>
                <dd>{detail.resourceCount}</dd>
              </dl>
            </div>

            {detail.advisory && (
              <div className="mt-3 rounded-lg border border-[#f0d48a] bg-[#fff8e1] px-3 py-2 text-xs">
                <p className="font-semibold">{detail.advisory.title}</p>
                <p className="mt-0.5 text-ink-soft">
                  {advisoryLabel[detail.advisory.status] ?? detail.advisory.status}
                  {detail.advisory.since ? ` · ${detail.advisory.since}` : ''}
                </p>
                {detail.advisory.note && <p className="mt-1">{detail.advisory.note}</p>}
                {detail.advisory.url && (
                  <button
                    type="button"
                    onClick={() => void api.openLink(detail.advisory?.url ?? '')}
                    className="mt-1 cursor-pointer font-semibold text-[#1d7fc4] underline decoration-dotted underline-offset-2"
                  >
                    Fix
                  </button>
                )}
              </div>
            )}

            {detail.conflicts.length > 0 && (
              <div className="mt-3">
                <p className="text-[11px] font-semibold text-ink-soft">Clashes</p>
                <ul className="mt-1 space-y-1">
                  {detail.conflicts.map((c) => (
                    <li
                      key={c.id}
                      className="flex items-center gap-2 rounded-lg bg-panel-dim px-2 py-1 text-[11px]"
                    >
                      <span className="min-w-0 flex-1 truncate">
                        {c.id.split('/').pop()} · {c.reason}
                      </span>
                      <Button size="sm" variant="plain" onClick={() => void ignore(c.id)}>
                        Ignore
                      </Button>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            <div className="mt-3">
              <p className="text-[11px] font-semibold text-ink-soft">Tags</p>
              <div className="mt-1 flex flex-wrap items-center gap-1.5">
                {(detail.tags ?? []).map((t) => (
                  <span
                    key={t}
                    className="inline-flex items-center gap-1 rounded-full bg-panel-dim px-2 py-0.5 text-[11px]"
                  >
                    {t}
                    <button
                      type="button"
                      onClick={() => void removeTag(t)}
                      aria-label={`Remove ${t}`}
                      className="cursor-pointer text-ink-soft hover:text-danger"
                    >
                      ×
                    </button>
                  </span>
                ))}
                <input
                  value={tagDraft}
                  onChange={(e) => setTagDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') void addTag()
                  }}
                  placeholder="Add tag"
                  maxLength={24}
                  className="w-28 rounded-full border border-[#c3d8e6] px-2 py-0.5 text-[11px] focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
                />
              </div>
            </div>

            <p className="mt-3 rounded-lg bg-panel-dim px-2 py-1.5 font-mono text-[10px] break-all text-ink">
              {detail.path}
            </p>

            {!!detail.files.length && (
              <ul className="scroll-soft mt-2 max-h-36 overflow-y-auto rounded-lg border border-[#d8e8f3] bg-white px-2 py-1.5 font-mono text-[11px]">
                {detail.files.map((f) => (
                  <li key={f} className="truncate py-0.5 text-ink-soft" title={f}>
                    {f}
                  </li>
                ))}
              </ul>
            )}
          </>
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
      <Thumb id={mod.id} has={mod.hasThumb} kind={mod.kind} />
      <span className="min-w-0 flex-1">
        <button
          type="button"
          onClick={() => onDetail(mod)}
          title="Details"
          className="block max-w-full cursor-pointer truncate text-left font-display text-sm leading-tight font-semibold transition-colors hover:text-[#1d7fc4] focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
        >
          {mod.name}
        </button>
        <span className="block truncate text-[11px] text-ink-soft">
          {mod.creator ? `${mod.creator} · ${bytes(mod.size)}` : bytes(mod.size)} · {mod.category}
          {!mod.enabled && ' · off'}
        </span>
      </span>
      {(mod.conflicts.length > 0 || mod.advisory) && (
        <span className="flex shrink-0 items-center gap-1">
          {mod.conflicts.length > 0 && (
            <span
              title={mod.conflicts.map((c) => `${c.id.split('/').pop()}: ${c.reason}`).join('\n')}
              className="rounded-full bg-[#ffe9e5] px-1.5 py-0.5 text-[10px] font-bold text-danger"
            >
              {mod.conflicts.length}
            </span>
          )}
          {mod.advisory && (
            <span
              title={mod.advisory.title}
              className="rounded-full bg-[#fff3cd] px-1.5 py-0.5 text-[10px] font-bold text-[#8a6100]"
            >
              !
            </span>
          )}
        </span>
      )}
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

function ModGrid({
  mods,
  busy,
  onFlip,
  onDetail,
}: {
  mods: Mod[]
  busy: string
  onFlip: (m: Mod, on: boolean) => void
  onDetail: (m: Mod) => void
}) {
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
      {mods.map((mod) => (
        <ModCard key={mod.id} mod={mod} busy={busy === mod.id} onFlip={onFlip} onDetail={onDetail} />
      ))}
    </div>
  )
}

function ModCard({
  mod,
  busy,
  onFlip,
  onDetail,
}: {
  mod: Mod
  busy: boolean
  onFlip: (m: Mod, on: boolean) => void
  onDetail: (m: Mod) => void
}) {
  return (
    <div
      data-row
      className={`flex h-full flex-col gap-2 rounded-xl border p-2 transition-colors ${
        mod.enabled
          ? 'border-[#d8e8f3] bg-white hover:border-[#8dbfe0]'
          : 'border-[#e3ebf1] bg-[#f2f6f9]'
      }`}
    >
      <button
        type="button"
        onClick={() => onDetail(mod)}
        title="Details"
        className="flex h-28 w-full cursor-pointer items-center justify-center overflow-hidden rounded-lg bg-panel-dim focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
      >
        <Thumb id={mod.id} has={mod.hasThumb} kind={mod.kind} size={96} />
      </button>
      <div className="min-w-0">
        <button
          type="button"
          onClick={() => onDetail(mod)}
          title="Details"
          className="block w-full cursor-pointer truncate text-left font-display text-sm leading-tight font-semibold transition-colors hover:text-[#1d7fc4] focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
        >
          {mod.name}
        </button>
        <p className="truncate text-[11px] text-ink-soft">
          {mod.creator ? `${mod.creator} · ` : ''}
          {bytes(mod.size)}
          {!mod.enabled && ' · off'}
        </p>
      </div>
      <div className="mt-auto flex items-center justify-between gap-1">
        <span className="flex min-w-0 items-center gap-1">
          <span className="rounded bg-panel-dim px-1.5 py-0.5 text-[9px] font-semibold text-ink-soft">
            {mod.category}
          </span>
          {mod.conflicts.length > 0 && (
            <span
              title={mod.conflicts.map((c) => `${c.id.split('/').pop()}: ${c.reason}`).join('\n')}
              className="rounded-full bg-[#ffe9e5] px-1.5 py-0.5 text-[10px] font-bold text-danger"
            >
              {mod.conflicts.length}
            </span>
          )}
          {mod.advisory && (
            <span
              title={mod.advisory.title}
              className="rounded-full bg-[#fff3cd] px-1.5 py-0.5 text-[10px] font-bold text-[#8a6100]"
            >
              !
            </span>
          )}
        </span>
        <Switch
          on={mod.enabled}
          disabled={busy}
          label={`${mod.enabled ? 'Disable' : 'Enable'} ${mod.name}`}
          onChange={(next) => onFlip(mod, next)}
        />
      </div>
    </div>
  )
}

function ViewButton({
  active,
  art,
  label,
  onClick,
}: {
  active: boolean
  art: 'viewList' | 'viewGrid'
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      aria-label={label}
      title={label}
      className={`flex h-6 w-6 cursor-pointer items-center justify-center rounded-md transition-colors focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none ${
        active ? 'bg-plumbob-soft' : 'hover:bg-panel-dim'
      }`}
    >
      <Art name={art} size={18} className={active ? '' : 'opacity-55'} />
    </button>
  )
}

function Thumb({
  id,
  has,
  kind,
  size = 32,
}: {
  id: string
  has: boolean
  kind: Mod['kind']
  size?: number
}) {
  const [src, setSrc] = useState('')

  useEffect(() => {
    if (!has) {
      setSrc('')
      return
    }
    let alive = true
    void api
      .thumbnail(id)
      .then((url) => {
        if (alive) setSrc(url)
      })
      .catch(() => undefined)
    return () => {
      alive = false
    }
  }, [id, has])

  if (src) {
    return (
      <img
        src={src}
        alt=""
        className="shrink-0 rounded-md border border-[#d8e8f3] object-cover"
        style={{ width: size, height: size }}
      />
    )
  }
  return (
    <span
      className="flex shrink-0 items-center justify-center rounded-md bg-panel-dim"
      style={{ width: size, height: size }}
    >
      <Art name={kind === 'cc' ? 'cc' : 'mod'} size={Math.round(size * 0.7)} />
    </span>
  )
}

function TrayRow({ item, onReveal }: { item: TrayItem; onReveal: (path: string) => void }) {
  return (
    <div data-row className="flex items-center gap-3 rounded-lg px-2 py-1.5 hover:bg-[#f4fafe]">
      {item.thumb ? (
        <img
          src={item.thumb}
          alt=""
          className="h-10 w-10 shrink-0 rounded-md border border-[#d8e8f3] object-cover"
        />
      ) : (
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-panel-dim">
          <Art name="backups" size={22} />
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate font-display text-sm font-semibold">{item.name}</span>
        <span className="block truncate text-[11px] text-ink-soft">
          {kindLabel[item.kind] ?? item.kind} · {bytes(item.size)}
          {item.creator ? ` · ${item.creator}` : ''}
        </span>
      </span>
      <Button size="sm" variant="plain" onClick={() => onReveal(item.path)}>
        Show
      </Button>
    </div>
  )
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`cursor-pointer rounded-full border px-2.5 py-0.5 text-[11px] font-semibold transition-colors focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none ${
        active
          ? 'border-[#3f7a17] bg-plumbob-soft text-plumbob-dark'
          : 'border-[#c3d8e6] bg-white text-ink-soft hover:border-[#8dbfe0]'
      }`}
    >
      {children}
    </button>
  )
}

function message(e: unknown): string {
  const text = e instanceof Error ? e.message : String(e)
  return text.replace(/^Error:\s*/, '')
}
