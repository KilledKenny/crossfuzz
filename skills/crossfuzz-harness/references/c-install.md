# Installing the C / C++ harness

## Clone

```sh
git clone https://github.com/KilledKenny/crossfuzz.git
cd crossfuzz
```

## Install

```sh
make install-c-harnes PREFIX=/usr/local
```

## Verify

```sh
pkg-config --modversion crossfuzz-c
pkg-config --modversion crossfuzz-cpp
```

Both should print a version string. If you get `Package crossfuzz-c was not found`, the install prefix isn't on `PKG_CONFIG_PATH` — either re-run install with a prefix your toolchain already searches (e.g. `/usr/local`), or export `PKG_CONFIG_PATH=<prefix>/lib/pkgconfig`.

