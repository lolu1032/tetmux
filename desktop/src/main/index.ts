import { join } from 'node:path'
import { app, BrowserWindow, ipcMain, shell } from 'electron'
import { PtyManager } from './pty-manager'
import { IPC, type PtyCreateOptions } from '../shared/ipc'

let mainWindow: BrowserWindow | null = null
const ptyManager = new PtyManager()

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 760,
    minHeight: 480,
    show: false,
    backgroundColor: '#0b0e14',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
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

// Forward pty output/exit to whichever window is alive.
ptyManager.on('data', (event) => mainWindow?.webContents.send(IPC.ptyData, event))
ptyManager.on('exit', (event) => mainWindow?.webContents.send(IPC.ptyExit, event))

ipcMain.handle(IPC.ptyCreate, (_event, opts: PtyCreateOptions) => ptyManager.create(opts))
ipcMain.on(IPC.ptyWrite, (_event, id: number, data: string) => ptyManager.write(id, data))
ipcMain.on(IPC.ptyResize, (_event, id: number, cols: number, rows: number) =>
  ptyManager.resize(id, cols, rows),
)
ipcMain.on(IPC.ptyKill, (_event, id: number) => ptyManager.kill(id))

app.whenReady().then(() => {
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
