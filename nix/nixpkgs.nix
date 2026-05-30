# Resolve nixpkgs from the user's (or CI's) nixpkgs-unstable channel.
# No pin — the image is rebuilt explicitly when the toolchain definition
# changes, keeping everything on the latest rolling nixpkgs-unstable.
import <nixpkgs> {}
