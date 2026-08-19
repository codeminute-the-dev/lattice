import numpy as np
from blake3 import blake3

SIZEOF_UINT32 = 4


def ceil_div(a, b):
    return (a + b - 1) // b


# assumes E, seed, key are numpy tensors on cpu
def noise_gen_ref_random(
    E,  # (m, R) int8
    seed,  #   (32), uint8
    key,  #   (32), uint8
    scale_factor,
    is_fp16=False,
):
    NOISE_ABS_MAX = 128
    PERM_IDXS_PER_COL = 2
    NOISE_RANGE = NOISE_ABS_MAX // PERM_IDXS_PER_COL
    key_bytes = key.tobytes()
    seed_bytes = seed.tobytes()

    m, R = E.shape
    num_messages = ceil_div(m * R, 32)

    for i in range(num_messages):
        R_offset = (i * 32) % R
        m_offset = (i * 32) // R
        message_prepend = np.zeros(8, dtype=np.int32)
        message_prepend[0] = 1 + i
        message_bytes = message_prepend.tobytes() + seed_bytes
        hash_bytes = blake3(message_bytes, key=key_bytes).digest()
        hash_int8 = np.frombuffer(hash_bytes, dtype=np.int8)
        for j in range(32):
            # Map to [-64, 63] range if 1 index, [-32, 32) if 2 indices
            val = np.int8(
                ((np.int32(hash_int8[j]) + NOISE_ABS_MAX) % NOISE_RANGE) - NOISE_RANGE / 2
            )
            E[m_offset, R_offset + j] = np.float16(np.int32(val) * scale_factor) if is_fp16 else val


# assumes E, seed, key are numpy tensors on cpu
def noise_gen_ref_index(
    E,  # (k, 2), uint8
    seed,  #   (32), uint8
    key,  #   (32), uint8
    R: int = 128,
):
    key_bytes = key.tobytes()
    seed_bytes = seed.tobytes()

    k = E.shape[0]
    # We use uint32 for each col
    num_k_per_message = blake3.digest_size // SIZEOF_UINT32
    num_messages = ceil_div(k, num_k_per_message)

    def np_mul_hi_u32(a: np.uint32, b: np.uint32):
        prod64 = a.astype(np.uint64) * b.astype(np.uint64)
        return (prod64 >> np.uint64(32)).astype(np.uint32)

    for i in range(num_messages):
        k_offset = i * num_k_per_message
        message_prepend = np.zeros(8, dtype=np.int32)
        message_prepend[1] = 1 + i
        message_bytes = message_prepend.tobytes() + seed_bytes
        hash_bytes = blake3(message_bytes, key=key_bytes).digest()
        hash_uint32 = np.frombuffer(hash_bytes, dtype=np.uint32)
        for j in range(num_k_per_message):
            # If no sign bit, truncate to [0, R)
            if k_offset + j < k:
                u = hash_uint32[j]
                k0 = u & (R - 1)
                k1 = k0 ^ (1 + np_mul_hi_u32(np.uint32(R - 1), u))
                E[k_offset + j, 0] = k0 % R
                E[k_offset + j, 1] = k1 % R


def noise_gen_ref_sparse(
    E,  # (k, R), int8
    seed,  #   (32), uint8
    key,  #   (32), uint8
    R: int = 128,
):
    k = E.shape[0]
    E_idx = np.empty((k, 2), dtype=np.uint8)
    noise_gen_ref_index(E_idx, seed, key, R)

    E.fill(0)
    for i in range(k):
        E[i, E_idx[i, 0]] = 1
        E[i, E_idx[i, 1]] = -1
