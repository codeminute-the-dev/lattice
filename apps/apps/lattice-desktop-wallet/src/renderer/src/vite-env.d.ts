/// <reference types="vite/client" />

import {AppBridge} from '../../types/app-bridge';

declare global {
  interface Window {
    appBridge: AppBridge;
  }
}

export {};
