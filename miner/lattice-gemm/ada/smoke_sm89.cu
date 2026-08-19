// Smoke test for the Ada (sm_89) backend.
//
// The production kernels target sm_90a and are built on Hopper-only hardware
// (TMA, warpgroup MMA). Before rewriting them for Ada, this establishes that
// the intended replacement actually runs on this GPU: an SM80-class int8
// tensor-core GEMM, int8 x int8 -> int32, which is the arithmetic the
// NoisyGEMM mainloop performs.
//
// It checks the result against a CPU reference, because "it ran" and "it is
// correct" are different claims and only the second one matters here.

#include <cstdio>
#include <cstdlib>
#include <vector>

#include "cutlass/cutlass.h"
#include "cutlass/gemm/device/gemm.h"
#include "cutlass/util/host_tensor.h"

using ElementIn = int8_t;
using ElementAcc = int32_t;

// Row-major A, column-major B, row-major C — the TN layout the int8 tensor
// core path requires.
using Gemm = cutlass::gemm::device::Gemm<
    ElementIn, cutlass::layout::RowMajor,
    ElementIn, cutlass::layout::ColumnMajor,
    ElementAcc, cutlass::layout::RowMajor,
    ElementAcc,
    cutlass::arch::OpClassTensorOp,
    cutlass::arch::Sm80,
    cutlass::gemm::GemmShape<128, 128, 64>,
    cutlass::gemm::GemmShape<64, 64, 64>,
    cutlass::gemm::GemmShape<16, 8, 32>>;

int main() {
  int M = 512, N = 512, K = 256;

  cutlass::HostTensor<ElementIn, cutlass::layout::RowMajor> A({M, K});
  cutlass::HostTensor<ElementIn, cutlass::layout::ColumnMajor> B({K, N});
  cutlass::HostTensor<ElementAcc, cutlass::layout::RowMajor> C({M, N});

  srand(1234);
  for (int i = 0; i < M * K; ++i) A.host_data()[i] = ElementIn(rand() % 255 - 127);
  for (int i = 0; i < K * N; ++i) B.host_data()[i] = ElementIn(rand() % 255 - 127);
  for (int i = 0; i < M * N; ++i) C.host_data()[i] = 0;

  A.sync_device(); B.sync_device(); C.sync_device();

  Gemm gemm;
  Gemm::Arguments args({M, N, K},
                       {A.device_data(), A.layout().stride(0)},
                       {B.device_data(), B.layout().stride(0)},
                       {C.device_data(), C.layout().stride(0)},
                       {C.device_data(), C.layout().stride(0)},
                       {1, 0});

  cutlass::Status st = gemm(args);
  if (st != cutlass::Status::kSuccess) {
    printf("FAIL: cutlass status %d (%s)\n", int(st), cutlass::cutlassGetStatusString(st));
    return 1;
  }
  if (cudaDeviceSynchronize() != cudaSuccess) {
    printf("FAIL: %s\n", cudaGetErrorString(cudaGetLastError()));
    return 1;
  }
  C.sync_host();

  // CPU reference over a sample of outputs.
  long checked = 0, bad = 0;
  for (int i = 0; i < M; i += 37) {
    for (int j = 0; j < N; j += 41) {
      ElementAcc acc = 0;
      for (int k = 0; k < K; ++k)
        acc += ElementAcc(A.host_data()[i * K + k]) * ElementAcc(B.host_data()[j * K + k]);
      if (acc != C.host_data()[i * N + j]) ++bad;
      ++checked;
    }
  }

  printf("int8 GEMM %dx%dx%d on sm_89: checked %ld outputs, %ld mismatched\n",
         M, N, K, checked, bad);
  printf(bad == 0 ? "PASS: SM80 int8 tensor-core path is correct on this GPU\n"
                  : "FAIL: results do not match the reference\n");
  return bad == 0 ? 0 : 1;
}
