from typing import ClassVar

import torch
from blake3 import blake3

from lattice_gemm import (
    get_host_signal_header_size,
    get_host_signal_sync_size,
    kEALScaleFactorDenoise,
    kEBRScaleFactorDenoise,
    make_pow_target_tensor,
)


def compute_EAR_and_EBL(ear_col_indices, ebl_col_indices, R):
    """Build sparse EAR and EBL matrices from random column index pairs.

    Each row of the output has exactly one +1 and one -1, placed at the
    column positions given by the two indices.  Returns R-major tensors.
    """
    assert ear_col_indices.shape[-1] == 2
    assert ear_col_indices.shape == ebl_col_indices.shape
    assert ear_col_indices.dtype == torch.uint8
    assert ebl_col_indices.dtype == torch.uint8
    assert ebl_col_indices.device == ear_col_indices.device
    staged = len(ear_col_indices.shape) == 3
    k = ear_col_indices.shape[-2]
    device = ear_col_indices.device
    # use vectorized indexing for improved performance
    if staged:
        s = ear_col_indices.shape[0]
        rows = torch.arange(k, device=device)[None, :].expand(s, k)
        stages = torch.arange(s, device=device)[:, None].expand(s, k)
        EAR = torch.zeros(size=(s, k, R), dtype=torch.int8, device=device)
        EBL = torch.zeros(size=(s, k, R), dtype=torch.int8, device=device)
        ear_0 = (ear_col_indices[:, :, 0]).to(torch.int)
        ear_1 = (ear_col_indices[:, :, 1]).to(torch.int)
        ebl_0 = (ebl_col_indices[:, :, 0]).to(torch.int)
        ebl_1 = (ebl_col_indices[:, :, 1]).to(torch.int)
        EAR[stages, rows, ear_0] += 1
        EAR[stages, rows, ear_1] -= 1
        EBL[stages, rows, ebl_0] += 1
        EBL[stages, rows, ebl_1] -= 1
    else:
        EAR = torch.zeros(size=(k, R), dtype=torch.int8, device=device)
        EBL = torch.zeros(size=(k, R), dtype=torch.int8, device=device)
        rows = torch.arange(k, device=device)
        ear_0 = (ear_col_indices[:, 0]).to(torch.int)
        ear_1 = (ear_col_indices[:, 1]).to(torch.int)
        ebl_0 = (ebl_col_indices[:, 0]).to(torch.int)
        ebl_1 = (ebl_col_indices[:, 1]).to(torch.int)
        EAR[rows, ear_0] += 1
        EAR[rows, ear_1] -= 1
        EBL[rows, ebl_0] += 1
        EBL[rows, ebl_1] -= 1

    return EAR, EBL


# Class for creating and holding the tensors. To have custom generation for
#  some Tensors (e.g. non constant scale_A/B) then extend the class on overload the
#  relevant function.
class GemmTensorGenerator:
    # static variables
    device: ClassVar[str] = "cuda"

    # Tensors that are batched by num_stages (stored with _ prefix internally)
    _batched_tensors: ClassVar[frozenset[str]] = frozenset(
        [
            "A",
            "B",
            "EAL",
            "EAL_fp16",
            "EBR",
            "EBR_fp16",
            "EAR_R_major",
            "EAR_K_major",
            "EBL_R_major",
            "EBL_K_major",
            "ApEA",
            "BpEB",
            "AxEBL",
            "EARxBpEB",
            "AxEBL_int32",
            "EARxBpEB_int32",
            "AxEBL_fp16",
            "EARxBpEB_fp16",
            "A_scales",
            "B_scales",
        ]
    )

    def __init__(self, gemm_params):
        self.gemm_params = gemm_params
        self.num_stages = gemm_params.num_stages

    def __getattribute__(self, name):
        """Auto-index batched tensors at stage 0 for backward compatibility."""
        if name in super().__getattribute__("_batched_tensors"):
            tensor = super().__getattribute__(f"_{name}")
            # don't index into tensor if it's actually None
            return tensor[0] if tensor is not None else tensor
        return super().__getattribute__(name)

    def __getitem__(self, stage):
        """Get tensors for a specific stage (for rotating memory access)."""
        from types import SimpleNamespace

        view = SimpleNamespace()
        for name in self._batched_tensors:
            tensor = getattr(self, f"_{name}")
            tensor_stage = tensor[0] if tensor is not None else tensor
            setattr(view, name, tensor_stage)
        view.C = self.C
        view.host_signal_header_pinned = self.host_signal_header_pinned
        view.host_signal_sync = self.host_signal_sync
        view.pow_target = self.pow_target
        view.pow_key = self.pow_key
        return view

    # operand tensor creations - A, B, C
    def create_operands_and_output(self, low=None, high=None):
        if low is None or high is None:
            low = -64
            high = 63

        self._A = torch.randint(
            low=low,
            high=high,
            size=(self.num_stages, self.gemm_params.m, self.gemm_params.k),
            dtype=torch.int8,
            device="cuda",
        )
        self._B = torch.randint(
            low=low,
            high=high,
            size=(self.num_stages, self.gemm_params.n, self.gemm_params.k),
            dtype=torch.int8,
            device="cuda",
        )
        # OUTPUT tensor: shared (not batched)
        self.C = torch.zeros(
            size=(self.gemm_params.m, self.gemm_params.n),
            dtype=torch.bfloat16,
            device="cuda",
        )

    def create_noising_factors(self):
        idxs_per_col = 2
        max_random_val = 64 // idxs_per_col

        self._EAL = torch.randint(
            low=-max_random_val,
            high=max_random_val,
            size=(self.num_stages, self.gemm_params.m, self.gemm_params.R),
            dtype=torch.int8,
            device="cuda",
        )
        # Random column indices used to construct sparse EAR/EBL matrices
        ear_col_indices = torch.randint(
            low=0,
            high=self.gemm_params.R,
            size=(self.num_stages, self.gemm_params.k, idxs_per_col),
            dtype=torch.uint8,
            device="cuda",
        )
        ebl_col_indices = torch.randint(
            low=0,
            high=self.gemm_params.R,
            size=(self.num_stages, self.gemm_params.k, idxs_per_col),
            dtype=torch.uint8,
            device="cuda",
        )
        self._EBR = torch.randint(
            low=-max_random_val,
            high=max_random_val,
            size=(self.num_stages, self.gemm_params.n, self.gemm_params.R),
            dtype=torch.int8,
            device="cuda",
        )
        # see lattice_gemm_constants.hpp for more info on scale factors
        self._EBR_fp16 = (kEBRScaleFactorDenoise * ((self._EBR).to(torch.int32))).to(torch.float16)
        self._EAL_fp16 = (kEALScaleFactorDenoise * ((self._EAL).to(torch.int32))).to(torch.float16)
        self._EAR_R_major, self._EBL_R_major = compute_EAR_and_EBL(
            ear_col_indices, ebl_col_indices, self.gemm_params.R
        )
        self._EAR_K_major = self._EAR_R_major.clone().transpose(-1, -2).contiguous()
        self._EBL_K_major = self._EBL_R_major.clone().transpose(-1, -2).contiguous()

    def create_noising_tensors(self):
        # Use randint to match int7 range
        self._ApEA = torch.randint(
            low=-63,
            high=64,
            size=(self.num_stages, self.gemm_params.m, self.gemm_params.k),
            dtype=torch.int8,
            device="cuda",
        )
        self._BpEB = torch.randint(
            low=-63,
            high=64,
            size=(self.num_stages, self.gemm_params.n, self.gemm_params.k),
            dtype=torch.int8,
            device="cuda",
        )
        self._AxEBL = torch.zeros(
            size=(self.num_stages, self.gemm_params.m, self.gemm_params.R),
            dtype=self.gemm_params.AxEBL_type_noising,
            device="cuda",
        )
        self._EARxBpEB = torch.zeros(
            size=(self.num_stages, self.gemm_params.n, self.gemm_params.R),
            dtype=self.gemm_params.EARxBpEB_type_noising,
            device="cuda",
        )
        self._AxEBL_fp16 = (
            self._AxEBL
            if self.gemm_params.AxEBL_type_noising == torch.float16
            else torch.zeros(
                size=(self.num_stages, self.gemm_params.m, self.gemm_params.R),
                dtype=torch.float16,
                device="cuda",
            )
        )
        self._EARxBpEB_fp16 = (
            self._EARxBpEB
            if self.gemm_params.EARxBpEB_type_noising == torch.float16
            else torch.zeros(
                size=(self.num_stages, self.gemm_params.n, self.gemm_params.R),
                dtype=torch.float16,
                device="cuda",
            )
        )
        self._AxEBL_int32 = (
            self._AxEBL if self.gemm_params.AxEBL_type_noising == torch.int32 else None
        )
        self._EARxBpEB_int32 = (
            self._EARxBpEB if self.gemm_params.EARxBpEB_type_noising == torch.int32 else None
        )

    def create_scaling_factors(self):
        if self.gemm_params.use_variable_scales:
            self._A_scales = (
                torch.randint(
                    low=0,
                    high=2,
                    size=(self.num_stages, self.gemm_params.m),
                    dtype=torch.float32,
                    device="cuda",
                )
                / 128
            )
            self._B_scales = (
                torch.randint(
                    low=0,
                    high=2,
                    size=(self.num_stages, self.gemm_params.n),
                    dtype=torch.float32,
                    device="cuda",
                )
                / 128
            )
        else:
            self._A_scales = (
                torch.ones(
                    size=(self.num_stages, self.gemm_params.m),
                    dtype=torch.float32,
                    device="cuda",
                )
                / 128
            )
            self._B_scales = (
                torch.ones(
                    size=(self.num_stages, self.gemm_params.n),
                    dtype=torch.float32,
                    device="cuda",
                )
                / 128
            )

    def create_misc_tensors(self, pow_target: int | None):
        h_header_size = get_host_signal_header_size()
        self.host_signal_header_pinned = torch.zeros(
            size=(h_header_size,),
            dtype=torch.int8,
            pin_memory=True,
        )
        h_sync_size = get_host_signal_sync_size()
        self.host_signal_sync = torch.zeros(
            size=(h_sync_size,),
            dtype=torch.int8,
            device="cuda",
        )
        self.pow_target = make_pow_target_tensor(pow_target if pow_target is not None else 1)
        self.pow_key = torch.randint(
            low=0, high=255, size=(blake3.key_size,), dtype=torch.uint8, device="cuda"
        ).view(torch.uint32)

    def generate(self, pow_target: int | None = None):
        self.create_operands_and_output()
        self.create_noising_factors()
        self.create_noising_tensors()
        self.create_scaling_factors()
        self.create_misc_tensors(pow_target)
