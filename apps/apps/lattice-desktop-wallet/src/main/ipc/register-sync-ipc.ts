import {ipcMain} from 'electron';
import {SyncService} from '../services/sync-service';

function registerSyncIpc(ss: SyncService) {
  ipcMain.handle('sync-get-progress', _event => ss.getSyncProgress());
  ipcMain.handle('sync-wait-for-sync', _event => ss.waitForSync());
  ipcMain.handle('sync-is-syncing', _event => ss.isSyncing());
}

export {registerSyncIpc};
