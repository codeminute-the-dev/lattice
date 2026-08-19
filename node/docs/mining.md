# Mining

latticed supports the `getblocktemplate` RPC.
The limited user cannot access this RPC.

## Add the payment addresses with the `miningaddr` option

Lattice requires a Taproot address for mining rewards. Generate one with:

```bash
./bin/latctl -u rpcuser -P rpcpass -s https://localhost:44207 getnewaddress
```

Then add it to your config:

```bash
[Application Options]
rpcuser=myuser
rpcpass=SomeDecentp4ssw0rd
miningaddr=<your-taproot-address>
```

## Add latticed's RPC TLS certificate to system Certificate Authority list

Your mining software uses [curl](http://curl.haxx.se/) to fetch data from the RPC server.
Since curl validates the certificate by default, we must install the `latticed` RPC
certificate into the default system Certificate Authority list.

## Ubuntu

1. Copy rpc.cert to /usr/share/ca-certificates: `cp /home/user/.latticed/rpc.cert /usr/share/ca-certificates/latticed.crt`
2. Add latticed.crt to /etc/ca-certificates.conf: `echo latticed.crt >> /etc/ca-certificates.conf`
3. Update the CA certificate list: `update-ca-certificates`

## Set your mining software url to use https

```bash
<your-mining-software> -o https://127.0.0.1:44107 -u rpcuser -p rpcpassword
```
