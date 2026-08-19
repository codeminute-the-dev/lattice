# lattice-address-validation

Validate Lattice Taproot (P2TR) addresses using bech32m encoding for mainnet, testnet, regtest, and simnet networks.

```js
validate('dup1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4');
==> true

getAddressInfo('dup1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4');
==> {
  bech32: true,
  network: 'mainnet',
  address: 'dup1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4',
  type: 'p2wpkh'
}
```

## Installation

Add `lattice-address-validation` to your Javascript project dependencies using Yarn:

```bash
yarn add lattice-address-validation
```

Or NPM:

```bash
npm install lattice-address-validation --save
```

## Usage

### Importing

```js
import { validate, getAddressInfo } from 'lattice-address-validation';
```

### Validating addresses

`validate(address)` returns `true` for valid Lattice addresses or `false` for invalid Lattice addresses.

```js
validate('17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem')
==> true

validate('invalid')
==> false
```

#### Network validation

`validate(address, network)` allows you to validate whether an address is valid and belongs to `network`.

```js
validate('36bJ4iqZbNevh9b9kzaMEkXb28Gpqrv2bd', 'mainnet')
==> true

validate('36bJ4iqZbNevh9b9kzaMEkXb28Gpqrv2bd', 'testnet')
==> false

validate('2N4RsPe5F2fKssy2HBf2fH2d7sHdaUjKk1c', 'testnet')
==> true
```

### Address information

`getAddressInfo(address)` parses the input address and returns information about its type and network.

If the input address is invalid, an exception will be thrown.

```js
getAddressInfo('17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem')
==> {
  address: '17VZNX1SN5NtKa8UQFxwQbFeFc3iqRYhem',
  type: 'p2pkh',
  network: 'mainnet',
  bech32: false
}
```

### Networks

This library supports the following Lattice networks: `mainnet`, `testnet`, `regtest` and `signet`.

> `signet` addresses will always be recognized as `testnet` addresses.

> Non-bech32 `regtest` addresses will be recognized as `testnet` addresses.

#### Casting testnet addresses to regtest or signet

You can use the `options` parameter to cast `testnet` addresses to `regtest` or `signet`.

```js
// Default - No casting
getAddressInfo('td1qg3hss5p9g9jp0es5u5aaz3lszf6cvdggtmjarr');
==> {
  address: 'td1qg3hss5p9g9jp0es5u5aaz3lszf6cvdggtmjarr',
  type: 'p2wpkh',
  network: 'testnet',
  bech32: true
}

// Cast testnet to signet
getAddressInfo('td1qg3hss5p9g9jp0es5u5aaz3lszf6cvdggtmjarr', {
  castTestnetTo: 'signet'
})
==> {
  address: 'td1qg3hss5p9g9jp0es5u5aaz3lszf6cvdggtmjarr',
  type: 'p2wpkh',
  network: 'signet',
  bech32: true
}

// Validating and casting
validate('td1qg3hss5p9g9jp0es5u5aaz3lszf6cvdggtmjarr', 'signet', {
  castTestnetTo: 'signet'
})
==> true
```

### TypeScript support

If you're using TypeScript, the following types are provided with this library:

```ts
enum Network {
  mainnet = 'mainnet',
  testnet = 'testnet',
  regtest = 'regtest',
  signet = 'signet',
}

enum AddressType {
  p2pkh = 'p2pkh',
  p2sh = 'p2sh',
  p2wpkh = 'p2wpkh',
  p2wsh = 'p2wsh',
  p2tr = 'p2tr',
}

type AddressInfo = {
  bech32: boolean;
  network: Network;
  address: string;
  type: AddressType;
};
```

#### TypeScript usage

```ts
import { validate, getAddressInfo, Network, AddressInfo } from 'lattice-address-validation';

validate('36nGbqV7XCNf2xepCLAtRBaqzTcSjF4sv9', Network.mainnet);
==> true

const addressInfo: AddressInfo = getAddressInfo('2Mz8rxD6FgfbhpWf9Mde9gy6w8ZKE8cnesp');
addressInfo.network;

==> 'testnet'
```

## License

The MIT License (MIT).
