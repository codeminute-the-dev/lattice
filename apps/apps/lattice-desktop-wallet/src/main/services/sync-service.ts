import { ManagerService } from './manager-service';
import type { SyncProgress } from '../../types/app-bridge';

class SyncService {
  constructor(private readonly managerService: ManagerService) { }

  async isSyncing() {
    const walletService = this.managerService.ensureWalletService();
    try {
      const progress = await walletService.getSyncProgress();

      return {
        syncing: !progress.synced,
        walletHeight: progress.blockHeight,
        networkHeight: progress.bestPeerHeight,
      };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error('[SyncService] Failed to check sync status:', errorMessage);
      return { syncing: true, error: errorMessage };
    }
  }

  async getSyncProgress(): Promise<SyncProgress> {
    return this.managerService.ensureWalletService().getSyncProgress();
  }

  async waitForSync() {
    const pollInterval = 3000;
    while (true) {
      try {
        const syncStatus = await this.isSyncing();

        if (syncStatus.error) {
          console.log('[SyncService] Error checking sync status, retrying...', syncStatus.error);
        } else if (!syncStatus.syncing) {
          const walletService = this.managerService.ensureWalletService();
          const balance = await walletService.getBalance('*', 0);

          console.log(
            `[SyncService] Chain fully synced! Height: ${syncStatus.walletHeight}, Balance: ${balance} BTC`
          );
          return { success: true as const, synced: true };
        } else {
          console.log(`[SyncService] Chain still syncing... Height: ${syncStatus.walletHeight}`);
        }
      } catch (error) {
        console.log('[SyncService] Error during sync check, retrying...', error);
      }

      await new Promise(resolve => setTimeout(resolve, pollInterval));
    }
  }
}

export { SyncService };
