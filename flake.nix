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
                # member names cannot be the same as their enclosing type),
                # so the generated class has to be renamed. The Protobuf
                # message keeps its name: `scip.Descriptor` is what the
                # descriptor pool, `Any` type URLs and canonical JSON say in
                # every other binding, so renaming the message in a copy of
                # scip.proto - what sourcegraph/scip-dotnet does - would buy
                # a tidier generator at the price of a C# binding that
                # disagrees with the protocol. SymbolDescriptor is the name
                # scip.proto's own comments already use.
                if ! grep -qE '^message Descriptor \{' ./scip.proto; then
                  echo 'scip.proto no longer declares a message named Descriptor: the C# rename below is stale.' >&2
                  exit 1
                fi
                before=$(sed -E 's/\bSymbolDescriptor\b/Descriptor/g' ./bindings/dotnet/src/Scip.cs)
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
                # Those expressions have to add up to exactly a rename.
                # Folding SymbolDescriptor back to Descriptor on both sides
                # must give the same text, so anything else they touched
                # fails here. An occurrence they miss fails in the
                # `dotnet-bindings` check instead, where the file no longer
                # compiles.
                after=$(sed -E 's/\bSymbolDescriptor\b/Descriptor/g' ./bindings/dotnet/src/Scip.cs)
                if [ "$before" != "$after" ]; then
                  echo 'The Descriptor -> SymbolDescriptor rewrite changed more than that identifier.' >&2
                  echo 'protoc emits a shape the expressions in flake.nix no longer match; fix them there.' >&2
                  exit 1
                fi

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
              dotnetCorePackages.sdk_10_0
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
