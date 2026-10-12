# sm_89 (Ada) PoW GEMM. Needs nvcc >= 12.0.
NVCC ?= nvcc
ARCH ?= sm_89

liblattice_ada.so: lattice_ada.cu
	$(NVCC) -std=c++17 -arch=$(ARCH) -O3 -shared -Xcompiler -fPIC $< -o $@

clean:
	rm -f liblattice_ada.so
