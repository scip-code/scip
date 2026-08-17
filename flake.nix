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
            vendorHash = "sha256-EK6YzQTpigUvMLEE8GWf1dzbPpXi0jb22CsstW2E/Ys=";
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
              ];
              text = ''
                buf generate
                goimports -w ./bindings/go/scip/scip.pb.go
                # protoc's C# backend emits `class Descriptor` containing a
                # static `Descriptor` property, which C# rejects (CS0542:
                # member names cannot be the same as their enclosing type).
                # Rename the generated class - not the Protobuf message, so
                # the descriptor pool still says `scip.Descriptor` - to
                # SymbolDescriptor, the name used both by scip.proto's own
                # comments and by the existing scip-dotnet bindings.
                sed -i -E \
                  -e 's/global::Scip\.Descriptor\b/global::Scip.SymbolDescriptor/g' \
                  -e 's/\bclass Descriptor\b/class SymbolDescriptor/' \
                  -e 's/<Descriptor>/<SymbolDescriptor>/g' \
                  -e 's/\bnew Descriptor\(/new SymbolDescriptor(/g' \
                  -e 's/\bpublic Descriptor\b/public SymbolDescriptor/g' \
                  -e 's/\bDescriptor other\b/SymbolDescriptor other/g' \
                  -e 's/\bas Descriptor\)/as SymbolDescriptor)/' \
                  -e 's/\bthe Descriptor message type\b/the SymbolDescriptor message type/' \
                  ./bindings/dotnet/src/Scip.cs
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
