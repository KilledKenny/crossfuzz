# Installing the C / C++ harness

## Clone

```sh
git clone https://github.com/KilledKenny/crossfuzz.git
cd crossfuzz
```

## Install

### CMake

```sh
cmake -B harness/build -S harness \
    -DCMAKE_C_COMPILER=clang \
    -DCMAKE_CXX_COMPILER=clang++ \
    -DCMAKE_INSTALL_PREFIX=/usr/local \
    -DCMAKE_BUILD_TYPE=Release
cmake --build harness/build
cmake --install harness/build
```

### Plain Makefile (no cmake required)

```sh
make -C harness install CC=clang CXX=clang++ PREFIX=/usr/local
```

## Verify

```sh
pkg-config --modversion crossfuzz-c
pkg-config --modversion crossfuzz-cpp
```

Both should print a version string. If you get `Package crossfuzz-c was not found`, the install prefix isn't on `PKG_CONFIG_PATH` — either re-run install with a prefix your toolchain already searches (e.g. `/usr/local`), or export `PKG_CONFIG_PATH=<prefix>/lib/pkgconfig`.

