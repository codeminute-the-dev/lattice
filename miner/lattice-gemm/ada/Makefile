# PoW GEMM for every NVIDIA GPU from Pascal (sm_61) up. Needs nvcc >= 12.0.
# The tensor core kernel is picked on sm_80+, the dp4a one below that.
NVCC ?= nvcc
GENCODE ?= -gencode arch=compute_61,code=sm_61 \
           -gencode arch=compute_75,code=sm_75 \
           -gencode arch=compute_80,code=sm_80 \
           -gencode arch=compute_86,code=sm_86 \
           -gencode arch=compute_89,code=sm_89 \
           -gencode arch=compute_89,code=compute_89

liblattice_ada.so: lattice_ada.cu
	$(NVCC) -std=c++17 $(GENCODE) -O3 -shared -Xcompiler -fPIC $< -o $@

clean:
	rm -f liblattice_ada.so
