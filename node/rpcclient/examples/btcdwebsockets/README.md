latticed Websockets Example
=========================

This example shows how to use the rpcclient package to connect to a latticed RPC
server using TLS-secured websockets, register for block connected and block
disconnected notifications, and get the current block count.

This example also sets a timer to shutdown the client after 10 seconds to
demonstrate clean shutdown.

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

Finally, navigate to the example's directory and run it with:

```bash
$ cd node/rpcclient/examples/btcdwebsockets
$ go run .
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
