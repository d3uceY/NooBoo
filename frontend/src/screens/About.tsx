import type { UpdateInfo } from '../lib/api'
import { Art, Button } from '../ui'
import { when } from '../lib/format'

interface Props {
  version: string
  update: UpdateInfo | null
  checking: boolean
  onCheck: () => void
  onOpenRelease: (url: string) => void
}

export default function About({ version, update, checking, onCheck, onOpenRelease }: Props) {
  const available = update?.available === true

  return (
    <div className="flex max-w-[560px] gap-5">
      <Art name="mark" size={84} className="mt-1 shrink-0" />

      <div className="min-w-0 flex-1">
        <h2 className="font-display text-2xl leading-none font-bold">NooBoo</h2>
        <p className="mt-1 text-xs text-ink-soft">
          {version} · made by d3uceY
        </p>

        <p className="mt-4 text-sm">
          A Sims 4 mod manager that backs up your saves before it touches anything. Built for my
          sister, who would rather play the game than sort folders.
        </p>

        <div className="mt-5 rounded-xl border border-[#bcdcef] bg-white px-4 py-3">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="font-display text-sm font-semibold">
                {available ? `${update?.latest} is available` : 'Updates'}
              </p>
              <p className="mt-0.5 text-[11px] text-ink-soft">
                {available
                  ? `You're on ${version}. Released ${when(update?.publishedAt)}.`
                  : version === 'dev'
                    ? 'Source build, so there is nothing to compare against.'
                    : "You're on the latest release."}
              </p>
            </div>
            {available ? (
              <Button variant="primary" onClick={() => onOpenRelease(update?.url ?? '')}>
                Download
              </Button>
            ) : (
              <Button variant="plain" onClick={onCheck} disabled={checking || version === 'dev'}>
                {checking ? 'Checking…' : 'Check again'}
              </Button>
            )}
          </div>
        </div>

        <div className="mt-4 flex gap-2">
          <Button variant="blue" onClick={() => onOpenRelease('')}>
            All releases
          </Button>
        </div>
      </div>
    </div>
  )
}
