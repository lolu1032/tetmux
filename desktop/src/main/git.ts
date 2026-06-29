import { execFile } from 'node:child_process'

// The cwd comes from a PTY-controlled OSC 7 escape, so strip GIT_* from the
// inherited environment — they could redirect git away from `-C <cwd>` (e.g.
// GIT_DIR) or otherwise change behaviour.
function gitSafeEnv(): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (!key.startsWith('GIT_')) env[key] = value
  }
  return env
}

/**
 * Resolve the current git branch for a working directory. Returns null if the
 * path is not a git repo (or git is missing / detached HEAD). Uses execFile (no
 * shell) and a short timeout so a slow/networked path cannot hang the UI.
 */
export function gitBranch(cwd: unknown): Promise<string | null> {
  // The cwd arrives from the renderer (an OSC 7 escape the shell emitted), so
  // validate it before handing it to git.
  if (typeof cwd !== 'string' || !cwd.startsWith('/') || cwd.length > 4096) {
    return Promise.resolve(null)
  }
  return new Promise((resolve) => {
    execFile(
      'git',
      ['-C', cwd, 'rev-parse', '--abbrev-ref', 'HEAD'],
      { timeout: 1500, windowsHide: true, maxBuffer: 64 * 1024, env: gitSafeEnv() },
      (err, stdout) => {
        if (err) return resolve(null)
        const branch = stdout.trim()
        resolve(branch && branch !== 'HEAD' ? branch : null)
      },
    )
  })
}
