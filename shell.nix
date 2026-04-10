{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  packages = [
    pkgs.openjdk25
    pkgs.go
    pkgs.clang
    pkgs.bun
    pkgs.gradle
    pkgs.maven
  ];

}