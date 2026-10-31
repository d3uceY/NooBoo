import { useState } from 'react'
import { api } from '../lib/api'
import type { Kind } from '../lib/api'
import { Art, Button, Field, Panel, PanelHeader } from '../ui'

function seedName(path: string): string {
  const base = path.split(/[\\/]/).filter(Boolean).pop() ?? ''
  return base.replace(/\.(package|ts4script|zip)$/i, '')
}

export default function Install({
  onDone,
  seed,
}: {
  onDone: () => Promise<void>
  seed: { path: string; at: number } | null
}) {
  const [source, setSource] = useState(seed?.path ?? '')
  const [name, setName] = useState(seed ? seedName(seed.path) : '')
  const [creator, setCreator] = useState('')
  const [kind, setKind] = useState<Kind>('mod')
  const [backup, setBackup] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState('')

  const adopt = (path: string) => {
    setSource(path)
    setName(seedName(path))
  }

  const pick = async (kindOfPick: 'file' | 'folder') => {
    setError('')
    setDone('')
    try {
      const picked = kindOfPick === 'file' ? await api.pickFile() : await api.pickFolder()
      if (!picked) return
      adopt(picked)
    } catch (e) {
      setError(message(e))
    }
  }

  const install = async () => {
    setBusy(true)
    setError('')
    setDone('')
    try {
      if (backup) await api.backupNow()
      const res = await api.install({ source, name, creator, kind })
      setDone(res.target === 'tray' ? `Added ${res.name} to Tray` : `Installed ${res.name}`)
      setSource('')
      setName('')
      setCreator('')
      await onDone()
    } catch (e) {
      setError(message(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div data-file-drop-target>
      <Panel className="w-[min(560px,100%)]">
        <PanelHeader title="Add a file" hint=".package, .ts4script, .zip, or drop one here" />
        <div className="space-y-4 px-4 py-4">
        <div>
          <span className="mb-1 block text-[11px] font-semibold text-ink-soft">File or folder</span>
          <div className="flex items-center gap-2">
            <Button variant="blue" onClick={() => pick('file')}>
              Pick file
            </Button>
            <Button onClick={() => pick('folder')}>Pick folder</Button>
            <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-ink-soft">
              {source || 'Nothing chosen'}
            </span>
          </div>
        </div>

        <Field label="Name" value={name} onChange={setName} placeholder="Folder name in Mods" />

        <div>
          <span className="mb-1 block text-[11px] font-semibold text-ink-soft">Type</span>
          <div className="flex gap-2">
            <KindButton active={kind === 'mod'} onClick={() => setKind('mod')} art="mod" label="Mod" hint="Mods / name" />
            <KindButton
              active={kind === 'cc'}
              onClick={() => setKind('cc')}
              art="cc"
              label="Custom content"
              hint="Mods / creator / name"
            />
          </div>
        </div>

        {kind === 'cc' && (
          <Field label="Creator" value={creator} onChange={setCreator} placeholder="Creator name" />
        )}

        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={backup}
            onChange={(e) => setBackup(e.target.checked)}
            className="h-4 w-4 accent-plumbob"
          />
          Back up saves and Tray first
        </label>

        {error && <p className="text-xs text-danger">{error}</p>}
        {done && <p className="text-xs text-plumbob-dark">{done}</p>}

        <div className="flex justify-end">
          <Button variant="primary" onClick={install} disabled={busy || !source || !name.trim()}>
            {busy ? 'Installing…' : 'Install'}
          </Button>
        </div>
      </div>
      </Panel>
    </div>
  )
}

function KindButton({
  active,
  onClick,
  art,
  label,
  hint,
}: {
  active: boolean
  onClick: () => void
  art: 'mod' | 'cc'
  label: string
  hint: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`flex flex-1 cursor-pointer items-center gap-2 rounded-xl border px-3 py-2 text-left transition-colors focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none ${
        active
          ? 'border-[#3f7a17] bg-linear-to-b from-[#8ed94f] to-[#57a52a] text-white shadow-[inset_0_1px_0_rgba(255,255,255,0.5),0_2px_5px_rgba(20,50,70,0.2)]'
          : 'border-[#c3d8e6] bg-white hover:border-[#8dbfe0]'
      }`}
    >
      <Art name={art} size={22} />
      <span className="min-w-0">
        <span className="block font-display text-sm font-semibold">{label}</span>
        <span className={`block font-mono text-[10px] ${active ? 'text-white/80' : 'text-ink-soft'}`}>
          {hint}
        </span>
      </span>
    </button>
  )
}

function message(e: unknown): string {
  const text = e instanceof Error ? e.message : String(e)
  return text.replace(/^Error:\s*/, '')
}
