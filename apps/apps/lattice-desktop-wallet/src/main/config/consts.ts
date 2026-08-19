// Legacy hardcoded peer hosts from older wallet versions. Kept only so stale
// peer-settings.json entries (saved as custom --addpeer targets) are detected
// and ignored, falling back to DNS-seeder-based discovery.
export const LEGACY_MAINNET_PEER_ADDRESSES = [
  'wallet-node0.lattice.codeminute.dev',
  'wallet-node1.lattice.codeminute.dev',
  'wallet-node2.lattice.codeminute.dev',
  'wallet-node3.lattice.codeminute.dev',
  'wallet-node4.lattice.codeminute.dev',
];
export const LEGACY_TESTNET_PEER_ADDRESSES = [
  'node1.testnet.lattice.codeminute.dev',
  'node2.testnet.lattice.codeminute.dev',
  'node3.testnet.lattice.codeminute.dev',
];

export const UPDATE_REPO_OWNER = 'codeminute';
export const UPDATE_REPO_NAME = 'lattice';
export const UPDATE_RELEASE_TAG_PREFIX = 'lattice-wallet-v';
