CC = /usr/bin/gcc
CXX = /usr/bin/g++
CFLAGS = -Wall -g -O3 -Wextra -Wpedantic
CXXFLAGS = -Wall -g -O3 -Wextra -std=c++11
LDLIBS =

SOURCES = external/params.c external/hash.c external/fips202.c external/hash_address.c \
          external/wots.c external/xmss_core.c external/xmss_commons.c external/utils.c
HEADERS = external/params.h external/hash.h external/fips202.h external/hash_address.h \
          external/wots.h external/xmss_core.h external/xmss_commons.h external/utils.h

TESTS = external/test/wots \
		external/test/oid \
		external/test/speed \
		external/test/vectors \
		external/test/xmss_determinism \
		external/test/maxsigsxmss

UI = external/ui/xmss_keypair \
	 external/ui/xmss_sign \
	 external/ui/xmss_open

all: lib

# Static library for Go FFI
libxmss.a: $(SOURCES) $(HEADERS) src/xmss.cpp xmss.h
	@mkdir -p /tmp/xmss_build_$$$$; \
	for src in $(SOURCES); do \
		$(CC) $(CFLAGS) -c -o /tmp/xmss_build_$$$$/`basename $$src .c`.o $$src; \
	done; \
	$(CXX) $(CXXFLAGS) -c -o /tmp/xmss_build_$$$$/xmss.o src/xmss.cpp; \
	$(AR) rcs $@ /tmp/xmss_build_$$$$/*.o; \
	rm -rf /tmp/xmss_build_$$$$

lib: libxmss.a

example: src/example
	@src/example

tests: $(TESTS)
ui: $(UI)

test: $(TESTS:=.exec)

.PHONY: clean test example lib all tests ui

external/test/%.exec: external/test/%
	@$<

# Tests compile directly without intermediate .o files
external/test/xmss_determinism: external/test/xmss_determinism.c $(SOURCES) $(HEADERS)
	$(CC) $(CFLAGS) -o $@ $(SOURCES) $< $(LDLIBS)

external/test/speed: external/test/speed.c $(SOURCES) $(HEADERS)
	$(CC) $(CFLAGS) -o $@ $(SOURCES) $< $(LDLIBS)

external/test/maxsigsxmss: external/test/xmss_max_signatures.c $(SOURCES) $(HEADERS)
	$(CC) $(CFLAGS) -o $@ $(SOURCES) $< $(LDLIBS)

external/test/%: external/test/%.c $(SOURCES) $(HEADERS)
	$(CC) $(CFLAGS) -o $@ $(SOURCES) $< $(LDLIBS)

external/ui/xmss_%: external/ui/%.c $(SOURCES) $(HEADERS)
	$(CC) $(CFLAGS) -o $@ $(SOURCES) $< $(LDLIBS)

src/example: src/example.cpp src/xmss.cpp xmss.h $(SOURCES) $(HEADERS)
	$(CXX) $(CXXFLAGS) -o $@ $(SOURCES) src/xmss.cpp src/example.cpp $(LDLIBS)

clean:
	-$(RM) $(TESTS)
	-$(RM) external/test/vectors
	-$(RM) $(UI)
	-$(RM) src/example
	-$(RM) libxmss.a
