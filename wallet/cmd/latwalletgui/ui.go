// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

// indexHTML is the whole UI. It is one file on purpose: the page must load
// with no external fetches (the CSP forbids them), and a wallet UI with no
// build step is one fewer thing that can go quietly wrong between the source
// you read and the page you type a passphrase into.
const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>Lattice Wallet</title>
<style>
  :root {
    --bg: #0f1115; --panel: #171a21; --panel2: #1e222b; --line: #2a2f3a;
    --text: #e7e9ee; --dim: #98a0b0; --accent: #6ee7a8; --accent2: #7aa2f7;
    --warn: #f7c96b; --bad: #f77a7a;
    --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; background: var(--bg); color: var(--text);
    font: 15px/1.5 ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
  }
  header {
    display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
    padding: 14px 20px; border-bottom: 1px solid var(--line); background: var(--panel);
  }
  header h1 { font-size: 17px; margin: 0; font-weight: 600; letter-spacing: .2px; }
  .chip {
    font: 12px/1 var(--mono); padding: 5px 9px; border-radius: 999px;
    background: var(--panel2); border: 1px solid var(--line); color: var(--dim);
  }
  .chip.ok { color: var(--accent); border-color: #24503a; }
  .chip.bad { color: var(--bad); border-color: #532a2a; }
  main { max-width: 1080px; margin: 0 auto; padding: 20px; display: grid; gap: 16px; }
  .grid { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); }
  section {
    background: var(--panel); border: 1px solid var(--line); border-radius: 10px; padding: 16px;
  }
  section h2 {
    margin: 0 0 12px; font-size: 12px; text-transform: uppercase;
    letter-spacing: .09em; color: var(--dim); font-weight: 600;
  }
  .bal { font: 600 30px/1.15 var(--mono); }
  .bal small { font-size: 14px; color: var(--dim); font-weight: 400; }
  .sub { color: var(--dim); font-size: 13px; margin-top: 6px; }
  label { display: block; font-size: 12px; color: var(--dim); margin: 10px 0 4px; }
  input, button, select {
    font: inherit; border-radius: 7px; border: 1px solid var(--line);
    background: var(--panel2); color: var(--text); padding: 9px 11px; width: 100%;
  }
  input[type=password], input.mono { font-family: var(--mono); font-size: 13px; }
  button {
    cursor: pointer; background: var(--panel2); border-color: #39415280;
    font-weight: 500; transition: border-color .12s, color .12s;
  }
  button:hover:not(:disabled) { border-color: var(--accent2); color: var(--accent2); }
  button:disabled { opacity: .5; cursor: not-allowed; }
  button.primary { background: #1d3b2c; border-color: #2f6b4c; color: var(--accent); }
  button.primary:hover:not(:disabled) { border-color: var(--accent); }
  .row { display: flex; gap: 8px; align-items: center; }
  .row > * { flex: 1; }
  .row > button { flex: 0 0 auto; width: auto; padding-inline: 14px; }
  .addr {
    font: 12px/1.45 var(--mono); word-break: break-all; background: var(--panel2);
    border: 1px solid var(--line); border-radius: 7px; padding: 10px; user-select: all;
  }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th { text-align: left; color: var(--dim); font-weight: 500; font-size: 11px;
       text-transform: uppercase; letter-spacing: .06em; padding: 0 8px 8px 0; }
  td { padding: 7px 8px 7px 0; border-top: 1px solid var(--line); vertical-align: top; }
  td.num { text-align: right; font-family: var(--mono); white-space: nowrap; }
  .pos { color: var(--accent); } .neg { color: var(--bad); } .imm { color: var(--warn); }
  .txid { font: 11px/1.4 var(--mono); color: var(--dim); word-break: break-all; }
  .msg { margin-top: 10px; font-size: 13px; padding: 9px 11px; border-radius: 7px; display: none; }
  .msg.show { display: block; }
  .msg.ok { background: #14301f; color: var(--accent); }
  .msg.err { background: #331a1a; color: var(--bad); }
  .note { font-size: 12px; color: var(--dim); margin-top: 10px; }
  footer { color: var(--dim); font-size: 12px; text-align: center; padding: 4px 20px 28px; }
  .empty { color: var(--dim); font-size: 13px; padding: 10px 0; }
</style>
</head>
<body>
<header>
  <h1>Lattice Wallet</h1>
  <span class="chip" id="c-net">connecting…</span>
  <span class="chip" id="c-height">height —</span>
  <span class="chip" id="c-peers">peers —</span>
  <span class="chip" id="c-lock">locked</span>
</header>

<main>
  <div class="grid">
    <section>
      <h2>Balance</h2>
      <div class="bal" id="bal">—<small> LATT</small></div>
      <div class="sub" id="bal-sub">spendable</div>
      <div class="sub" id="bal-imm"></div>
      <div class="note">Mined coins need 100 confirmations before they can be spent.</div>
    </section>

    <section>
      <h2>Receive</h2>
      <div class="addr" id="addr">—</div>
      <div class="row" style="margin-top:10px">
        <button id="btn-copy">Copy</button>
        <button id="btn-new">New address</button>
      </div>
      <div class="msg" id="recv-msg"></div>
    </section>
  </div>

  <section>
    <h2>Send</h2>
    <div class="grid" style="grid-template-columns:repeat(auto-fit,minmax(220px,1fr))">
      <div>
        <label for="to">To address</label>
        <input id="to" class="mono" placeholder="lat1…" autocomplete="off" spellcheck="false">
      </div>
      <div>
        <label for="amt">Amount (LATT)</label>
        <input id="amt" type="number" step="0.00000001" min="0" placeholder="0.00000000">
      </div>
      <div>
        <label for="pass">Wallet passphrase</label>
        <input id="pass" type="password" placeholder="to unlock for this send" autocomplete="current-password">
      </div>
    </div>
    <div class="row" style="margin-top:14px">
      <button class="primary" id="btn-send">Send</button>
      <button id="btn-lock">Lock wallet</button>
    </div>
    <div class="msg" id="send-msg"></div>
    <div class="note">
      The passphrase unlocks the wallet for 60 seconds, is used for this one send, and is
      never stored. It is not written to the server log.
    </div>
  </section>

  <section>
    <h2>Transactions</h2>
    <table>
      <thead><tr><th>When</th><th>Type</th><th>Address</th><th class="num">Amount</th><th class="num">Conf</th></tr></thead>
      <tbody id="txs"></tbody>
    </table>
    <div class="empty" id="tx-empty">No transactions yet.</div>
  </section>
</main>

<footer>Serving a local latwallet. Nothing here leaves this machine.</footer>

<script>
const TOKEN = "__TOKEN__";

async function rpc(target, method, params) {
  const r = await fetch("/api/rpc", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Lat-Token": TOKEN },
    body: JSON.stringify({ target, method, params: (params || []).map(p => p) })
  });
  if (!r.ok) throw new Error(await r.text());
  const j = await r.json();
  if (j.error) throw new Error(j.error.message || JSON.stringify(j.error));
  return j.result;
}

function show(id, text, ok) {
  const el = document.getElementById(id);
  el.textContent = text;
  el.className = "msg show " + (ok ? "ok" : "err");
  if (ok) setTimeout(() => { el.className = "msg"; }, 6000);
}

function fmt(n) {
  return Number(n).toLocaleString(undefined, { minimumFractionDigits: 8, maximumFractionDigits: 8 });
}

function setChip(id, text, cls) {
  const el = document.getElementById(id);
  el.textContent = text;
  el.className = "chip" + (cls ? " " + cls : "");
}

async function refreshChain() {
  // getblockcount rather than getblockchaininfo: the latter can hang on this
  // node while the CPU miner holds the chain lock, and the UI must not stall
  // behind it.
  try {
    const h = await rpc("node", "getblockcount", []);
    setChip("c-net", "mainnet", "ok");
    setChip("c-height", "height " + h);
  } catch (e) {
    setChip("c-net", "node offline", "bad");
  }
  try {
    const peers = await rpc("node", "getconnectioncount", []);
    setChip("c-peers", "peers " + peers, peers > 0 ? "" : "bad");
  } catch (e) { setChip("c-peers", "peers —"); }
}

async function refreshWallet() {
  try {
    const bal = await rpc("wallet", "getbalance", []);
    document.getElementById("bal").innerHTML = fmt(bal) + "<small> LATT</small>";
  } catch (e) {
    document.getElementById("bal").innerHTML = "—<small> LATT</small>";
    document.getElementById("bal-sub").textContent = "wallet offline: " + e.message;
    return;
  }
  document.getElementById("bal-sub").textContent = "spendable";

  let txs = [];
  try { txs = await rpc("wallet", "listtransactions", ["*", 50, 0]) || []; } catch (e) {}
  txs = txs.slice().reverse();

  const imm = txs.filter(t => t.category === "immature")
                 .reduce((s, t) => s + Number(t.amount), 0);
  document.getElementById("bal-imm").textContent =
    imm > 0 ? fmt(imm) + " LATT maturing" : "";

  const body = document.getElementById("txs");
  body.textContent = "";
  document.getElementById("tx-empty").style.display = txs.length ? "none" : "block";
  for (const t of txs) {
    const tr = document.createElement("tr");
    const amtCls = t.category === "immature" ? "imm" : (Number(t.amount) < 0 ? "neg" : "pos");
    const cells = [
      [new Date(t.time * 1000).toLocaleString(), ""],
      [t.category + (t.generated ? " (mined)" : ""), ""],
      [t.address || "—", "txid"],
      [fmt(t.amount), "num " + amtCls],
      [String(t.confirmations), "num"]
    ];
    for (const [text, cls] of cells) {
      const td = document.createElement("td");
      td.textContent = text;
      if (cls) td.className = cls;
      tr.appendChild(td);
    }
    body.appendChild(tr);
  }
}

async function loadAddress(fresh) {
  try {
    const a = fresh
      ? await rpc("wallet", "getnewaddress", [])
      : await rpc("wallet", "getaccountaddress", ["default"]);
    document.getElementById("addr").textContent = a;
    if (fresh) show("recv-msg", "New address generated.", true);
  } catch (e) { show("recv-msg", e.message, false); }
}

document.getElementById("btn-new").onclick = () => loadAddress(true);

document.getElementById("btn-copy").onclick = async () => {
  const a = document.getElementById("addr").textContent;
  try {
    await navigator.clipboard.writeText(a);
    show("recv-msg", "Address copied.", true);
  } catch (e) {
    show("recv-msg", "Copy failed — select the address and copy manually.", false);
  }
};

document.getElementById("btn-lock").onclick = async () => {
  try {
    await rpc("wallet", "walletlock", []);
    setChip("c-lock", "locked");
    show("send-msg", "Wallet locked.", true);
  } catch (e) { show("send-msg", e.message, false); }
};

document.getElementById("btn-send").onclick = async () => {
  const btn = document.getElementById("btn-send");
  const to = document.getElementById("to").value.trim();
  const amt = parseFloat(document.getElementById("amt").value);
  const pass = document.getElementById("pass").value;
  if (!to) return show("send-msg", "Enter a destination address.", false);
  if (!(amt > 0)) return show("send-msg", "Enter an amount greater than zero.", false);
  if (!pass) return show("send-msg", "Enter your wallet passphrase.", false);

  btn.disabled = true;
  try {
    const v = await rpc("wallet", "validateaddress", [to]);
    if (!v || !v.isvalid) throw new Error("That is not a valid Lattice address.");

    await rpc("wallet", "walletpassphrase", [pass, 60]);
    setChip("c-lock", "unlocked 60s", "ok");
    document.getElementById("pass").value = "";

    const txid = await rpc("wallet", "sendtoaddress", [to, amt]);
    show("send-msg", "Sent. txid " + txid, true);
    document.getElementById("to").value = "";
    document.getElementById("amt").value = "";
    await refreshWallet();
  } catch (e) {
    show("send-msg", e.message, false);
  } finally {
    btn.disabled = false;
    try { await rpc("wallet", "walletlock", []); setChip("c-lock", "locked"); } catch (e) {}
  }
};

async function tick() { await refreshChain(); await refreshWallet(); }
loadAddress(false);
tick();
setInterval(tick, 8000);
</script>
</body>
</html>`
