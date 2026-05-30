# Builds a Docker image containing the full crossfuzz CI toolchain.
#
# Build:  nix-build nix/docker.nix
# Load:   docker load --input result
# Run:    docker run --rm -v "$PWD":/w -w /w crossfuzz-ci:latest bash -c "..."
#
# The image is pushed to GHCR by .github/workflows/ci-image.yml and consumed
# by .github/workflows/test.yml.

let
  pkgs     = import ./nixpkgs.nix;
  packages = import ./packages.nix { inherit pkgs; };

  # Merge all package /bin directories into a single tree so we can set a
  # stable, predictable PATH inside the container.
  env = pkgs.buildEnv {
    name             = "crossfuzz-ci-env";
    paths            = packages;
    pathsToLink      = [ "/bin" ];
    ignoreCollisions = true;
  };

in pkgs.dockerTools.buildLayeredImage {
  name = "crossfuzz-ci";
  tag  = "latest";

  # `env` pulls its full Nix closure into the image (all tools + their
  # runtime libs).  fakeNss + usrBinEnv provide minimal /etc and /usr/bin/env.
  contents = [
    env
    pkgs.dockerTools.fakeNss
    pkgs.dockerTools.usrBinEnv
  ];

  # Create a few paths that tools expect to exist at runtime.
  extraCommands = ''
    # /bin/sh expected by many shell scripts (#!/bin/sh)
    mkdir -p bin
    ln -sf ${pkgs.bashInteractive}/bin/bash bin/sh

    # Writable home directory (Gradle wrapper, Cargo registry, Go cache)
    mkdir -p root

    # Standard sticky /tmp
    mkdir -p tmp
    chmod 1777 tmp
  '';

  config = {
    Env = [
      # All tools live in the merged env; this is the only PATH entry needed.
      "PATH=${env}/bin"

      # TLS certificates for Go module proxy, Gradle download, cargo, git, curl…
      "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
      "NIX_SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
      "CURL_CA_BUNDLE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
      "GIT_SSL_CAINFO=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"

      # Standard home / cache locations inside the ephemeral container
      "HOME=/root"
      "GOPATH=/root/go"
      "GOCACHE=/tmp/go-cache"
      # Disable VCS stamping: the bind-mounted workspace is owned by the host
      # user, not root, so git exits 128 when Go tries to read git metadata.
      "GOFLAGS=-buildvcs=false"
    ];
  };
}
