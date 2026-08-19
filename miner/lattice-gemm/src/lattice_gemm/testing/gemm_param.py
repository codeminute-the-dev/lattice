from dataclasses import KW_ONLY, dataclass

import torch
from lattice_gemm_build_utils.kernel_configs import (
    MatmulKernelConfig,
    NoisingAKernelConfig,
    NoisingBKernelConfig,
)

torch_denoise_types = {
    "fp16": torch.float16,
    "int32": torch.int32,
}


# class to hold the test params
@dataclass
class GEMMParam:
    m: int
    n: int
    k: int
    R: int = 64
    num_stages: int = 1  # Number of tensor copies for rotating memory access (1 = no rotation)
    _: KW_ONLY
    matmul_config: MatmulKernelConfig | None = None
    noising_a_config: NoisingAKernelConfig | None = None
    noising_b_config: NoisingBKernelConfig | None = None
    tile_size_m: int = 128
    tile_size_n: int = 128
    tile_size_k: int = 128
    pipeline_stages: int = 3
    cluster_size_m: int = 1
    cluster_size_n: int = 1
    tile_size_m_noising_A: int | None = None
    tile_size_n_noising_B: int | None = None
    tile_size_k_noising_A: int | None = None
    tile_size_k_noising_B: int | None = None
    pipeline_stages_noising_A: int = 2
    pipeline_stages_noising_B: int = 2
    AxEBL_type_noising: torch.dtype | str = torch.float16
    EARxBpEB_type_noising: torch.dtype | str = torch.float16
    k_blocks_per_split_noising_A: int | None = None
    k_blocks_per_split_noising_B: int | None = None
    swizzle: int | None = None
    swizzle_n_maj: bool = True
    skip_reduction: bool = False
    skip_denoising: bool = False
    skip_noising_a: bool = False
    skip_noising_b: bool = False
    use_variable_scales: bool = False

    def __post_init__(self):
        if self.matmul_config and self.noising_a_config:
            assert self.matmul_config.R == self.noising_a_config.R
        if self.matmul_config and self.noising_b_config:
            assert self.matmul_config.R == self.noising_b_config.R
        if self.noising_a_config and self.noising_b_config:
            assert self.noising_a_config.R == self.noising_b_config.R

        if self.matmul_config:
            self.tile_size_m = self.matmul_config.tile_size_m
            self.tile_size_n = self.matmul_config.tile_size_n
            self.tile_size_k = self.matmul_config.tile_size_k
            self.R = self.matmul_config.R
            self.pipeline_stages = self.matmul_config.pipeline_stages
            self.cluster_size_m = self.matmul_config.cM
            self.cluster_size_n = self.matmul_config.cN
        if self.noising_a_config:
            self.tile_size_m_noising_A = self.noising_a_config.tile_size_m
            self.R = self.noising_a_config.R
            self.AxEBL_type_noising = self.noising_a_config.AxEBL_type
        if self.noising_b_config:
            self.tile_size_n_noising_B = self.noising_b_config.tile_size_n
            self.R = self.noising_b_config.R
            self.EARxBpEB_type_noising = self.noising_b_config.EARxBpEB_type

        if isinstance(self.AxEBL_type_noising, str):
            self.AxEBL_type_noising = torch_denoise_types[self.AxEBL_type_noising]
        if isinstance(self.EARxBpEB_type_noising, str):
            self.EARxBpEB_type_noising = torch_denoise_types[self.EARxBpEB_type_noising]
