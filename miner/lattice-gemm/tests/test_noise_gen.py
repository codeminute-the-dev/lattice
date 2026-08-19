from enum import Enum, auto

import numpy as np
import numpy.testing as npt
import pytest
import torch
from lattice_gemm import noise_gen
from lattice_gemm.testing import noise_gen_ref_random, noise_gen_ref_sparse

KEY_LEN = 32

noise_gen_setups = {
    "noising_A": ["EAL", "EAR_R_major", "EBL_K_major", "key_A", "key_B"],
    "noising_B": ["EBR", "EAR_K_major", "EBL_R_major", "key_A", "key_B"],
    "all": [
        "EAL",
        "EBR",
        "EAL_fp16",
        "EBR_fp16",
        "EAR_R_major",
        "EAR_K_major",
        "EBL_R_major",
        "EBL_K_major",
        "key_A",
        "key_B",
    ],
}


class NoiseGenMatrixType(Enum):
    DENSE = auto()
    SPARSE = auto()


class NoiseGenTensorGenerator:
    def __init__(self, gen_which, m, n, k, R, device="cuda"):
        self.tensor_list = noise_gen_setups[gen_which]
        self.m = m
        self.n = n
        self.k = k
        self.R = R
        self.device = device

    def make_optional_noise_gen_tensor(self, name, size, dtype, key=False):
        if name in self.tensor_list:
            if key:
                return torch.randint(low=0, high=255, size=size, dtype=dtype, device=self.device)
            else:
                return torch.empty(size=size, dtype=dtype, device=self.device)
        else:
            return None

    def generate(self):
        self.EAL = self.make_optional_noise_gen_tensor("EAL", (self.m, self.R), torch.int8)
        self.EBR = self.make_optional_noise_gen_tensor("EBR", (self.n, self.R), torch.int8)
        self.EAL_fp16 = self.make_optional_noise_gen_tensor(
            "EAL_fp16", (self.m, self.R), torch.float16
        )
        self.EBR_fp16 = self.make_optional_noise_gen_tensor(
            "EBR_fp16", (self.n, self.R), torch.float16
        )
        self.EAR_R_major = self.make_optional_noise_gen_tensor(
            "EAR_R_major", (self.k, self.R), torch.int8
        )
        self.EBL_R_major = self.make_optional_noise_gen_tensor(
            "EBL_R_major", (self.k, self.R), torch.int8
        )
        self.EAR_K_major = self.make_optional_noise_gen_tensor(
            "EAR_K_major", (self.R, self.k), torch.int8
        )
        self.EBL_K_major = self.make_optional_noise_gen_tensor(
            "EBL_K_major", (self.R, self.k), torch.int8
        )
        self.key_A = self.make_optional_noise_gen_tensor("key_A", (KEY_LEN,), torch.uint8, key=True)
        self.key_B = self.make_optional_noise_gen_tensor("key_B", (KEY_LEN,), torch.uint8, key=True)

        self.seed_A = torch.frombuffer(bytearray(b"A_tensor" + b"\x00" * 24), dtype=torch.uint8).to(
            self.device
        )
        self.seed_B = torch.frombuffer(bytearray(b"B_tensor" + b"\x00" * 24), dtype=torch.uint8).to(
            self.device
        )

    def verify(
        self, name, matrix_type: NoiseGenMatrixType, noisingA=True, transpose=False, is_fp16=False
    ):
        if self.__dict__[name] is not None:
            ours = self.__dict__[name]
            ours_np = ours.cpu().numpy().transpose() if transpose else ours.cpu().numpy()
            ref_np = np.zeros_like(ours_np)
            if noisingA:
                seed = self.seed_A.cpu().numpy()
                key = self.key_A.cpu().numpy()
                scale_factor = -1  # for EAL_fp16
            else:
                seed = self.seed_B.cpu().numpy()
                key = self.key_B.cpu().numpy()
                scale_factor = -4  # for EBR_fp16
            if matrix_type == NoiseGenMatrixType.DENSE:
                noise_gen_ref_random(ref_np, seed, key, scale_factor, is_fp16)
            else:
                noise_gen_ref_sparse(ref_np, seed, key, self.R)
            npt.assert_equal(ours_np, ref_np, err_msg=name)


@pytest.mark.parametrize("R", [64, 128])
@pytest.mark.parametrize("gen_which", noise_gen_setups.keys())
@pytest.mark.parametrize("aux_buffer_size", [0, 128])
def test_noise_gen_opcheck(R, gen_which, aux_buffer_size):
    m, n, k = 256, 512, 1024
    torch.random.manual_seed(0)
    tg = NoiseGenTensorGenerator(gen_which, m, n, k, R)
    tg.generate()

    if aux_buffer_size > 0:
        aux_buffer = torch.ones(size=(aux_buffer_size,), dtype=torch.int32, device="cuda")
    else:
        aux_buffer = None

    args = [
        R,
        64,  # num_threads
        tg.EAL,
        tg.EAL_fp16,
        tg.EAR_R_major,
        tg.EAR_K_major,
        tg.EBL_R_major,
        tg.EBL_K_major,
        tg.EBR,
        tg.EBR_fp16,
        tg.key_A,
        tg.key_B,
        aux_buffer,
    ]
    torch.library.opcheck(torch.ops.lattice_gemm.noise_gen.default, args)


@pytest.mark.parametrize("R", [64, 128])
@pytest.mark.parametrize("m", [1, 4, 256, 555, 1000])
@pytest.mark.parametrize("n", [1, 4, 256, 555, 1000])
@pytest.mark.parametrize("k", [16, 64, 256, 4096, 8176])
@pytest.mark.parametrize("gen_which", noise_gen_setups.keys())
@pytest.mark.parametrize("aux_buffer_size", [128])
def test_noise_gen_output(R, m, n, k, gen_which, aux_buffer_size):
    torch.random.manual_seed(0)
    tg = NoiseGenTensorGenerator(gen_which, m, n, k, R)
    tg.generate()

    if aux_buffer_size > 0:
        aux_buffer = torch.ones(size=(aux_buffer_size,), dtype=torch.int32, device="cuda")
    else:
        aux_buffer = None

    noise_gen(
        R=R,
        EAL=tg.EAL,
        EAL_fp16=tg.EAL_fp16,
        EAR_R_major=tg.EAR_R_major,
        EAR_K_major=tg.EAR_K_major,
        EBL_R_major=tg.EBL_R_major,
        EBL_K_major=tg.EBL_K_major,
        EBR=tg.EBR,
        EBR_fp16=tg.EBR_fp16,
        key_A=tg.key_A,
        key_B=tg.key_B,
        aux_buffer=aux_buffer,
    )

    if aux_buffer_size > 0:
        assert torch.all(aux_buffer == 0), "aux_buffer should be all zeros"

    tg.verify("EAL", NoiseGenMatrixType.DENSE, noisingA=True)
    tg.verify("EBR", NoiseGenMatrixType.DENSE, noisingA=False)
    tg.verify("EAL_fp16", NoiseGenMatrixType.DENSE, noisingA=True, is_fp16=True)
    tg.verify("EBR_fp16", NoiseGenMatrixType.DENSE, noisingA=False, is_fp16=True)
    tg.verify("EAR_R_major", NoiseGenMatrixType.SPARSE, noisingA=True)
    tg.verify("EAR_K_major", NoiseGenMatrixType.SPARSE, noisingA=True, transpose=True)
    tg.verify("EBL_R_major", NoiseGenMatrixType.SPARSE, noisingA=False)
    tg.verify("EBL_K_major", NoiseGenMatrixType.SPARSE, noisingA=False, transpose=True)
