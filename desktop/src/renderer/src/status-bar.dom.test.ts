import { describe, expect, it } from 'vitest'
import { renderStatus } from './status-bar'

describe('renderStatus (XSS safety)', () => {
  it('renders a PTY-controlled window title as inert text, never markup', () => {
    const bar = document.createElement('div')
    const evil = '<img src=x onerror=alert(1)><script>alert(2)</script>'
    renderStatus(bar, {
      winLabel: `1:${evil}`,
      winCount: 1,
      attention: 0,
      focus: 'terminal',
      score: 0,
      best: 0,
      level: 0,
      gameStatus: 'ready',
      hint: 'hint',
    })
    // The malicious title must NOT be parsed into DOM nodes…
    expect(bar.querySelector('img')).toBeNull()
    expect(bar.querySelector('script')).toBeNull()
    // …it must survive as literal text inside a span.
    expect(bar.textContent).toContain(evil)
    expect(bar.querySelectorAll('span.seg').length).toBe(5)
  })
})
