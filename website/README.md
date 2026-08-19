# lattice.codeminute.dev

The public site. A self-contained `index.html` — no build step, no bundler, no
dependencies — plus `favicon.svg`, `robots.txt`, and `sitemap.xml`. Fonts come
from Google Fonts; everything else is inline.

`index.html` is a complete HTML document (doctype, `<head>`, viewport, and the
rest). That matters: it is served raw by `latsite`, with nothing wrapping it, so
omitting the viewport meta would leave the page rendering at desktop width on
phones.

## Deploying

Serve the directory. That is the whole deployment.

```bash
# locally
python3 -m http.server 8000 --directory website

# Caddy
lattice.codeminute.dev {
    root * /srv/lattice/website
    file_server
    encode gzip
}
```

Netlify, Cloudflare Pages, GitHub Pages, or an S3 bucket all work the same way:
point them at `website/` with no build command.

## Turning on the live countdown

By default the page shows only the parts of the emission schedule that are true
regardless of chain state — the starting reward, epoch length, per-epoch
issuance, and the reward curve — and says plainly that no node is connected.
Nothing is faked or guessed.

To show live height, current reward, and a real countdown, it needs a node. A
browser cannot call `latticed` directly: the RPC interface uses HTTP basic auth
over TLS and sends no CORS headers, and putting node credentials in a web page
would be a bad idea anyway.

`latstatus` bridges the gap. It holds the credentials server-side and
re-publishes a closed set of read-only methods:

| Endpoint | What it answers |
| --- | --- |
| `/status` | `getnextreset` — the emission countdown the front page renders |
| `/api/chain` | height, difficulty, best block, peers, emission |
| `/api/address?a=` | balance, maturing balance, recent transactions |
| `/api/tx?id=` | one transaction |
| `/api/block?id=` | one block, by hash or height |

Every one is read-only by construction. There is no endpoint that moves coins
and no path from the shim to a wallet, which is what makes it safe to expose:
compromising the web server cannot cost anyone their funds. `/api/address`
needs the node running with `addrindex=1`.

```bash
go build -o bin/latstatus ./node/cmd/latstatus


./bin/latstatus \
  -node http://127.0.0.1:44107 \
  -user rpcuser -pass rpcpass \
  -listen 127.0.0.1:8099 \
  -origin https://lattice.codeminute.dev
```

Then set `STATUS_URL` near the bottom of `index.html`:

```js
const STATUS_URL = "https://lattice.codeminute.dev/status";
```

Proxy `/status` and `/api/*` to the shim so the pages and the endpoints share an
origin:

```
lattice.codeminute.dev {
    root * /srv/lattice/website
    file_server
    handle /status  { reverse_proxy 127.0.0.1:8099 }
    handle /api/*   { reverse_proxy 127.0.0.1:8099 }
    encode gzip
}
```

## The wallet app

`website/app/index.html` is the watch-only wallet at `/app/`. It looks up any
address, follows the chain, and explores blocks and transactions — all from the
read-only API above.

It deliberately cannot spend. There is no field on that page that accepts a
seed, a key, or a passphrase, and the API behind it has no method that could use
one. This is not a limitation to be fixed later: a page served from a public
website is exactly the wrong place to type a passphrase, because whoever
compromises the site (a bad deploy, a hijacked DNS record, a stolen Cloudflare
session) then serves JavaScript that captures it. Spending lives in
`latwalletgui`, which runs on the user's own machine and is unreachable from
here.

`/app/?a=<address>` deep-links straight to a lookup.

The shim caches node responses for 5 seconds (`-cache`) and serves the last good
response if the node briefly goes away, so polling every 20 seconds costs the
node almost nothing.

## Editing

The emission constants are duplicated at the top of the page script:

```js
const K = 39420000, S_LATT = 78840002, EPOCH = 3153600, BLOCK_SECS = 40;
```

These mirror `node/chaincfg/emission.go`. If the consensus constants ever change
in a hardfork, update them here too — the hero chart and the reward table are
drawn from them. Everything else on the page is static prose.
