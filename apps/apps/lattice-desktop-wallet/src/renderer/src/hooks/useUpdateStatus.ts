import { useEffect, useState } from 'react';
import type { UpdateStatus } from '../../../types/app-bridge';

let cachedStatus: UpdateStatus | null = null;

export function useUpdateStatus(): UpdateStatus | null {
  const [status, setStatus] = useState<UpdateStatus | null>(cachedStatus);

  useEffect(() => {
    let cancelled = false;

    window.appBridge.update
      .getStatus()
      .then(result => {
        if (cancelled) return;
        cachedStatus = result;
        setStatus(result);
      })
      .catch(err => {
        console.error('[useUpdateStatus] Failed to load update status:', err);
      });

    const unsubscribe = window.appBridge.update.onStatusChanged(next => {
      cachedStatus = next;
      setStatus(next);
    });

    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, []);

  return status;
}

export async function triggerUpdateCheck(): Promise<void> {
  try {
    await window.appBridge.update.checkForUpdates();
  } catch (err) {
    console.error('[useUpdateStatus] Manual update check failed:', err);
  }
}

export async function openReleasePage(): Promise<void> {
  try {
    await window.appBridge.update.openReleasePage();
  } catch (err) {
    console.error('[useUpdateStatus] Failed to open release page:', err);
  }
}
