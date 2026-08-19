import { BrowserWindow, shell } from 'electron';
import { join } from 'path';
import { is } from '@electron-toolkit/utils';

function createMainWindow(): BrowserWindow {
  const mainWindow = new BrowserWindow({
    width: 1000,
    height: 900,
    minWidth: 800,
    minHeight: 600,
    maxWidth: 2000,
    maxHeight: 2000,
    resizable: true,
    show: false,
    autoHideMenuBar: true,
    webPreferences: {
      preload: join(__dirname, '../preload/index.mjs'),
      sandbox: false,
      nodeIntegration: false,
      contextIsolation: true,
    },
    useContentSize: false,
  });

  mainWindow.on('ready-to-show', () => {
    mainWindow.show();
  });

  mainWindow.webContents.setWindowOpenHandler(details => {
    shell.openExternal(details.url);
    return { action: 'deny' };
  });

  mainWindow.webContents.on('did-fail-load', (_, errorCode, errorDescription, validatedURL) => {
    console.error('[Main] Renderer failed to load:', errorCode, errorDescription, validatedURL);
  });

  mainWindow.webContents.on('render-process-gone', () => {
    console.error('[Main] Renderer process crashed');
  });

  if (is.dev && process.env['ELECTRON_RENDERER_URL']) {
    mainWindow.loadURL(process.env['ELECTRON_RENDERER_URL']);
  } else {
    const htmlPath = join(__dirname, '../renderer/index.html');
    mainWindow.loadFile(htmlPath);
  }

  return mainWindow;
}

export { createMainWindow };
