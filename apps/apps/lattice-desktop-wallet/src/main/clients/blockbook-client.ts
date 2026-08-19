import { getCurrentNetwork, type Network } from '../config/network-config';

const BlockbookBaseUrlMap: Record<Network, string> = {
    testnet: 'http://blockbook.testnet.lattice.codeminute.dev',
    mainnet: 'http://blockbook.lattice.codeminute.dev',
};

function getBaseUrl(): string {
    return BlockbookBaseUrlMap[getCurrentNetwork()];
}

export const BlockbookClient = {
    async estimateFee(numBlocks: number) {
        const response = await fetch(`${getBaseUrl()}/api/v1/estimatefee/${numBlocks}`);
        const data = await response.json();
        return data.result;
    },
};
