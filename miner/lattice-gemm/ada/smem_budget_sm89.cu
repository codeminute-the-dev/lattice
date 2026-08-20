// Shared-memory budget for the Ada (sm_89) port.
//
// Ada allows 101376 B of shared memory per block; Hopper allows 233472 B. The
// production kernel's SharedStorage does not fit, so something has to give.
// This works out what, using the real layouts from ../csrc/gemm/kernel_traits.hpp
// rather than an estimate, and checks that the regrouping proposed below
// actually fits.
//
// The finding: A/B staging is not the problem. The denoise factors are. All
// four of them are held simultaneously -- 192 KB at the production tile --
// even though collective_epilogue.hpp::denoise() consumes them as two
// sequential GEMMs that never overlap. Splitting them into two phases halves
// the requirement.
//
// It is a host program; no GPU needed.
//
//   nvcc -std=c++17 -x cu -arch=sm_90a -w --expt-relaxed-constexpr \
//     -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
//     -I csrc ada/smem_budget_sm89.cu -o /tmp/smem_budget && /tmp/smem_budget
//
// (sm_90a because it instantiates the production Hopper traits to read their
//  layouts. Nothing is launched.)

#include <cstdio>
#include <algorithm>

#include "cute/tensor.hpp"
#include "cutlass/numeric_types.h"

#include "gemm/kernel_traits.hpp"

using namespace cute;

static constexpr size_t kAdaSmemPerBlock = 101376;     // sm_89 opt-in
static constexpr size_t kHopperSmemPerBlock = 233472;  // sm_90a opt-in

static int failures = 0;

struct Budget {
  size_t ab_per_stage;
  size_t ab_total;
  size_t c;
  size_t denoise_phase_ea;   // EAL x EARxBpEB
  size_t denoise_phase_axeb; // AxEBL x EBR
  size_t scales;
  size_t barriers;

  size_t production_total() const {
    // csrc/gemm/kernel_traits.hpp: all four denoise buffers coexist.
    size_t const shared = std::max({ab_total, c, denoise_phase_ea + denoise_phase_axeb});
    return shared + scales + barriers;
  }
  size_t two_phase_total() const {
    // Proposed: the two denoise GEMMs are already sequential, so their operands
    // can share storage.
    size_t const shared =
        std::max({ab_total, c, denoise_phase_ea, denoise_phase_axeb});
    return shared + scales + barriers;
  }
};

template <int bK, int Stages>
static Budget budget() {
  using KTraits =
      lattice::KernelTraits<int8_t, cutlass::bfloat16_t, cutlass::half_t, float,
                            Shape<_128, _256, Int<bK>, _128>,
                            /*Is_Even_M=*/true, /*Is_Even_N=*/true,
                            /*cM=*/1, /*cN=*/1, /*SkipReduction=*/false,
                            /*SkipDenoising=*/false, Stages, /*EnableDebug=*/false>;

  auto bytes = [](auto layout, size_t element_bytes) {
    return cosize(layout) * element_bytes;
  };
  constexpr size_t kIn = sizeof(int8_t);
  constexpr size_t kOut = sizeof(cutlass::bfloat16_t);
  constexpr size_t kDen = sizeof(cutlass::half_t);

  size_t const ab_total = bytes(typename KTraits::SmemLayoutA{}, kIn) +
                          bytes(typename KTraits::SmemLayoutB{}, kIn);

  return Budget{
      .ab_per_stage = ab_total / Stages,
      .ab_total = ab_total,
      .c = bytes(typename KTraits::SmemLayoutC{}, kOut),
      .denoise_phase_ea = bytes(typename KTraits::SmemLayoutEAL{}, kDen) +
                          bytes(typename KTraits::SmemLayoutEARxBpEB{}, kDen),
      .denoise_phase_axeb = bytes(typename KTraits::SmemLayoutAxEBL{}, kDen) +
                            bytes(typename KTraits::SmemLayoutEBR{}, kDen),
      .scales = bytes(typename KTraits::SmemLayoutScaleA{}, sizeof(float)) +
                bytes(typename KTraits::SmemLayoutScaleB{}, sizeof(float)),
      // One mbarrier pair per mainloop stage, plus the two denoise pipelines.
      .barriers = 16 * Stages + 2 * 16,
  };
}

static double kb(size_t bytes) { return bytes / 1024.0; }

template <int bK, int Stages>
static void report(char const* note) {
  Budget const b = budget<bK, Stages>();
  printf("  bK=%-3d stages=%d  A+B %5.1f KB (%4.1f/stage)  as shipped %6.1f KB   two-phase %6.1f KB  %s\n",
         bK, Stages, kb(b.ab_total), kb(b.ab_per_stage),
         kb(b.production_total()), kb(b.two_phase_total()), note);
}

template <int bK, int Stages>
static void require_fits() {
  Budget const b = budget<bK, Stages>();
  if (b.two_phase_total() > kAdaSmemPerBlock) {
    printf("  FAIL: bK=%d stages=%d needs %zu B, Ada allows %zu B\n", bK, Stages,
           b.two_phase_total(), kAdaSmemPerBlock);
    ++failures;
  }
}

int main() {
  printf("Shared memory per block: Ada %zu B (%.1f KB), Hopper %zu B (%.1f KB)\n\n",
         kAdaSmemPerBlock, kb(kAdaSmemPerBlock), kHopperSmemPerBlock,
         kb(kHopperSmemPerBlock));

  Budget const production = budget<128, 3>();
  printf("Production tile 128x256x128, 3 stages, rank 128:\n");
  printf("  A+B staging          %6.1f KB\n", kb(production.ab_total));
  printf("  C                    %6.1f KB\n", kb(production.c));
  printf("  denoise EAL x EARxBpEB %4.1f KB\n", kb(production.denoise_phase_ea));
  printf("  denoise AxEBL x EBR    %4.1f KB\n", kb(production.denoise_phase_axeb));
  printf("  scales                 %4.1f KB\n", kb(production.scales));
  printf("  total                %6.1f KB  -- %s on Ada\n\n",
         kb(production.production_total()),
         production.production_total() <= kAdaSmemPerBlock ? "fits" : "does not fit");

  printf("Candidate Ada tilings (M and N are pinned by the accumulator layout,\n"
         "see layout_equiv_sm89.cu, so only bK and the stage count can move):\n");
  report<128, 3>("production");
  report<128, 2>("");
  report<64, 4>("<- chosen");
  report<64, 3>("");
  report<64, 2>("");
  report<32, 6>("");
  report<32, 4>("");
  printf("\n");

  require_fits<64, 4>();
  require_fits<64, 3>();
  require_fits<32, 6>();

  Budget const chosen = budget<64, 4>();
  printf("Chosen: bK=64, 4 stages, two-phase denoise -> %zu B of %zu B (%.1f%% used,"
         " %zu B spare)\n",
         chosen.two_phase_total(), kAdaSmemPerBlock,
         100.0 * chosen.two_phase_total() / kAdaSmemPerBlock,
         kAdaSmemPerBlock - chosen.two_phase_total());

  if (failures == 0) {
    printf("\nPASS: the proposed Ada layout fits in sm_89 shared memory\n");
    return 0;
  }
  printf("\nFAIL: %d check(s) failed\n", failures);
  return 1;
}
