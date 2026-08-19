HTTP POST Example
=================

This example shows how to use the rpcclient package to connect to an RPC server
using HTTP POST mode with TLS disabled and gets the current block count.

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
$ cd node/rpcclient/examples/bitcoincorehttp
$ go run .
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
