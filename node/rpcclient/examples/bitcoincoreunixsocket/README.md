Unix Socket Example
===================

This example shows how to use the rpcclient package to connect to an RPC server
using HTTP POST mode over a Unix Socket with TLS disabled and gets the current
block count.

## Running the Example

The first step is to ensure the module is available. From the project root:

```bash
$ go mod download
```

Next, modify the `main.go` source to specify the correct RPC username and
password for the RPC server:

```Go
	User: "yourrpcuser",
	Pass: "yourrpcpass",
```

If the RPC server only supports TCP/IP, you can redirect requests from the
Unix Socket using the `socat` command:

```bash
$ socat -d UNIX-LISTEN:"my-unix-socket-path",fork TCP:"host-address"
$ socat -d UNIX-LISTEN:/tmp/test.XXXX,fork TCP:localhost:44207
```

Finally, navigate to the example's directory and run it with:

```bash
$ cd node/rpcclient/examples/bitcoincoreunixsocket
$ go run .
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
