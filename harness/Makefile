PREFIX     ?= /usr/local
LIBDIR     ?= $(PREFIX)/lib
INCLUDEDIR ?= $(PREFIX)/include
PKGCONFIGDIR ?= $(LIBDIR)/pkgconfig

CC         ?= cc
CXX        ?= c++
CFLAGS     += -fsanitize-coverage=trace-pc-guard -O2
CXXFLAGS   += -fsanitize-coverage=trace-pc-guard -std=c++23 -O2
VERSION    := 0.0.1

.PHONY: all install clean

all: libcrossfuzz_c.a libcrossfuzz_cpp.a

c/crossfuzz.o: c/crossfuzz.c c/crossfuzz.h
	$(CC) $(CFLAGS) -I c -c c/crossfuzz.c -o c/crossfuzz.o

libcrossfuzz_c.a: c/crossfuzz.o
	$(AR) rcs $@ $^

cpp/crossfuzz.o: cpp/crossfuzz.cpp cpp/crossfuzz.hpp c/crossfuzz.h
	$(CXX) $(CXXFLAGS) -I c -I cpp -c cpp/crossfuzz.cpp -o cpp/crossfuzz.o

libcrossfuzz_cpp.a: cpp/crossfuzz.o
	$(AR) rcs $@ $^

install: all
	install -d $(LIBDIR) $(INCLUDEDIR)/crossfuzz $(PKGCONFIGDIR)
	install -m 644 libcrossfuzz_c.a   $(LIBDIR)/
	install -m 644 libcrossfuzz_cpp.a $(LIBDIR)/
	install -m 644 c/crossfuzz.h      $(INCLUDEDIR)/crossfuzz/
	install -m 644 cpp/crossfuzz.hpp  $(INCLUDEDIR)/crossfuzz/
	sed -e 's|@CMAKE_INSTALL_PREFIX@|$(PREFIX)|g' \
	    -e 's|@CMAKE_INSTALL_FULL_LIBDIR@|$(LIBDIR)|g' \
	    -e 's|@CMAKE_INSTALL_FULL_INCLUDEDIR@|$(INCLUDEDIR)|g' \
	    -e 's|@PROJECT_VERSION@|$(VERSION)|g' \
	    c/crossfuzz-c.pc.in   > $(PKGCONFIGDIR)/crossfuzz-c.pc
	sed -e 's|@CMAKE_INSTALL_PREFIX@|$(PREFIX)|g' \
	    -e 's|@CMAKE_INSTALL_FULL_LIBDIR@|$(LIBDIR)|g' \
	    -e 's|@CMAKE_INSTALL_FULL_INCLUDEDIR@|$(INCLUDEDIR)|g' \
	    -e 's|@PROJECT_VERSION@|$(VERSION)|g' \
	    cpp/crossfuzz-cpp.pc.in > $(PKGCONFIGDIR)/crossfuzz-cpp.pc

clean:
	rm -f c/crossfuzz.o cpp/crossfuzz.o libcrossfuzz_c.a libcrossfuzz_cpp.a
