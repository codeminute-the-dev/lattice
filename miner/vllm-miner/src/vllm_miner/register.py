"""
Lattice quantization plugin for vLLM.

Extends CompressedTensorsConfig to add support for:
- Mining layer (7-bit quantization + noisy GEMM) -> LatticeScheme(mining_enabled=True)
- Non-mining layer (8-bit quantization + vanilla GEMM) -> LatticeScheme(mining_enabled=False)

Both use lattice GEMM kernels and handle smooth_quant_scale internally.
"""

import multiprocessing

from miner_base.settings import MinerSettings
from miner_utils import get_logger

from .mining_state import (
    get_async_manager,
    init_async_manager,
    init_pinned_pool,
)
from .vllm_config import LatticeConfig

_LOGGER = get_logger("vllm.lattice_miner")


def _is_vllm_worker() -> bool:
    # v1 engine: worker processes show up as "EngineCore_DP{rank}"
    # In multi-gpu setups, "VllmWorker-{number}" is used (has a different name in logs)
    name = multiprocessing.current_process().name or ""
    return name.startswith("EngineCore") or name.startswith("VllmWorker")


def register_lattice_miner_layer() -> None:
    """
    Register the LatticeMiner layer.
    The gateway socket path is loaded from the configuration file.
    """
    from vllm.model_executor.layers.quantization import register_quantization_config

    # Initialize the global mining state, but only if we're running in a vLLM *worker*
    # We only want to start threads or pre-allocate the pinned pool in workers.
    if _is_vllm_worker():
        init_async_manager()
        init_pinned_pool(get_async_manager()._conf.pinned_pool_size)
        init_plugin = not get_async_manager()._conf.no_vllm_plugin
    else:
        init_plugin = not MinerSettings().no_vllm_plugin

    if init_plugin:
        register_quantization_config("lattice")(LatticeConfig)
