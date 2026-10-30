import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from './lib/api'
import type { GameFolder, Mod, Progress, UpdateInfo } from './lib/api'
import { revealScreen } from './lib/motion'
import About from './screens/About'
import Backups from './screens/Backups'
import Install from './screens/Install'
import Library from './screens/Library'
import Troubleshoot from './screens/Troubleshoot'
import { Art, Button, Dialog, ProgressBar } from './ui'
import type { ArtName } from './ui'

type Screen = 'library' | 'install' | 'backups' | 'troubleshoot' | 'about'

const nav: { id: Screen; label: string; art: ArtName; title: string; hint: string }[] = [
  { id: 'library', label: 'Library', art: 'library', title: 'Library', hint: 'Everything in your Mods folder' },
  { id: 'install', label: 'Install', art: 'install', title: 'Install', hint: 'Add a downloaded mod or CC file' },
  { id: 'backups', label: 'Backups', art: 'backups', title: 'Backups', hint: 'Saves and Tray snapshots' },
  { id: 'troubleshoot', label: 'Fix', art: 'fix', title: 'Troubleshoot', hint: 'Switch mods off to find the broken one' },
  { id: 'about', label: 'About', art: 'mark', title: 'About', hint: 'Version, credits and updates' },
]

const busyPhases = new Set(['Backing up', 'Restoring'])

export default function App() {
  const [folder, setFolder] = useState<GameFolder | null>(null)
  const [mods, setMods] = useState<Mod[]>([])
  const [screen, setScreen] = useState<Screen>('library')
  const [progress, setProgress] = useState<Progress | null>(null)
  const [error, setError] = useState('')
  const [version, setVersion] = useState('')
  const [update, setUpdate] = useState<UpdateInfo | null>(null)
  const [checking, setChecking] = useState(false)
  const [updateOpen, setUpdateOpen] = useState(false)
  const askedForUpdate = useRef(false)
  const panel = useRef<HTMLElement>(null)

  const refresh = useCallback(async () => {
    try {
      let current = await api.current()
      if (!current.root) {
        // First run: adopt the standard Documents/Electronic Arts/The Sims 4 folder.
        const detected = await api.detectGameRoot()
        if (detected) current = await api.setGameRoot(detected)
      }
      setFolder(current)
      setMods(current.valid ? ((await api.scan()) ?? []) : [])
    } catch (e) {
      setError(message(e))
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => api.onProgress(setProgress), [])

  useEffect(() => {
    void api
      .about()
      .then((info) => setVersion(info.version))
      .catch(() => undefined)
  }, [])

  const checkUpdate = useCallback(async (announce: boolean) => {
    setChecking(true)
    try {
      const info = await api.checkUpdate()
      setUpdate(info)
      if (announce && info.available) setUpdateOpen(true)
    } catch {
      // A failed check is not worth a message; the About screen still works.
    } finally {
      setChecking(false)
    }
  }, [])

  useEffect(() => {
    if (askedForUpdate.current || !version) return
    askedForUpdate.current = true
    void checkUpdate(true)
  }, [version, checkUpdate])

  useEffect(() => {
    revealScreen(panel.current)
  }, [screen, folder?.valid])

  const pickFolder = async () => {
    setError('')
    try {
      const picked = await api.pickGameFolder()
      if (!picked.valid) {
        setError(picked.problem || 'Not a Sims 4 folder')
        return
      }
      setFolder(picked)
      await refresh()
    } catch (e) {
      setError(message(e))
    }
  }

  const toggle = async (ids: string[], enabled: boolean) => {
    setError('')
    try {
      await api.setEnabled(ids, enabled)
      await refresh()
    } catch (e) {
      setError(message(e))
    }
  }

  const removeMod = async (id: string) => {
    setError('')
    try {
      await api.deleteMod(id)
      await refresh()
    } catch (e) {
      setError(message(e))
    }
  }

  const ready = folder?.valid === true
  const current = nav.find((n) => n.id === screen) ?? nav[0]
  const enabledCount = mods.filter((m) => m.enabled).length

  return (
    <div className="flex h-full">
      <nav className="flex w-23 shrink-0 flex-col items-center gap-1 border-r border-white/80 bg-white/55 py-4 backdrop-blur">
        <div className="plumbob-spin mb-4">
          <Art name="mark" size={36} />
        </div>
        {nav.map(({ id, label, art }) => {
          const active = screen === id
          return (
            <button
              key={id}
              type="button"
              onClick={() => setScreen(id)}
              aria-current={active}
              className={`flex w-18.5 cursor-pointer flex-col items-center gap-1 rounded-xl px-1 py-2 text-[11px] font-bold transition-colors focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none ${
                active
                  ? 'bg-linear-to-b from-[#8ed94f] to-[#57a52a] text-white shadow-[inset_0_1px_0_rgba(255,255,255,0.5),0_2px_5px_rgba(20,50,70,0.2)]'
                  : 'text-ink-soft hover:bg-white/80'
              }`}
            >
              <Art name={art} size={24} className={active ? '' : 'opacity-80'} />
              {label}
            </button>
          )
        })}
      </nav>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="px-6 pt-5 pb-3">
          <h1 className="font-display text-[26px] leading-none font-bold">{current.title}</h1>
          <p className="mt-1 text-xs text-ink-soft">
            {ready ? current.hint : 'Pick your Sims 4 folder to start'}
          </p>
        </header>

        {error && (
          <div className="mx-6 mb-2 rounded-lg border border-danger/40 bg-white/85 px-3 py-2 text-xs text-danger">
            {error}
          </div>
        )}

        <main ref={panel} className="scroll-soft flex-1 overflow-y-auto px-6 pb-4">
          {folder === null ? (
            <p className="mt-16 text-center text-sm text-ink-soft">Loading…</p>
          ) : !ready ? (
            <Setup folder={folder} onPick={pickFolder} />
          ) : screen === 'library' ? (
            <Library
              mods={mods}
              onToggle={toggle}
              onDelete={removeMod}
              onReveal={(p) => void api.reveal(p)}
            />
          ) : screen === 'install' ? (
            <Install onDone={refresh} />
          ) : screen === 'backups' ? (
            <Backups />
          ) : screen === 'about' ? (
            <About
              version={version}
              update={update}
              checking={checking}
              onCheck={() => void checkUpdate(false)}
              onOpenRelease={(url) => void api.openRelease(url)}
            />
          ) : (
            <Troubleshoot mods={mods} onToggle={toggle} />
          )}
        </main>

        <footer className="flex h-9 shrink-0 items-center gap-2 border-t border-white/80 bg-white/45 px-6 text-[11px] text-ink-soft">
          <span className="min-w-0 truncate" title={folder?.root || undefined}>
            {ready ? folder?.root : 'No folder set'}
          </span>
          <button
            type="button"
            onClick={pickFolder}
            className="shrink-0 cursor-pointer font-semibold text-[#1d7fc4] underline decoration-dotted underline-offset-2 transition-colors hover:text-ink focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
          >
            {ready ? 'Change' : 'Choose folder'}
          </button>
          {ready && (
            <span className="ml-auto shrink-0 tabular-nums">
              {enabledCount} on / {mods.length} mods
            </span>
          )}
          {version && (
            <button
              type="button"
              onClick={() => void api.openRepo()}
              title="Releases on GitHub"
              className="shrink-0 cursor-pointer font-semibold text-[#1d7fc4] underline decoration-dotted underline-offset-2 transition-colors hover:text-ink focus-visible:ring-2 focus-visible:ring-sky/60 focus-visible:outline-none"
            >
              {version}
            </button>
          )}
        </footer>
      </div>

      <Dialog
        open={progress !== null && busyPhases.has(progress.phase)}
        onClose={() => undefined}
        title={progress?.phase === 'Restoring' ? 'Restoring' : 'Backing up'}
      >
        <ProgressBar
          value={percent(progress)}
          label={`${progress?.label || 'Working'} · ${progress?.done ?? 0}/${progress?.total ?? 0} files`}
        />
      </Dialog>

      <Dialog
        open={updateOpen}
        onClose={() => setUpdateOpen(false)}
        title="Update available"
        footer={
          <>
            <Button variant="plain" onClick={() => setUpdateOpen(false)}>
              Later
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                void api.openRelease(update?.url ?? '')
                setUpdateOpen(false)
              }}
            >
              Download
            </Button>
          </>
        }
      >
        <p className="text-sm">
          NooBoo <span className="font-semibold">{update?.latest}</span> is out. You&apos;re on{' '}
          {version}.
        </p>
      </Dialog>
    </div>
  )
}

/** First run: show what was auto-detected, then let the player confirm. */
function Setup({ folder, onPick }: { folder: GameFolder; onPick: () => void }) {
  return (
    <div className="mx-auto mt-10 w-[min(540px,100%)] overflow-hidden rounded-2xl border border-[#bcdcef] bg-panel shadow-[0_16px_38px_-26px_rgba(23,60,90,0.55)]">
      <div className="border-b border-[#145c92]/40 bg-linear-to-b from-[#5cbdf3] to-[#1d7fc4] px-4 py-3">
        <h2 className="font-display text-base font-bold text-white drop-shadow-[0_1px_1px_rgba(10,50,80,0.55)]">
          Where is The Sims 4?
        </h2>
      </div>
      <div className="px-4 py-4">
        <p className="mb-3 text-sm text-ink-soft">
          Normally in Documents / Electronic Arts / The Sims 4. The folder holding saves, Tray and Mods.
        </p>
        <p className="mb-3 rounded-lg bg-panel-dim px-3 py-2 font-mono text-[11px] break-all text-ink">
          {folder.root || 'Nothing detected'}
        </p>
        <p className="mb-4 text-xs text-danger">{folder.problem}</p>
        <Button variant="primary" onClick={onPick}>
          Choose folder
        </Button>
      </div>
    </div>
  )
}

function percent(p: Progress | null): number {
  if (!p || p.total <= 0) return 0
  return Math.min(100, (p.done / p.total) * 100)
}

function message(e: unknown): string {
  const text = e instanceof Error ? e.message : String(e)
  return text.replace(/^Error:\s*/, '')
}
