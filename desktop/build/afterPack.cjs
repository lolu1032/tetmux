// electron-builder afterPack hook. Two jobs:
//
// 1. Strip the Chromium UI-locale files we don't ship. Chromium bundles ~220
//    `.lproj` translations of its OWN chrome (context menus, the crash page,
//    etc.) — tetmux shows none of that, so all but English and Korean are dead
//    weight (~46 MB). `electronLanguages` already removes most; this catches any
//    gender-variant locales it leaves behind.
//
// 2. Re-sign the app (ad-hoc) AFTER the strip. Removing files from the already-
//    signed Electron Framework invalidates its signature, and electron-builder
//    skips re-signing when there's no Developer ID — which ships a BROKEN
//    signature that macOS (esp. Apple Silicon) rejects as "damaged". A valid
//    ad-hoc signature turns that hard wall into the normal unsigned-app prompt
//    (right-click → Open / `xattr -cr`). Proper notarization is still the real fix.
const fs = require('node:fs')
const path = require('node:path')
const { execFileSync } = require('node:child_process')

const KEEP = new Set(['en', 'en-US', 'ko'])

function dirSize(dir) {
  let total = 0
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    total += e.isDirectory() ? dirSize(p) : fs.statSync(p).size || 0
  }
  return total
}

// Collect `*.lproj` dirs under the Electron Framework that aren't in KEEP.
function collect(dir, out) {
  let entries
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true })
  } catch {
    return
  }
  for (const e of entries) {
    if (!e.isDirectory()) continue
    const p = path.join(dir, e.name)
    if (e.name.endsWith('.lproj')) {
      // ".lproj" is 6 chars; only touch the engine's locales, never app ones.
      if (p.includes('Electron Framework.framework') && !KEEP.has(e.name.slice(0, -6))) {
        out.push(p)
      }
      continue // never descend into a locale dir
    }
    collect(p, out)
  }
}

exports.default = async function afterPack(context) {
  // ---- 1. strip locales ----
  const targets = []
  collect(context.appOutDir, targets)
  let bytes = 0
  let removed = 0
  for (const t of targets) {
    try {
      bytes += dirSize(t)
      fs.rmSync(t, { recursive: true, force: true })
      removed++
    } catch (err) {
      console.warn('[afterPack] could not remove', t, err.message)
    }
  }
  console.log(
    `[afterPack] stripped ${removed} unused Chromium locales (~${(bytes / 1048576).toFixed(1)} MB freed)`,
  )

  // ---- 2. repair the signature (macOS only) ----
  // Must be the LAST modification to the bundle, so do it here (electron-builder
  // signs after afterPack but skips without an identity, leaving ours intact).
  if (process.platform !== 'darwin') return
  for (const name of fs.readdirSync(context.appOutDir)) {
    if (!name.endsWith('.app')) continue
    const appPath = path.join(context.appOutDir, name)
    try {
      execFileSync('codesign', ['--force', '--deep', '--sign', '-', appPath], { stdio: 'pipe' })
      execFileSync('codesign', ['--verify', '--deep', '--strict', appPath], { stdio: 'pipe' })
      console.log(`[afterPack] ad-hoc re-signed ${name} (valid signature restored)`)
    } catch (err) {
      console.warn(`[afterPack] ad-hoc re-sign of ${name} failed:`, err.message)
    }
  }
}
