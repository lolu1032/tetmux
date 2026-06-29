// electron-builder afterPack hook: strip the Chromium UI-locale files we don't
// ship. Chromium bundles ~220 `.lproj` translations of its OWN chrome (context
// menus, the crash page, etc.) — tetmux shows none of that, so all but English
// and Korean are dead weight (~46 MB uncompressed). Removing them here is
// deterministic and also catches the gender-variant locales that the
// `electronLanguages` option misses.
const fs = require('node:fs')
const path = require('node:path')

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
}
