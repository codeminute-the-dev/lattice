#!/usr/bin/env python3
"""Mine against a lattice-gateway from a consumer GPU.

    MINER_RPC_SOCKET_PATH=/tmp/latticegw.sock python mine.py --m 2048 --n 2048 --k 2048

Each attempt uses fresh random operands (the commitment, and so the noise,
changes with them). Blocks found are submitted as plain proofs.
"""

import argparse
import os
import time

# Same numbering as nvidia-smi, so --device 1 is the second card in its list.
os.environ.setdefault("CUDA_DEVICE_ORDER", "PCI_BUS_ID")

import torch
from lattice_gateway.config import MinerRpcConfig
from miner_base.gateway_client import MiningClient

import ada_miner


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--m", type=int, default=2048)
    ap.add_argument("--n", type=int, default=2048)
    ap.add_argument("--k", type=int, default=2048)
    ap.add_argument("--device", type=int, default=0, help="GPU index as in nvidia-smi (one process per GPU)")
    ap.add_argument("--max-blocks", type=int, default=0, help="stop after this many (0 = run forever)")
    ap.add_argument("--report", type=float, default=10.0, help="seconds between stats lines")
    args = ap.parse_args()
    assert args.k >= 2048 and args.k % 64 == 0 and args.k <= 65536, "consensus bounds on k"

    torch.cuda.set_device(args.device)
    print(f"GPU {args.device}: {torch.cuda.get_device_name(args.device)}, "
          f"kernel {ada_miner.lattice_ada.best_config()} (1 = tensor cores, 2 = dp4a)", flush=True)

    # The gateway may still be starting (or restarting): wait for it.
    while True:
        try:
            client = MiningClient(MinerRpcConfig())
            job = client.get_mining_info()
            break
        except Exception as e:
            print(f"waiting for the gateway: {e}", flush=True)
            time.sleep(2)
    job_at = time.time()
    solved = None  # header of the last job we submitted a block for
    attempts = tiles = blocks = 0
    busy = 0.0  # seconds spent hashing (the miner idles once a template is solved)
    t0 = last = time.time()
    tiles_per_attempt = (args.m // 16) * (args.n // 16)

    while True:
        now = time.time()
        if now - last >= args.report:
            rate = tiles / busy / 1e6 if busy else 0.0
            print(f"{attempts} attempts, {rate:.1f}M tile hashes/s while hashing "
                  f"({100 * busy / (now - t0):.0f}% busy), blocks={blocks}", flush=True)
            last = now
        if time.time() - job_at > 0.5:
            try:
                job = client.get_mining_info()
            except Exception as e:  # gateway pauses while the node has no template
                print(f"no job: {e}", flush=True)
                time.sleep(1)
                continue
            job_at = time.time()
        if job.incomplete_header_bytes == solved:
            time.sleep(0.05)  # this template is done; wait for the next one
            continue
        A, Bt = ada_miner.random_operands(args.m, args.n, args.k)
        t_a = time.time()
        hit = ada_miner.attempt(job, A, Bt)
        busy += time.time() - t_a
        attempts += 1
        tiles += tiles_per_attempt
        if hit:
            proof = ada_miner.make_proof(job, hit)
            try:
                client.submit_plain_proof(proof, job)
                solved = job.incomplete_header_bytes
                blocks += 1
                print(f"submitted block proof for tile ({hit.row}, {hit.col})", flush=True)
            except Exception as e:
                print(f"submit failed: {e}", flush=True)
            if args.max_blocks and blocks >= args.max_blocks:
                break


if __name__ == "__main__":
    main()
