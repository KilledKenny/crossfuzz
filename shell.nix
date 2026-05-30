# Local development shell.
#
# Packages are defined in nix/packages.nix and shared with the CI Docker image
# (nix/docker.nix) so dev and CI environments stay in sync.
#
# Usage: nix-shell
{ pkgs ? import ./nix/nixpkgs.nix }:

pkgs.mkShell {
  packages = import ./nix/packages.nix { inherit pkgs; };
}
