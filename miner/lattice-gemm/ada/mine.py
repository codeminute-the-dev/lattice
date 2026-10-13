#!/usr/bin/env python3
"""Mine against a lattice-gateway from a consumer GPU.

    MINER_RPC_SOCKET_PATH=/tmp/latticegw.sock python mine.py --m 2048 --n 2048 --k 2048

Each attempt uses fresh random operands (the commitment, and so the noise,
changes with them). Blocks found are submitted as plain proofs.
"""

import argparse
import time

import torch
from lattice_gateway.config import MinerRpcConfig
from miner_base.gateway_client import MiningClient

import ada_miner


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--m", type=int, default=2048)
    ap.add_argument("--n", type=int, default=2048)
    ap.add_argument("--k", type=int, default=2048)
    ap.add_argument("--max-blocks", type=int, default=0, help="stop after this many (0 = run forever)")
    ap.add_argument("--report", type=float, default=10.0, help="seconds between stats lines")
    args = ap.parse_args()
    assert args.k >= 2048 and args.k % 64 == 0 and args.k <= 65536, "consensus bounds on k"

    client = MiningClient(MinerRpcConfig())
    job = client.get_mining_info()
    job_at = time.time()
    solved = None  # header of the last job we submitted a block for
    attempts = tiles = blocks = 0
    t0 = last = time.time()
    tiles_per_attempt = (args.m // 16) * (args.n // 16)

    while True:
        if time.time() - job_at > 0.5:
            try:
                job = client.get_mining_info()
            except Exception as e:  # gateway pauses while the node has no template
                print(f"no job: {e}")
                time.sleep(1)
                continue
            job_at = time.time()
        A, Bt = ada_miner.random_operands(args.m, args.n, args.k)
        hit = ada_miner.attempt(job, A, Bt)
        attempts += 1
        tiles += tiles_per_attempt
        if hit and job.incomplete_header_bytes == solved:
            hit = None  # block for this template already submitted; wait for the next one
        if hit:
            proof = ada_miner.make_proof(job, hit)
            try:
                client.submit_plain_proof(proof, job)
                solved = job.incomplete_header_bytes
                blocks += 1
                print(f"submitted block proof for tile ({hit.row}, {hit.col})")
            except Exception as e:
                print(f"submit failed: {e}")
            if args.max_blocks and blocks >= args.max_blocks:
                break
        now = time.time()
        if now - last >= args.report:
            dt = now - t0
            print(f"{attempts / dt:.1f} attempts/s  {tiles / dt / 1e3:.0f}k tile hashes/s  blocks={blocks}")
            last = now


if __name__ == "__main__":
    main()
