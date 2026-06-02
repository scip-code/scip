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
                  # Remove once https://github.com/NixOS/nixpkgs/pull/525163
                  # lands in the pinned nixos-26.05 channel.
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
              # ScalaPB's protoc plugin. nixpkgs does not package it, so we
              # fetch the universal `-unix.sh` self-executing JAR launcher
              # published to Maven Central and wrap it so `java` is on PATH.
              # The wrapped script also works on aarch64 (unlike the native
              # binaries published to GitHub releases).
              # Must stay in sync with the scalapb.version in
              # bindings/scala/pom.xml.
              protoc-gen-scala =
                let
                  scalapbVersion = "0.11.20";
                  launcher = pkgs.fetchurl {
                    url = "https://repo1.maven.org/maven2/com/thesamet/scalapb/protoc-gen-scala/${scalapbVersion}/protoc-gen-scala-${scalapbVersion}-unix.sh";
                    hash = "sha256-aJZX96LQ+uH22fvobUo1n2pIcfs7feRjKaitSlNCoAE=";
                  };
                in
                pkgs.writeShellScriptBin "protoc-gen-scala" ''
                  export PATH=${pkgs.jre}/bin:$PATH
                  exec ${pkgs.bash}/bin/sh ${launcher} "$@"
                '';
            in
            pkgs.writeShellApplication {
              name = "proto-generate";
              runtimeInputs = with pkgs; [
                buf
                gotools
                haskellPackages.proto-lens-protoc
                prettier
                protobuf
                protoc-gen-doc
                protoc-gen-es
                protoc-gen-go
                protoc-gen-rs
                protoc-gen-scala
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
