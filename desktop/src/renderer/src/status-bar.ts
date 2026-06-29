import type { Focus } from './types'
import type { GameStatus } from './tetris/engine'

export interface StatusData {
  winLabel: string
  winCount: number
  attention: number
  focus: Focus
  score: number
  best: number
  level: number
  gameStatus: GameStatus | 'ready'
  hint: string
}

/** A stable key so the caller can skip a rebuild when nothing changed. */
export function statusKey(d: StatusData): string {
  return [d.winLabel, d.winCount, d.attention, d.focus, d.score, d.best, d.level, d.gameStatus, d.hint].join(
    '|',
  )
}

function seg(cls: string, text: string): HTMLSpanElement {
  const el = document.createElement('span')
  el.className = `seg ${cls}`
  // textContent, never innerHTML: winLabel embeds the PTY-controlled terminal
  // title, which must never be parsed as markup (XSS sink otherwise).
  el.textContent = text
  return el
}

/** Rebuild the status bar from `data`, escaping every value via textContent. */
export function renderStatus(container: HTMLElement, d: StatusData): void {
  const focusLabel = d.focus === 'tetris' ? 'TETRIS' : 'TERMINAL'
  const segments = [
    seg('win', `win ${d.winLabel} (${d.winCount})`),
    seg(`focus focus-${d.focus}`, `focus:${focusLabel}`),
    seg('score', `score ${d.score} · best ${d.best} · lv ${d.level}`),
    seg(`game game-${d.gameStatus}`, d.gameStatus),
    seg('hint', d.hint),
  ]
  if (d.attention > 0) {
    // Inserted right after the window segment so it reads "win 1:zsh (3) ● 2 waiting".
    segments.splice(1, 0, seg('attention', `● ${d.attention} waiting`))
  }
  container.replaceChildren(...segments)
}
