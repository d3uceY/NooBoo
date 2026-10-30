import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { Backup, Storage } from '../lib/api'
import { bytes, stamp } from '../lib/format'
import { Art, Button, Dialog, Panel, PanelHeader } from '../ui'

type Pending = { action: 'restore' | 'delete' | 'empty'; id: string }

const emptyStorage: Storage = {
  snapshots: 0,
  snapshotSize: 0,
  partial: 0,
  quarantine: 0,
  quarantineSize: 0,
}

export default function Backups() {
  const [list, setList] = useState<Backup[]>([])
  const [storage, setStorage] = useState<Storage>(emptyStorage)
  const [busy, setBusy] = useState('')
  const [pending, setPending] = useState<Pending | null>(null)
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const [keep, setKeep] = useState(10)

  const load = useCallback(async () => {
    try {
      const [snapshots, usage, limit] = await Promise.all([
        api.backups(),
        api.backupStorage(),
        api.keepLast(),
      ])
      setList(snapshots ?? [])
      setStorage(usage ?? emptyStorage)
      setKeep(limit)
    } catch (e) {
      setError(message(e))
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const backupNow = async () => {
    setBusy('new')
    setError('')
    setNote('')
    try {
      const made = await api.backupNow()
      setNote(`Backed up ${made.files} files (${bytes(made.size)})`)
      await load()
    } catch (e) {
      setError(message(e))
    } finally {
      setBusy('')
    }
  }

  const confirm = async () => {
    if (!pending) return
    const { action, id } = pending
    setPending(null)
    setBusy(action)
    setError('')
    setNote('')
    try {
      if (action === 'empty') {
        await api.emptyQuarantine()
        setNote('Quarantine emptied.')
      } else if (action === 'restore') {
        await api.restore(id)
        setNote('Restored. Your previous files were kept in quarantine.')
      } else {
        await api.deleteBackup(id)
        setNote('Deleted.')
      }
      await load()
    } catch (e) {
      setError(message(e))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="w-[min(720px,100%)]">
      <div className="mb-2 flex items-center gap-2">
        <Button variant="primary" onClick={backupNow} disabled={busy !== ''}>
          {busy === 'new' ? 'Backing up…' : 'Back up now'}
        </Button>
        <Button variant="plain" onClick={() => void api.revealBackups()}>
          Open folder
        </Button>
      </div>

      <StorageLine
        storage={storage}
        keep={keep}
        onKeep={(n) => {
          setKeep(n)
          void api.setKeepLast(n).catch((e) => setError(message(e)))
        }}
        onEmpty={() => setPending({ action: 'empty', id: '' })}
        disabled={busy !== ''}
      />

      {error && <p className="mb-3 text-xs text-danger">{error}</p>}
      {note && <p className="mb-3 text-xs text-plumbob-dark">{note}</p>}

      {list.length === 0 ? (
        <p className="mt-8 text-sm text-ink-soft">No snapshots yet.</p>
      ) : (
        <Panel>
          <PanelHeader title="Snapshots" hint="newest first" />
          <div className="divide-y divide-[#e4f0f8]">
            {list.map((b) => (
              <div key={b.id} className="flex items-center gap-3 px-4 py-2.5">
                <Art name="backups" size={24} />
                <span className="min-w-0 flex-1">
                  <span className="block font-display text-sm font-semibold">{stamp(b.id)}</span>
                  <span className="block text-[11px] text-ink-soft">
                    {b.files} files · {bytes(b.size)}
                  </span>
                </span>
                <Button
                  variant="blue"
                  onClick={() => setPending({ action: 'restore', id: b.id })}
                  disabled={busy !== ''}
                >
                  Restore
                </Button>
                <Button
                  variant="danger"
                  onClick={() => setPending({ action: 'delete', id: b.id })}
                  disabled={busy !== ''}
                >
                  Delete
                </Button>
              </div>
            ))}
          </div>
        </Panel>
      )}

      <Dialog
        open={pending !== null}
        onClose={() => setPending(null)}
        title={pendingTitle(pending)}
        footer={
          <>
            <Button variant="plain" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button variant={pending?.action === 'restore' ? 'primary' : 'danger'} onClick={confirm}>
              {pending?.action === 'restore' ? 'Restore' : pending?.action === 'empty' ? 'Empty' : 'Delete'}
            </Button>
          </>
        }
      >
        <p className="text-sm">{pendingBody(pending)}</p>
      </Dialog>
    </div>
  )
}

function StorageLine({
  storage,
  keep,
  onKeep,
  onEmpty,
  disabled,
}: {
  storage: Storage
  keep: number
  onKeep: (n: number) => void
  onEmpty: () => void
  disabled: boolean
}) {
  return (
    <div className="mb-4 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-ink-soft">
      <span>
        {storage.snapshots} {storage.snapshots === 1 ? 'snapshot' : 'snapshots'} ·{' '}
        {bytes(storage.snapshotSize)}
      </span>
      {storage.quarantine > 0 && (
        <>
          <span aria-hidden="true">·</span>
          <span className="text-[#a13028]">{bytes(storage.quarantineSize)} set aside</span>
          <Button size="sm" variant="danger" onClick={onEmpty} disabled={disabled}>
            Empty
          </Button>
        </>
      )}
      <label className="ml-auto flex items-center gap-1.5">
        Keep last
        <input
          type="number"
          min={1}
          max={200}
          value={keep}
          onChange={(e) => onKeep(Math.min(200, Math.max(1, Number(e.target.value) || 1)))}
          aria-label="Snapshots to keep"
          className="w-12 rounded-md border border-[#a9c6da] bg-white px-1 py-0.5 text-center text-[11px] text-ink tabular-nums shadow-[inset_0_2px_3px_rgba(20,50,70,0.12)] focus:border-[#1d7fc4] focus:ring-2 focus:ring-sky/30 focus:outline-none"
        />
      </label>
    </div>
  )
}

function pendingTitle(pending: Pending | null): string {
  if (pending?.action === 'delete') return 'Delete snapshot?'
  if (pending?.action === 'empty') return 'Empty quarantine?'
  return 'Restore snapshot?'
}

function pendingBody(pending: Pending | null): string {
  if (pending?.action === 'delete') return 'This snapshot is removed for good.'
  if (pending?.action === 'empty') {
    return 'Removes the folders your earlier restores set aside. Snapshots are not touched.'
  }
  return 'Your current saves and Tray are moved to a quarantine folder first, not deleted.'
}

function message(e: unknown): string {
  const text = e instanceof Error ? e.message : String(e)
  return text.replace(/^Error:\s*/, '')
}
