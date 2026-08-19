import { BrowserWindow, ipcMain } from 'electron';
import { UpdateService, UpdateStatus } from '../services/update-service/update-service';

function registerUpdateIpc(service: UpdateService): void {
  ipcMain.handle('update-get-status', () => service.getStatus());
  ipcMain.handle('update-check', () => service.checkForUpdates());
  ipcMain.handle('update-open-release-page', () => service.openReleasePage());

  service.on('status-changed', (status: UpdateStatus) => {
    for (const window of BrowserWindow.getAllWindows()) {
      if (!window.isDestroyed()) {
        window.webContents.send('update-status-changed', status);
      }
    }
  });
}

export { registerUpdateIpc };
