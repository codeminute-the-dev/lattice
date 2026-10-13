// sm_89 (Ada) proof-of-work GEMM.
//
// Computes C = A * B^T for int8 A (m x k, row-major) and Bt (n x k, row-major,
// i.e. B stored K-major), accumulating in int32, and performs the PoW
// transcript hashing described by miner-base/noisy_gemm.py:
//
//   * C is partitioned into 16x16 hash tiles aligned at (16*hi, 16*wi). Only
//     tiles lying fully inside the matrix are hashed.
//   * After every full 128-wide k step, the XOR of the 256 running int32
//     accumulators of a hash tile is rotl(.,13)-xored into transcript word
//     (reduction_count % 16) of that tile.
//   * After the last k step, each tile's 16-word transcript is hashed with a
//     single keyed BLAKE3 block and compared (as a little-endian uint256)
//     with the target.
//
// Only SM80-class instructions are used (cp.async, mma.sync m16n8k32 s8), so
// this runs on Ada and Ampere. XOR is commutative, so the result does not
// depend on how accumulators are distributed across threads.

#include <cstdint>
#include <cuda_runtime.h>

namespace {

constexpr int kRank = 128;             // k step between transcript updates
constexpr int kHashTile = 16;
constexpr int kTranscriptWords = 16;
constexpr int kRowBytes = kRank;       // bytes of one k chunk per row
constexpr int kRowStride = kRank + 16; // padded smem row, conflict free
constexpr uint32_t kHashRotation = 13;

__device__ __forceinline__ uint32_t rotl32(uint32_t x, int n) {
  return (x << n) | (x >> (32 - n));
}
__device__ __forceinline__ uint32_t rotr32(uint32_t x, int n) {
  return (x << (32 - n)) | (x >> n);
}

// ---- BLAKE3, single 64-byte block, keyed, CHUNK_START|CHUNK_END|ROOT ------

__device__ __constant__ uint32_t kIV[4] = {0x6A09E667u, 0xBB67AE85u,
                                           0x3C6EF372u, 0xA54FF53Au};

#define B3_G(a, b, c, d, mx, my)         \
  do {                                   \
    v[a] = v[a] + v[b] + (mx);           \
    v[d] = rotr32(v[d] ^ v[a], 16);      \
    v[c] = v[c] + v[d];                  \
    v[b] = rotr32(v[b] ^ v[c], 12);      \
    v[a] = v[a] + v[b] + (my);           \
    v[d] = rotr32(v[d] ^ v[a], 8);       \
    v[c] = v[c] + v[d];                  \
    v[b] = rotr32(v[b] ^ v[c], 7);       \
  } while (0)

// Writes the first 8 output words (state[i] ^ state[i+8]) to out.
__device__ void blake3_keyed_single_block(const uint32_t key[8],
                                          const uint32_t msg_in[16],
                                          uint32_t out[8]) {
  constexpr uint32_t CHUNK_START = 1, CHUNK_END = 2, ROOT = 8, KEYED_HASH = 16;
  uint32_t m[16];
#pragma unroll
  for (int i = 0; i < 16; ++i) m[i] = msg_in[i];
  uint32_t v[16];
#pragma unroll
  for (int i = 0; i < 8; ++i) v[i] = key[i];
  v[8] = kIV[0];
  v[9] = kIV[1];
  v[10] = kIV[2];
  v[11] = kIV[3];
  v[12] = 0;
  v[13] = 0;
  v[14] = 64;
  v[15] = CHUNK_START | CHUNK_END | ROOT | KEYED_HASH;
#pragma unroll
  for (int r = 0; r < 7; ++r) {
    B3_G(0, 4, 8, 12, m[0], m[1]);
    B3_G(1, 5, 9, 13, m[2], m[3]);
    B3_G(2, 6, 10, 14, m[4], m[5]);
    B3_G(3, 7, 11, 15, m[6], m[7]);
    B3_G(0, 5, 10, 15, m[8], m[9]);
    B3_G(1, 6, 11, 12, m[10], m[11]);
    B3_G(2, 7, 8, 13, m[12], m[13]);
    B3_G(3, 4, 9, 14, m[14], m[15]);
    if (r < 6) {
      uint32_t t[16];
#pragma unroll
      for (int i = 0; i < 16; ++i) t[i] = m[i];
      // BLAKE3 message permutation: 2,6,3,10,7,0,4,13,1,11,12,5,9,14,15,8
      m[0] = t[2];  m[1] = t[6];  m[2] = t[3];   m[3] = t[10];
      m[4] = t[7];  m[5] = t[0];  m[6] = t[4];   m[7] = t[13];
      m[8] = t[1];  m[9] = t[11]; m[10] = t[12]; m[11] = t[5];
      m[12] = t[9]; m[13] = t[14]; m[14] = t[15]; m[15] = t[8];
    }
  }
#pragma unroll
  for (int i = 0; i < 8; ++i) out[i] = v[i] ^ v[i + 8];
}


// ---- noise generation --------------------------------------------------
// Mirrors miner-base/noise_generation.py. Draw `i` is
//   blake3_keyed(key, int32[8]{0.. with [prepend] = 1 + i} || seed)
// i.e. one 64-byte block: words 0..7 = prepend, words 8..15 = seed.

__device__ void noise_draw(const uint32_t key[8], const uint32_t seed[8],
                           int prepend, uint32_t index, uint32_t out[8]) {
  uint32_t msg[16];
#pragma unroll
  for (int i = 0; i < 8; ++i) msg[i] = 0;
  msg[prepend] = 1u + index;
#pragma unroll
  for (int i = 0; i < 8; ++i) msg[8 + i] = seed[i];
  blake3_keyed_single_block(key, msg, out);
}

// Uniform int8 matrix (rows x 128): byte j of the draw stream -> (b & 63) - 32.
__global__ void noise_uniform_kernel(const uint32_t* key, const uint32_t* seed,
                                     int8_t* out, int rows) {
  uint32_t draw = blockIdx.x * blockDim.x + threadIdx.x;
  uint32_t total = (uint32_t)rows * 128u / 32u;
  if (draw >= total) return;
  uint32_t kk[8], ss[8], h[8];
  for (int i = 0; i < 8; ++i) { kk[i] = key[i]; ss[i] = seed[i]; }
  noise_draw(kk, ss, 0, draw, h);
  uint32_t* o = reinterpret_cast<uint32_t*>(out + (size_t)draw * 32);
#pragma unroll
  for (int w = 0; w < 8; ++w) {
    uint32_t v = h[w], r = 0;
#pragma unroll
    for (int b = 0; b < 4; ++b) {
      int32_t x = (int32_t)((v >> (8 * b)) & 63u) - 32;
      r |= ((uint32_t)(x & 0xFF)) << (8 * b);
    }
    o[w] = r;
  }
}

// Sparse +1/-1 structure: for line c (c < k), p[c] gets +1 and q[c] gets -1.
__global__ void noise_perm_kernel(const uint32_t* key, const uint32_t* seed,
                                  int32_t* p, int32_t* q, int k) {
  uint32_t draw = blockIdx.x * blockDim.x + threadIdx.x;
  uint32_t total = ((uint32_t)k * 4u + 31u) / 32u;
  if (draw >= total) return;
  uint32_t kk[8], ss[8], h[8];
  for (int i = 0; i < 8; ++i) { kk[i] = key[i]; ss[i] = seed[i]; }
  noise_draw(kk, ss, 1, draw, h);
#pragma unroll
  for (int j = 0; j < 8; ++j) {
    uint32_t c = draw * 8u + j;
    if (c >= (uint32_t)k) break;
    uint32_t r = h[j];
    uint32_t first = r & 127u;
    uint32_t mulhi = (uint32_t)(((uint64_t)127u * r) >> 32);
    uint32_t second = first ^ (1u + mulhi);
    p[c] = (int32_t)first;
    q[c] = (int32_t)second;
  }
}

// out[i, c] = X[i, c] + L[i, p[c]] - L[i, q[c]]   (X: rows x k, L: rows x 128)
__global__ void apply_noise_kernel(const int8_t* X, const int8_t* L,
                                   const int32_t* p, const int32_t* q,
                                   int8_t* out, int rows, int k) {
  size_t idx = (size_t)blockIdx.x * blockDim.x + threadIdx.x;
  size_t total = (size_t)rows * k;
  if (idx >= total) return;
  int i = (int)(idx / k);
  int c = (int)(idx % k);
  int v = (int)X[idx] + (int)L[(size_t)i * 128 + p[c]] - (int)L[(size_t)i * 128 + q[c]];
  out[idx] = (int8_t)v;
}


// ---- BLAKE3 keyed tree hash of a byte buffer ----------------------------
// The Merkle root of a matrix is exactly BLAKE3(key, data zero-padded to a
// multiple of 1024 bytes). One thread hashes one 1 KB chunk; parents are
// combined level by level, an odd node at the end of a level is promoted
// unchanged (this reproduces BLAKE3's left-full binary tree).

__constant__ uint32_t kIVfull[8] = {0x6A09E667u, 0xBB67AE85u, 0x3C6EF372u,
                                    0xA54FF53Au, 0x510E527Fu, 0x9B05688Cu,
                                    0x1F83D9ABu, 0x5BE0CD19u};

__device__ void blake3_compress_cv(const uint32_t cv[8], const uint32_t msg_in[16],
                                   uint32_t counter, uint32_t block_len,
                                   uint32_t flags, uint32_t out[8]) {
  uint32_t m[16];
#pragma unroll
  for (int i = 0; i < 16; ++i) m[i] = msg_in[i];
  uint32_t v[16];
#pragma unroll
  for (int i = 0; i < 8; ++i) v[i] = cv[i];
  v[8] = kIVfull[0];
  v[9] = kIVfull[1];
  v[10] = kIVfull[2];
  v[11] = kIVfull[3];
  v[12] = counter;
  v[13] = 0;
  v[14] = block_len;
  v[15] = flags;
#pragma unroll
  for (int r = 0; r < 7; ++r) {
    B3_G(0, 4, 8, 12, m[0], m[1]);
    B3_G(1, 5, 9, 13, m[2], m[3]);
    B3_G(2, 6, 10, 14, m[4], m[5]);
    B3_G(3, 7, 11, 15, m[6], m[7]);
    B3_G(0, 5, 10, 15, m[8], m[9]);
    B3_G(1, 6, 11, 12, m[10], m[11]);
    B3_G(2, 7, 8, 13, m[12], m[13]);
    B3_G(3, 4, 9, 14, m[14], m[15]);
    if (r < 6) {
      uint32_t t[16];
#pragma unroll
      for (int i = 0; i < 16; ++i) t[i] = m[i];
      m[0] = t[2];  m[1] = t[6];  m[2] = t[3];   m[3] = t[10];
      m[4] = t[7];  m[5] = t[0];  m[6] = t[4];   m[7] = t[13];
      m[8] = t[1];  m[9] = t[11]; m[10] = t[12]; m[11] = t[5];
      m[12] = t[9]; m[13] = t[14]; m[14] = t[15]; m[15] = t[8];
    }
  }
#pragma unroll
  for (int i = 0; i < 8; ++i) out[i] = v[i] ^ v[i + 8];
}

constexpr uint32_t F_CHUNK_START = 1, F_CHUNK_END = 2, F_PARENT = 4, F_ROOT = 8,
                   F_KEYED = 16;

// data: `len` bytes; chunks past len are zero. Writes one CV per chunk.
__global__ void b3_chunk_kernel(const uint8_t* __restrict__ data, size_t len,
                                const uint32_t* __restrict__ key,
                                uint32_t* __restrict__ cvs, uint32_t num_chunks) {
  uint32_t chunk = blockIdx.x * blockDim.x + threadIdx.x;
  if (chunk >= num_chunks) return;
  uint32_t cv[8];
#pragma unroll
  for (int i = 0; i < 8; ++i) cv[i] = key[i];
  const size_t base = (size_t)chunk * 1024;
  for (int b = 0; b < 16; ++b) {
    uint32_t msg[16];
    const size_t off = base + (size_t)b * 64;
    if (off + 64 <= len) {
      const uint4* src = reinterpret_cast<const uint4*>(data + off);
#pragma unroll
      for (int i = 0; i < 4; ++i) {
        uint4 w = src[i];
        msg[4 * i] = w.x; msg[4 * i + 1] = w.y; msg[4 * i + 2] = w.z; msg[4 * i + 3] = w.w;
      }
    } else {
      // partial or fully padded block: byte-wise with zero fill
      uint8_t tmp[64];
      for (int i = 0; i < 64; ++i) tmp[i] = (off + i < len) ? data[off + i] : 0;
#pragma unroll
      for (int i = 0; i < 16; ++i)
        msg[i] = (uint32_t)tmp[4 * i] | ((uint32_t)tmp[4 * i + 1] << 8) |
                 ((uint32_t)tmp[4 * i + 2] << 16) | ((uint32_t)tmp[4 * i + 3] << 24);
    }
    uint32_t flags = F_KEYED;
    if (b == 0) flags |= F_CHUNK_START;
    if (b == 15) {
      flags |= F_CHUNK_END;
      if (num_chunks == 1) flags |= F_ROOT;
    }
    uint32_t out[8];
    blake3_compress_cv(cv, msg, chunk, 64, flags, out);
#pragma unroll
    for (int i = 0; i < 8; ++i) cv[i] = out[i];
  }
#pragma unroll
  for (int i = 0; i < 8; ++i) cvs[(size_t)chunk * 8 + i] = cv[i];
}

// One level: parent i = compress(key, cv[2i] || cv[2i+1]); odd tail promoted.
__global__ void b3_parent_kernel(const uint32_t* __restrict__ in,
                                 uint32_t* __restrict__ out,
                                 const uint32_t* __restrict__ key, uint32_t n_in,
                                 bool is_root) {
  uint32_t i = blockIdx.x * blockDim.x + threadIdx.x;
  uint32_t n_out = (n_in + 1) / 2;
  if (i >= n_out) return;
  if (2 * i + 1 >= n_in) {  // odd one out
#pragma unroll
    for (int w = 0; w < 8; ++w) out[(size_t)i * 8 + w] = in[(size_t)(2 * i) * 8 + w];
    return;
  }
  uint32_t msg[16], cv[8], res[8];
#pragma unroll
  for (int w = 0; w < 16; ++w) msg[w] = in[(size_t)(2 * i) * 8 + w];
#pragma unroll
  for (int w = 0; w < 8; ++w) cv[w] = key[w];
  uint32_t flags = F_PARENT | F_KEYED | (is_root ? F_ROOT : 0);
  blake3_compress_cv(cv, msg, 0, 64, flags, res);
#pragma unroll
  for (int w = 0; w < 8; ++w) out[(size_t)i * 8 + w] = res[w];
}

// hash <= target as little-endian uint256 (word 7 most significant).
__device__ bool le_uint256(const uint32_t h[8], const uint32_t* target) {
  for (int i = 7; i >= 0; --i) {
    if (h[i] > target[i]) return false;
    if (h[i] < target[i]) return true;
  }
  return true;
}

// ---- tensor core / async copy helpers --------------------------------------

__device__ __forceinline__ void cp_async16(void* smem, const void* gmem,
                                           bool valid) {
  unsigned s = static_cast<unsigned>(__cvta_generic_to_shared(smem));
  int src_size = valid ? 16 : 0;  // 0 => zero fill
  asm volatile("cp.async.cg.shared.global [%0], [%1], 16, %2;\n" ::"r"(s),
               "l"(gmem), "r"(src_size));
}
__device__ __forceinline__ void cp_async_commit() {
  asm volatile("cp.async.commit_group;\n" ::);
}
template <int N>
__device__ __forceinline__ void cp_async_wait() {
  asm volatile("cp.async.wait_group %0;\n" ::"n"(N));
}

__device__ __forceinline__ void mma_s8_16x8x32(int32_t c[4], const uint32_t a[4],
                                               const uint32_t b[2]) {
  asm volatile(
      "mma.sync.aligned.m16n8k32.row.col.s32.s8.s8.s32 "
      "{%0,%1,%2,%3}, {%4,%5,%6,%7}, {%8,%9}, {%0,%1,%2,%3};\n"
      : "+r"(c[0]), "+r"(c[1]), "+r"(c[2]), "+r"(c[3])
      : "r"(a[0]), "r"(a[1]), "r"(a[2]), "r"(a[3]), "r"(b[0]), "r"(b[1]));
}

// ---- the kernel ------------------------------------------------------------

// Each warp owns WTM x WTN hash tiles (16x16 each) for the whole k loop.
// A block is WARPS_M x WARPS_N warps.
template <int WARPS_M, int WARPS_N, int WTM, int WTN>
struct Cfg {
  static constexpr int kThreads = WARPS_M * WARPS_N * 32;
  static constexpr int kBM = WARPS_M * WTM * kHashTile;
  static constexpr int kBN = WARPS_N * WTN * kHashTile;
  static constexpr int kStageBytes = (kBM + kBN) * kRowStride;
};

struct FoundSlot {
  int32_t flag;  // 0 = none, 1 = claimed, 2 = written
  int32_t row;
  int32_t col;
};

template <int WARPS_M, int WARPS_N, int WTM, int WTN>
__global__ void __launch_bounds__(Cfg<WARPS_M, WARPS_N, WTM, WTN>::kThreads)
    pow_gemm_kernel(const int8_t* __restrict__ A, const int8_t* __restrict__ Bt,
                    int32_t* __restrict__ C, int m, int n, int k,
                    const uint32_t* __restrict__ key,
                    const uint32_t* __restrict__ target,
                    FoundSlot* __restrict__ found,
                    uint32_t* __restrict__ transcripts_out, int tiles_w) {
  using S = Cfg<WARPS_M, WARPS_N, WTM, WTN>;
  extern __shared__ __align__(16) int8_t smem[];

  const int tid = threadIdx.x;
  const int warp = tid >> 5;
  const int lane = tid & 31;
  const int warp_m = warp / WARPS_N;
  const int warp_n = warp % WARPS_N;
  const int gid = lane >> 2;
  const int tig = lane & 3;

  const int block_row0 = blockIdx.y * S::kBM;
  const int block_col0 = blockIdx.x * S::kBN;
  const int warp_row0 = block_row0 + warp_m * WTM * kHashTile;
  const int warp_col0 = block_col0 + warp_n * WTN * kHashTile;

  const int num_chunks = (k + kRank - 1) / kRank;
  const int full_chunks = k / kRank;

  // Stage loader: rows of A then rows of Bt, 128 bytes of k per row.
  auto load_stage = [&](int stage, int chunk) {
    int8_t* sA = smem + stage * S::kStageBytes;
    int8_t* sB = sA + S::kBM * kRowStride;
    const int k0 = chunk * kRank;
    constexpr int kGranules = kRowBytes / 16;
    for (int idx = tid; idx < (S::kBM + S::kBN) * kGranules; idx += S::kThreads) {
      int row = idx / kGranules;
      int g = idx % kGranules;
      int kk = k0 + g * 16;
      bool in_k = kk < k;
      if (row < S::kBM) {
        int grow = block_row0 + row;
        bool ok = in_k && grow < m;
        const int8_t* src = ok ? A + (size_t)grow * k + kk : A;
        cp_async16(sA + row * kRowStride + g * 16, src, ok);
      } else {
        int brow = row - S::kBM;
        int gcol = block_col0 + brow;
        bool ok = in_k && gcol < n;
        const int8_t* src = ok ? Bt + (size_t)gcol * k + kk : Bt;
        cp_async16(sB + brow * kRowStride + g * 16, src, ok);
      }
    }
    cp_async_commit();
  };

  int32_t acc[WTM][WTN][2][4];
#pragma unroll
  for (int i = 0; i < WTM; ++i)
#pragma unroll
    for (int j = 0; j < WTN; ++j)
#pragma unroll
      for (int h = 0; h < 2; ++h)
#pragma unroll
        for (int r = 0; r < 4; ++r) acc[i][j][h][r] = 0;

  // Transcript word `lane` of every hash tile this warp owns (lanes >= 16 idle).
  uint32_t transcript[WTM][WTN];
#pragma unroll
  for (int i = 0; i < WTM; ++i)
#pragma unroll
    for (int j = 0; j < WTN; ++j) transcript[i][j] = 0;

  load_stage(0, 0);

  for (int chunk = 0; chunk < num_chunks; ++chunk) {
    if (chunk + 1 < num_chunks) {
      load_stage((chunk + 1) & 1, chunk + 1);
      cp_async_wait<1>();
    } else {
      cp_async_wait<0>();
    }
    __syncthreads();

    const int8_t* sA = smem + (chunk & 1) * S::kStageBytes;
    const int8_t* sB = sA + S::kBM * kRowStride;

#pragma unroll
    for (int ks = 0; ks < kRank / 32; ++ks) {
      uint32_t afrag[WTM][4];
#pragma unroll
      for (int i = 0; i < WTM; ++i) {
        const int8_t* base =
            sA + (warp_m * WTM * kHashTile + i * kHashTile + gid) * kRowStride +
            ks * 32 + tig * 4;
        afrag[i][0] = *reinterpret_cast<const uint32_t*>(base);
        afrag[i][1] = *reinterpret_cast<const uint32_t*>(base + 8 * kRowStride);
        afrag[i][2] = *reinterpret_cast<const uint32_t*>(base + 16);
        afrag[i][3] =
            *reinterpret_cast<const uint32_t*>(base + 8 * kRowStride + 16);
      }
#pragma unroll
      for (int j = 0; j < WTN; ++j) {
#pragma unroll
        for (int h = 0; h < 2; ++h) {
          const int8_t* base =
              sB + (warp_n * WTN * kHashTile + j * kHashTile + h * 8 + gid) *
                       kRowStride +
              ks * 32 + tig * 4;
          uint32_t bfrag[2];
          bfrag[0] = *reinterpret_cast<const uint32_t*>(base);
          bfrag[1] = *reinterpret_cast<const uint32_t*>(base + 16);
#pragma unroll
          for (int i = 0; i < WTM; ++i) mma_s8_16x8x32(acc[i][j][h], afrag[i], bfrag);
        }
      }
    }

    if (chunk < full_chunks) {
      const int idx = chunk % kTranscriptWords;
#pragma unroll
      for (int i = 0; i < WTM; ++i) {
#pragma unroll
        for (int j = 0; j < WTN; ++j) {
          uint32_t x = 0;
#pragma unroll
          for (int h = 0; h < 2; ++h)
#pragma unroll
            for (int r = 0; r < 4; ++r) x ^= (uint32_t)acc[i][j][h][r];
#pragma unroll
          for (int off = 16; off > 0; off >>= 1) x ^= __shfl_xor_sync(0xffffffffu, x, off);
          if (lane == idx) transcript[i][j] = rotl32(transcript[i][j], kHashRotation) ^ x;
        }
      }
    }
    __syncthreads();
  }

  // Write C.
#pragma unroll
  for (int i = 0; i < WTM; ++i) {
#pragma unroll
    for (int j = 0; j < WTN; ++j) {
#pragma unroll
      for (int h = 0; h < 2; ++h) {
        int row = warp_row0 + i * kHashTile + gid;
        int col = warp_col0 + j * kHashTile + h * 8 + tig * 2;
        if (row < m && col + 1 < n) {
          *reinterpret_cast<int2*>(C + (size_t)row * n + col) =
              make_int2(acc[i][j][h][0], acc[i][j][h][1]);
        } else if (row < m && col < n) {
          C[(size_t)row * n + col] = acc[i][j][h][0];
        }
        if (row + 8 < m && col + 1 < n) {
          *reinterpret_cast<int2*>(C + (size_t)(row + 8) * n + col) =
              make_int2(acc[i][j][h][2], acc[i][j][h][3]);
        } else if (row + 8 < m && col < n) {
          C[(size_t)(row + 8) * n + col] = acc[i][j][h][2];
        }
      }
    }
  }

  // Transcript check. Only tiles fully inside the matrix are hashed, and only
  // if at least one full k step happened.
  if (full_chunks == 0) return;
#pragma unroll
  for (int i = 0; i < WTM; ++i) {
#pragma unroll
    for (int j = 0; j < WTN; ++j) {
      const int tr = warp_row0 + i * kHashTile;
      const int tc = warp_col0 + j * kHashTile;
      if (tr + kHashTile > m || tc + kHashTile > n) continue;  // warp-uniform

      uint32_t msg[16];
#pragma unroll
      for (int w = 0; w < 16; ++w) msg[w] = __shfl_sync(0xffffffffu, transcript[i][j], w);

      if (transcripts_out != nullptr && lane < 16) {
        transcripts_out[((size_t)(tr / kHashTile) * tiles_w + (tc / kHashTile)) * 16 + lane] =
            msg[lane];
      }
      if (lane == 0 && key != nullptr && target != nullptr && found != nullptr) {
        uint32_t kk[8], tt[8], hh[8];
#pragma unroll
        for (int w = 0; w < 8; ++w) {
          kk[w] = key[w];
          tt[w] = target[w];
        }
        blake3_keyed_single_block(kk, msg, hh);
        if (le_uint256(hh, tt)) {
          if (atomicCAS(&found->flag, 0, 1) == 0) {
            found->row = tr;
            found->col = tc;
            __threadfence();
            atomicExch(&found->flag, 2);
          }
        }
      }
    }
  }
}

template <int WARPS_M, int WARPS_N, int WTM, int WTN>
cudaError_t launch(const int8_t* A, const int8_t* Bt, int32_t* C, int m, int n,
                   int k, const uint32_t* key, const uint32_t* target,
                   FoundSlot* found, uint32_t* transcripts_out, int tiles_w,
                   cudaStream_t stream) {
  using S = Cfg<WARPS_M, WARPS_N, WTM, WTN>;
  size_t smem = 2 * (size_t)S::kStageBytes;
  auto kern = pow_gemm_kernel<WARPS_M, WARPS_N, WTM, WTN>;
  cudaError_t e = cudaFuncSetAttribute(
      kern, cudaFuncAttributeMaxDynamicSharedMemorySize, (int)smem);
  if (e != cudaSuccess) return e;
  dim3 grid((n + S::kBN - 1) / S::kBN, (m + S::kBM - 1) / S::kBM);
  kern<<<grid, S::kThreads, smem, stream>>>(A, Bt, C, m, n, k, key, target,
                                            found, transcripts_out, tiles_w);
  return cudaGetLastError();
}

}  // namespace

extern "C" {

// found: device int32[3] {flag, row, col}; must be zeroed by the caller.
//        flag == 2 after the kernel means a hash tile met the target and
//        (row, col) is its top-left corner in C.
// transcripts_out: optional device uint32[(m/16)*(n/16)*16], for testing.
// config: 0 = 32x64 block (1 hash tile per warp), 1 = 64x128 block (4 per warp)
int lattice_ada_pow_gemm(const int8_t* A, const int8_t* Bt, int32_t* C, int m,
                         int n, int k, const uint32_t* key,
                         const uint32_t* target, int32_t* found,
                         uint32_t* transcripts_out, int config, void* stream) {
  if (k % 16 != 0 || m <= 0 || n <= 0 || k <= 0) return (int)cudaErrorInvalidValue;
  cudaStream_t s = static_cast<cudaStream_t>(stream);
  const int tiles_w = n / kHashTile;
  FoundSlot* f = reinterpret_cast<FoundSlot*>(found);
  cudaError_t e;
  if (config == 0)
    e = launch<2, 4, 1, 1>(A, Bt, C, m, n, k, key, target, f, transcripts_out, tiles_w, s);
  else
    e = launch<2, 4, 2, 2>(A, Bt, C, m, n, k, key, target, f, transcripts_out, tiles_w, s);
  return (int)e;
}

int lattice_ada_noise_uniform(const uint32_t* key, const uint32_t* seed,
                              int8_t* out, int rows, void* stream) {
  uint32_t total = (uint32_t)rows * 4u;
  noise_uniform_kernel<<<(total + 127) / 128, 128, 0, (cudaStream_t)stream>>>(key, seed, out, rows);
  return (int)cudaGetLastError();
}

int lattice_ada_noise_perm(const uint32_t* key, const uint32_t* seed, int32_t* p,
                           int32_t* q, int k, void* stream) {
  uint32_t total = ((uint32_t)k * 4u + 31u) / 32u;
  noise_perm_kernel<<<(total + 127) / 128, 128, 0, (cudaStream_t)stream>>>(key, seed, p, q, k);
  return (int)cudaGetLastError();
}

int lattice_ada_apply_noise(const int8_t* X, const int8_t* L, const int32_t* p,
                            const int32_t* q, int8_t* out, int rows, int k,
                            void* stream) {
  size_t total = (size_t)rows * k;
  apply_noise_kernel<<<(unsigned)((total + 255) / 256), 256, 0, (cudaStream_t)stream>>>(X, L, p, q, out, rows, k);
  return (int)cudaGetLastError();
}

// Number of uint32 words of scratch lattice_ada_blake3_keyed needs: 2 * chunks * 8.
size_t lattice_ada_blake3_scratch_words(size_t len) {
  size_t chunks = (len + 1023) / 1024;
  if (chunks == 0) chunks = 1;
  return 2 * chunks * 8;
}

// BLAKE3 keyed hash of `len` bytes (zero padded to a 1024 multiple) -> 8 words.
int lattice_ada_blake3_keyed(const uint8_t* data, size_t len, const uint32_t* key,
                             uint32_t* scratch, uint32_t* out, void* stream) {
  cudaStream_t st = static_cast<cudaStream_t>(stream);
  uint32_t chunks = (uint32_t)((len + 1023) / 1024);
  if (chunks == 0) chunks = 1;
  uint32_t* a = scratch;
  uint32_t* b = scratch + (size_t)chunks * 8;
  b3_chunk_kernel<<<(chunks + 63) / 64, 64, 0, st>>>(data, len, key, a, chunks);
  uint32_t n = chunks;
  while (n > 1) {
    uint32_t n_out = (n + 1) / 2;
    b3_parent_kernel<<<(n_out + 127) / 128, 128, 0, st>>>(a, b, key, n, n_out == 1);
    uint32_t* t = a; a = b; b = t;
    n = n_out;
  }
  cudaError_t e = cudaMemcpyAsync(out, a, 32, cudaMemcpyDeviceToDevice, st);
  if (e != cudaSuccess) return (int)e;
  return (int)cudaGetLastError();
}

}  // extern "C"
