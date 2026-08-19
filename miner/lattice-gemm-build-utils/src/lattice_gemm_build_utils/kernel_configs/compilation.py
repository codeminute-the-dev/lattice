"""
Kernel compilation grid schema.

Defines the complete set of kernels to compile for a given configuration.
Used by:
- default_compiled_kernels.py and release_kernels.py (direct instantiation)
- setup.py (loaded and filtered by R values)
"""

from pydantic import BaseModel, Field

from lattice_gemm_build_utils.kernel_configs.gemm import MatmulKernelConfig
from lattice_gemm_build_utils.kernel_configs.noising import NoisingAKernelConfig, NoisingBKernelConfig


class KernelCompilationGrid(BaseModel):
    """
    Complete set of kernels to compile.

    This schema represents all the kernel configurations that will be
    compiled into the library for a given build configuration.
    """

    model_config = {"frozen": True, "extra": "forbid"}

    matmul_kernels: list[MatmulKernelConfig] = Field(
        default_factory=list, description="GEMM/matmul kernel configurations to compile"
    )

    noising_a_kernels: list[NoisingAKernelConfig] = Field(
        default_factory=list, description="Noising A kernel configurations to compile"
    )

    noising_b_kernels: list[NoisingBKernelConfig] = Field(
        default_factory=list, description="Noising B kernel configurations to compile"
    )
