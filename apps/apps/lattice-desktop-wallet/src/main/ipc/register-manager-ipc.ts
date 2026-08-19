import { ipcMain } from 'electron';
import { ManagerService } from '../services/manager-service';

function registerManagerIpc(ms: ManagerService) {
  ipcMain.handle('get-wallets-stats', () => ms.getWalletsStats());
  ipcMain.handle('select-wallet', (_, walletName) => ms.selectWallet(walletName));
  ipcMain.handle('wallet-create', (_, options) => ms.create(options));
  ipcMain.handle('wallet-import', (_, options) => ms.import(options));
  ipcMain.handle('get-existing-wallets', () => ms.getExistingWallets());

  // Network management
  ipcMain.handle('get-network-info', () => ms.getNetworkInfo());
  ipcMain.handle('set-network', (_, network) => ms.setNetwork(network));

  // Peer settings management
  ipcMain.handle('get-peer-settings', () => ms.getPeerSettings());
  ipcMain.handle('validate-peer', (_, address, port) => ms.validatePeerAddress(address, port));
  ipcMain.handle('set-custom-peer', (_, address, port) => ms.setCustomPeerAddress(address, port));
  ipcMain.handle('reset-peer-to-default', () => ms.resetPeerToDefault());
}

export { registerManagerIpc };
