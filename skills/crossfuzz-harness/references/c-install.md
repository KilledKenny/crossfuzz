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

