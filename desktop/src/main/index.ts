import { join } from 'node:path'
import { app, BrowserWindow, ipcMain, Menu, shell, type MenuItemConstructorOptions } from 'electron'
import { PtyManager } from './pty-manager'
import { gitBranch } from './git'
import { IPC, type PtyCreateOptions } from '../shared/ipc'

let mainWindow: BrowserWindow | null = null
const ptyManager = new PtyManager()

function appIconPath(): string | undefined {
  if (process.platform === 'darwin') return undefined
  return app.isPackaged
    ? join(process.resourcesPath, 'icon.png')
    : join(__dirname, '../../build/icon.png')
}

// Install a custom application menu so the default Electron menu (which exposes
// View ▸ Reload / Force Reload via Cmd+R / Ctrl+R) is gone in shipped builds. A
// reload is a main-frame, non-same-document navigation, so the
// 'did-start-navigation' handler below would killAll() every pty — silent data
// loss for a "leave it running while you wait" app. Reload/devtools therefore
// exist ONLY in development. Essentials are preserved via built-in roles: the
// macOS app menu (About/Quit = Cmd+Q) and Edit (Undo/Redo/Cut/Copy/Paste/
// Select-All — keeps xterm copy/paste working). The Window submenu is hand-built
// with minimize/zoom only and deliberately OMITS the role's Close item: Cmd+W is
// owned in-app to close the focused terminal tab (not the whole OS window, which
// would killAll() every pty). The red traffic-light button / Cmd+Q still quit.
function buildAppMenu(): void {
  const isDev = !app.isPackaged
  const template: MenuItemConstructorOptions[] = [
    ...(process.platform === 'darwin'
      ? [{ role: 'appMenu' } as MenuItemConstructorOptions]
      : []),
    { role: 'editMenu' },
    { label: 'Window', submenu: [{ role: 'minimize' }, { role: 'zoom' }] },
    ...(isDev
      ? [
          {
            label: 'View',
            submenu: [
              { role: 'reload' },
              { role: 'forceReload' },
              { role: 'toggleDevTools' },
            ],
          } as MenuItemConstructorOptions,
        ]
      : []),
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

function createWindow(): void {
  const icon = appIconPath()

  mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 760,
    minHeight: 480,
    show: false,
    backgroundColor: '#0b0e14',
    ...(icon ? { icon } : {}),
    // Darwin uses the inset traffic lights; nudge them inward so they clear the
    // sidebar 'tetmux' brand text (trafficLightPosition is only meaningful with
    // hiddenInset on darwin). Other platforms keep the standard title bar.
    ...(process.platform === 'darwin'
      ? { titleBarStyle: 'hiddenInset' as const, trafficLightPosition: { x: 16, y: 16 } }
      : { titleBarStyle: 'default' as const }),
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      sandbox: false,
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  mainWindow.on('ready-to-show', () => mainWindow?.show())
  mainWindow.on('closed', () => {
    mainWindow = null
  })

  // A full main-frame document replacement (reload/navigation) or a renderer
  // crash means the current renderer's windows are gone and about to be
  // re-created — kill their ptys so a reload (e.g. Cmd+R in dev) does not orphan
  // the user's running commands. Ignore in-page and sub-frame navigations so we
  // don't kill ptys spuriously. On the very first load the set is empty (no-op).
  mainWindow.webContents.on('did-start-navigation', (details) => {
    if (details.isMainFrame && !details.isSameDocument) ptyManager.killAll()
  })
  mainWindow.webContents.on('render-process-gone', () => ptyManager.killAll())

  // Open target=_blank / external links in the OS browser, never in-app.
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })

  if (process.env['ELECTRON_RENDERER_URL']) {
    mainWindow.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

// Forward pty output/exit to the renderer, guarding against a webContents that
// is mid-teardown (reload/close): node-pty can emit async data after the window
// starts closing, and an unguarded send throws 'Object has been destroyed'.
function sendToRenderer(channel: string, payload: unknown): void {
  const wc = mainWindow?.webContents
  if (!mainWindow || mainWindow.isDestroyed() || !wc || wc.isDestroyed()) return
  try {
    wc.send(channel, payload)
  } catch {
    // webContents can still tear down between the guard above and the send.
  }
}
ptyManager.on('data', (event) => sendToRenderer(IPC.ptyData, event))
ptyManager.on('exit', (event) => sendToRenderer(IPC.ptyExit, event))

ipcMain.handle(IPC.ptyCreate, (_event, opts: PtyCreateOptions) => ptyManager.create(opts))
ipcMain.on(IPC.ptyWrite, (_event, id: number, data: string) => ptyManager.write(id, data))
ipcMain.on(IPC.ptyResize, (_event, id: number, cols: number, rows: number) =>
  ptyManager.resize(id, cols, rows),
)
ipcMain.on(IPC.ptyKill, (_event, id: number) => ptyManager.kill(id))
ipcMain.handle(IPC.gitBranch, (_event, cwd: string) => gitBranch(cwd))

app.whenReady().then(() => {
  buildAppMenu()
  createWindow()
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  ptyManager.killAll()
  if (process.platform !== 'darwin') app.quit()
})

app.on('before-quit', () => ptyManager.killAll())
