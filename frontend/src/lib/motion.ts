import gsap from 'gsap'

const EASE = 'expo.out'

// Checked once per call; a mid-session OS setting change is not worth a listener.
const reduced = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

// fromTo with explicit end values, not from(): StrictMode runs effects twice, and a
// second from() would capture the already-hidden state as its target and stay hidden.
/** Stagger list rows in when a screen mounts. */
export function revealRows(scope: HTMLElement | null) {
  if (!scope || reduced()) return
  const rows = scope.querySelectorAll('[data-row]')
  if (!rows.length) return
  gsap.fromTo(
    rows,
    { y: 10, autoAlpha: 0 },
    { y: 0, autoAlpha: 1, duration: 0.34, stagger: 0.028, ease: EASE, overwrite: true },
  )
}

/** Slide the active screen up into place. */
export function revealScreen(scope: HTMLElement | null) {
  if (!scope || reduced()) return
  gsap.fromTo(
    scope,
    { y: 14, autoAlpha: 0 },
    { y: 0, autoAlpha: 1, duration: 0.3, ease: EASE, overwrite: true },
  )
}

/** Small pop for a value that just changed. */
export function pop(el: HTMLElement | null) {
  if (!el || reduced()) return
  gsap.fromTo(el, { scale: 0.97 }, { scale: 1, duration: 0.24, ease: EASE, overwrite: true })
}
