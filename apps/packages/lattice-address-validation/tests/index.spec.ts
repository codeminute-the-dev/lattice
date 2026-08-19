import validate, { getAddressInfo, Network } from '../src/index';
import { expect, describe, it } from 'vitest';

describe('Taproot Address Validation', () => {
  describe('Valid Taproot Addresses', () => {
    it('validates Mainnet P2TR', () => {
      const address = 'dup1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnqnqzpxm';

      expect(validate(address)).toBe(true);
      expect(getAddressInfo(address)).toEqual({ bech32: true, type: 'p2tr', network: 'mainnet', address });
    });

    it('validates Testnet P2TR', () => {
      const address = 'td1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnql32xdy';

      expect(validate(address)).toBe(true);
      expect(getAddressInfo(address)).toEqual({ bech32: true, type: 'p2tr', network: 'testnet', address });
    });

    it('validates Regtest P2TR', () => {
      const address = 'duprt1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnqevu7ll';

      expect(validate(address)).toBe(true);
      expect(getAddressInfo(address)).toEqual({ bech32: true, type: 'p2tr', network: 'regtest', address });
    });

    it('validates Simnet P2TR', () => {
      const address = 'sd1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnqcqqp5p';

      expect(validate(address)).toBe(true);
      expect(getAddressInfo(address)).toEqual({ bech32: true, type: 'p2tr', network: 'simnet', address });
    });
  });

  describe('Validation with Network Parameter', () => {
    it('validates Mainnet P2TR with network parameter', () => {
      const address = 'dup1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnqnqzpxm';
      expect(validate(address, Network.mainnet)).toBe(true);
    });

    it('validates Testnet P2TR with network parameter', () => {
      const address = 'td1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnql32xdy';
      expect(validate(address, Network.testnet)).toBe(true);
    });

    it('validates Regtest P2TR with network parameter', () => {
      const address = 'duprt1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnqevu7ll';
      expect(validate(address, Network.regtest)).toBe(true);
    });

    it('validates Simnet P2TR with network parameter', () => {
      const address = 'sd1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnqcqqp5p';
      expect(validate(address, Network.simnet)).toBe(true);
    });

    it('rejects mainnet address when validating against testnet', () => {
      const address = 'dup1paardr2nczq0rx5rqpfwnvpzm497zvux64y0f7wjgcs7xuuuh2nnqnqzpxm';
      expect(validate(address, Network.testnet)).toBe(false);
    });
  });

  describe('Invalid/Rejected Addresses', () => {
    it('rejects Legacy SegWit v0 P2WPKH addresses', () => {
      const addresses = [
        'dup1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4', // mainnet
        'td1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx', // testnet
        'duprt1q6z64a43mjgkcq0ul2znwneq3spghrlau9slefp', // regtest
        'sd1qw508d6qejxtdg4y5r3zarvary0c5xw7kwahwc4', // simnet
      ];

      addresses.forEach((address) => {
        expect(validate(address)).toBe(false);
      });
    });

    it('rejects Legacy SegWit v0 P2WSH addresses', () => {
      const addresses = [
        'dup1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3qccfmv3', // mainnet
        'td1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7', // testnet
        'duprt1q5n2k3frgpxces3dsw4qfpqk4kksv0cz96pldxdwxrrw0d5ud5hcqzzx7zt', // regtest
      ];

      addresses.forEach((address) => {
        expect(validate(address)).toBe(false);
      });
    });

    it('rejects invalid bech32 address', () => {
      const address = 'dup1qw508d6qejxtdg4y5r3zrrvary0c5xw7kv8f3t4'; // invalid checksum
      expect(validate(address)).toBe(false);
    });

    it('rejects bogus addresses', () => {
      expect(validate('x')).toBe(false);
      expect(validate('invalid')).toBe(false);
      expect(validate('')).toBe(false);
    });

    it('rejects addresses with wrong witness version', () => {
      // Address with witness version 2 (not v1/Taproot)
      expect(validate('dup1z...someaddress')).toBe(false);
    });
  });

  describe('Case Sensitivity', () => {
    it('validates uppercase Taproot addresses', () => {
      const address = 'DUP1PAARDR2NCZQ0RX5RQPFWNVPZM497ZVUX64Y0F7WJGCS7XUUUH2NNQNQZPXM';
      expect(validate(address)).toBe(true);
    });
  });

  describe('Error Messages', () => {
    it('throws error for P2WPKH addresses', () => {
      const address = 'dup1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4';

      // The error message is caught and re-thrown as "Invalid address" by getAddressInfo
      expect(() => getAddressInfo(address)).toThrow('Invalid address');
    });

    it('throws descriptive error for invalid address', () => {
      const address = 'invalid';

      expect(() => getAddressInfo(address)).toThrow('Invalid address');
    });
  });
});
