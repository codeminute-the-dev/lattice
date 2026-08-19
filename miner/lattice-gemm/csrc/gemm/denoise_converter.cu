#include "denoise_converter_host.h"
#include "lattice_api_params.h"

template void run_denoise_converter<128>(LatticeAPIParams&, cudaStream_t);
template void run_denoise_converter<64>(LatticeAPIParams&, cudaStream_t);