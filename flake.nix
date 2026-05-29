{
  description = "SCIP - SCIP Code Intelligence Protocol";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = pkgs.lib.fileContents ./cmd/scip/version.txt;
      in
      {
        packages = {
          scip = pkgs.buildGoModule {
            pname = "scip";
            inherit version;

            src = ./.;
            vendorHash = "sha256-WheG2CpEUDtzpqRuzRu+g0hDrfqVZu+2GNCntNJdfDQ=";
            proxyVendor = true;

            subPackages = [ "cmd/scip" ];

            env.GOWORK = "off";
            ldflags = [ "-X main.Reproducible=true" ];

            meta = {
              description = "SCIP Code Intelligence Protocol";
              homepage = "https://github.com/scip-code/scip";
              license = pkgs.lib.licenses.asl20;
              mainProgram = "scip";
            };
          };

          proto-generate =
            let
              protoc-gen-rs = pkgs.rustPlatform.buildRustPackage {
                pname = "protoc-gen-rs";
                version = "3.7.2";
                src = pkgs.fetchCrate {
                  pname = "protobuf-codegen";
                  version = "3.7.2";
                  # crates.io's API host (https://crates.io/api/v1/crates)
                  # 403s default HTTP-library User-Agents (curl/*, python-
                  # requests/*) per their crawler policy. fetchurl shells
                  # out to curl with the default UA, so the fetch fails.
                  # The static CDN at static.crates.io serves identical
                  # bytes with no UA gate. nixpkgs fixed this for
                  # importCargoLock and fetchCargoVendor (already in
                  # nixos-26.05) but fetchcrate.nix still defaults to the
                  # blocked endpoint; remove this override once
                  # https://github.com/NixOS/nixpkgs/pull/512735 has a
                  # counterpart for fetchCrate.
                  registryDl = "https://static.crates.io/crates";
                  hash = "sha256-0d+xjYXpl87Sq/DdE8K2olnKa5bNpEHX7RTjp/2xza4=";
                };
                cargoHash = "sha256-xxw1WSP0Qatf5QT+JBUQPi8HFOPRMGbnFMVLOiKnTNk=";
                cargoBuildFlags = [
                  "--bin"
                  "protoc-gen-rs"
                ];
                nativeBuildInputs = [ pkgs.protobuf ];
              };
            in
            pkgs.writeShellApplication {
              name = "proto-generate";
              runtimeInputs = with pkgs; [
                buf
                gotools
                haskellPackages.proto-lens-protoc
                prettier
                protoc-gen-doc
                protoc-gen-es
                protoc-gen-go
                protoc-gen-rs
              ];
              text = ''
                buf generate
                goimports -w ./bindings/go/scip/scip.pb.go
                prettier --write --list-different '**/*.{ts,js(on)?,md,yml}'
              '';
            };

          default = self.packages.${system}.scip;
        };

        checks = import ./checks.nix {
          inherit pkgs version;
        };

        formatter = pkgs.nixfmt;

        devShells.default = pkgs.mkShell {
          inputsFrom = [ self.packages.${system}.scip ];

          packages =
            with pkgs;
            [
              cargo
              go
              nodejs
              rustc
              tree-sitter
            ]
            ++ (with pkgs.haskellPackages; [
              cabal-install
              ghc
              proto-lens-runtime
            ]);
        };
      }
    );
}
