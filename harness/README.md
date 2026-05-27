# crossfuzz C and C++ harnesses

The C and C++ harnesses are packaged for CMake (`find_package` / `add_subdirectory`) and for
automake/pkg-config projects via a plain `make install`.

Coverage instrumentation requires **Clang** — the harness uses
`-fsanitize-coverage=trace-pc-guard` callbacks that are a Clang/LLVM feature.

---

## Installing

### CMake

```sh
cmake -B build -S harness \
    -DCMAKE_C_COMPILER=clang \
    -DCMAKE_CXX_COMPILER=clang++ \
    -DCMAKE_INSTALL_PREFIX=/usr/local \
    -DCMAKE_BUILD_TYPE=Release
cmake --build build
cmake --install build
```

This installs:

| Path | Content |
|------|---------|
| `lib/libcrossfuzz_c.a` | C harness static library |
| `lib/libcrossfuzz_cpp.a` | C++ harness static library |
| `include/crossfuzz/crossfuzz.h` | C header |
| `include/crossfuzz/crossfuzz.hpp` | C++ header |
| `lib/cmake/crossfuzz/` | CMake config files for `find_package` |
| `lib/pkgconfig/crossfuzz-c.pc` | pkg-config file for C harness |
| `lib/pkgconfig/crossfuzz-cpp.pc` | pkg-config file for C++ harness |

### Plain Makefile (automake / manual projects)

```sh
make -C harness install \
    CC=clang CXX=clang++ \
    PREFIX=/usr/local
```

Installs the same libraries, headers, and pkg-config files as above.
No cmake required.

---

## Using in a CMake project

### After installing (`find_package`)

```cmake
find_package(crossfuzz REQUIRED)

add_executable(my_target my_target.c)
target_link_libraries(my_target PRIVATE crossfuzz::c)
```

For C++:
```cmake
find_package(crossfuzz REQUIRED)

add_executable(my_target my_target.cpp)
target_link_libraries(my_target PRIVATE crossfuzz::cpp)
```

Linking against `crossfuzz::c` or `crossfuzz::cpp` automatically propagates
`-fsanitize-coverage=trace-pc-guard` to your target so it gets instrumented.

To disable instrumentation (e.g. for a server-mode target where coverage comes
from a separate process):

```sh
cmake ... -DCROSSFUZZ_INSTRUMENT=OFF
```

### In-tree (`add_subdirectory`)

Copy or submodule the repo, then point cmake at the harness subdirectory:

```cmake
add_subdirectory(third_party/crossfuzz/harness/c)

add_executable(my_target my_target.c)
target_link_libraries(my_target PRIVATE crossfuzz::c)
```

For C++, use `harness/cpp` — it pulls in `harness/c` automatically:

```cmake
add_subdirectory(third_party/crossfuzz/harness/cpp)

add_executable(my_target my_target.cpp)
target_link_libraries(my_target PRIVATE crossfuzz::cpp)
```

---

## Using in an automake project

After installing, use `PKG_CHECK_MODULES` in `configure.ac`:

```m4
PKG_CHECK_MODULES([CROSSFUZZ_C], [crossfuzz-c])
```

Then in `Makefile.am`:

```makefile
my_target_CFLAGS  = $(CROSSFUZZ_C_CFLAGS)
my_target_LDADD   = $(CROSSFUZZ_C_LIBS)
```

For C++, use `crossfuzz-cpp` instead (it already pulls in `crossfuzz-c` via `Requires:`).
