import { describe, expect, it } from 'vitest'
import { WindowActivity } from './window-activity'

describe('WindowActivity', () => {
  it('starts idle', () => {
    const a = new WindowActivity()
    expect(a.state).toBe('idle')
    expect(a.needsAttention).toBe(false)
  })

  it('marks unread on background output, not on active output', () => {
    const a = new WindowActivity()
    expect(a.onOutput(true, 'hello')).toBe(false) // active window: on screen
    expect(a.state).toBe('idle')
    expect(a.onOutput(false, 'hello')).toBe(true) // background: unread
    expect(a.state).toBe('unread')
    expect(a.onOutput(false, 'more')).toBe(false) // already unread: no change
  })

  it('escalates to bell on a background BEL byte', () => {
    const a = new WindowActivity()
    a.onOutput(false, 'work\x07done')
    expect(a.state).toBe('bell')
    expect(a.needsAttention).toBe(true)
  })

  it('clears unread/bell when selected, but keeps exited', () => {
    const a = new WindowActivity()
    a.onOutput(false, 'x\x07')
    expect(a.state).toBe('bell')
    expect(a.onSelect()).toBe(true)
    expect(a.state).toBe('idle')
    expect(a.onSelect()).toBe(false) // nothing to clear
  })

  it('exit is sticky and outranks unread/bell', () => {
    const a = new WindowActivity()
    a.onOutput(false, 'x')
    expect(a.onExit()).toBe(true)
    expect(a.state).toBe('exited')
    expect(a.onExit()).toBe(false)
    // further output on a dead window changes nothing
    expect(a.onOutput(false, 'zombie\x07')).toBe(false)
    // selecting an exited window does not un-exit it
    a.onSelect()
    expect(a.state).toBe('exited')
    expect(a.needsAttention).toBe(true)
  })
})
