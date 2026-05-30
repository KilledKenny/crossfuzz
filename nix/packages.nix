# Full toolchain required by `make test` + `make test-e2e`.
#
# Shared between shell.nix (local dev) and nix/docker.nix (CI image).
# Adding a tool here makes it available in both environments automatically.

{ pkgs }:

let
  # The e2e fixtures invoke `clang-19` and `clang++-19` by those exact names.
  # nixpkgs's llvmPackages_19.clang provides `clang` / `clang++` only.
  # This tiny derivation creates the versioned symlinks the fixtures expect.
  clang19Versioned = pkgs.runCommand "clang-19-versioned" {} ''
    mkdir -p $out/bin
    ln -s ${pkgs.llvmPackages_19.clang}/bin/clang   $out/bin/clang-19
    ln -s ${pkgs.llvmPackages_19.clang}/bin/clang++ $out/bin/clang++-19
  '';
in [
  # Shell / POSIX utilities
  pkgs.bashInteractive
  pkgs.coreutils
  pkgs.findutils
  pkgs.gawk
  pkgs.gnugrep
  pkgs.gnumake
  pkgs.gnused
  pkgs.which
  pkgs.git

  # Go (for `make test`, the Go harness, and the coordinator itself)
  pkgs.go

  # Java (JDK + build tools; Java harness builds via ./gradlew)
  pkgs.openjdk25
  pkgs.gradle
  pkgs.maven

  # JavaScript / TypeScript (Bun harness)
  pkgs.bun

  # Rust (harness via cargo with sancov passes)
  pkgs.rustc
  pkgs.cargo

  # Python (harness; venv is created at run time by the caller)
  pkgs.python3

  # C / C++ — LLVM 19 as hardcoded in the e2e fixture build_cmd strings
  pkgs.llvmPackages_19.clang  # provides clang, clang++, and the full clang toolchain
  clang19Versioned             # provides clang-19, clang++-19 symlinks

  # GCC (g++ / libstdc++ needed by the C++ e2e fixture's patchelf rpath step)
  pkgs.gcc

  # patchelf: used by e2e C/C++ fixtures to fix the ELF interpreter for Nix-built
  # binaries so they run correctly outside the Nix environment
  pkgs.patchelf

  # curl (used by Makefile to download gradle-wrapper.jar)
  pkgs.curl

  # TLS certificates (Go module downloads, Gradle, cargo, git, curl, …)
  pkgs.cacert
]
