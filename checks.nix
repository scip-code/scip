{
  pkgs,
  version,
}:
let
  # Extracts the project's <version> element from a pom.xml.
  # Looks for the unique top-level <version> at 2-space indentation, which
  # distinguishes it from <version> elements nested inside <dependencies>,
  # <plugin>, etc. (deeper indentation).
  extractPomVersion =
    pomFile:
    let
      lines = pkgs.lib.splitString "\n" (builtins.readFile pomFile);
      isVersionLine = line: builtins.match "^  <version>[0-9.]+</version>$" line != null;
      versionLine = pkgs.lib.findFirst isVersionLine null lines;
      match = builtins.match "^  <version>([0-9.]+)</version>$" versionLine;
    in
    builtins.head match;
in
{
  github-actions = pkgs.stdenv.mkDerivation {
    pname = "scip-github-actions";
    inherit version;
    src = ./.;
    nativeBuildInputs = [ pkgs.action-validator ];
    buildPhase = ''
      for f in .github/workflows/*.yml .github/workflows/*.yaml; do
        [ -e "$f" ] && action-validator -v "$f"
      done
    '';
    installPhase = "touch $out";
  };

  formatting = pkgs.stdenv.mkDerivation {
    pname = "scip-formatting";
    inherit version;
    src = ./.;
    nativeBuildInputs = with pkgs; [
      buf
      go
      gotools
      nixfmt
      prettier
    ];
    buildPhase = ''
      prettier --check '**/*.{ts,js(on)?,md,yml}'
      BUF_CACHE_DIR=$(mktemp -d) buf format --diff --exit-code scip.proto
      gofmt -d . | tee /dev/stderr | diff /dev/null -
      goimports -d . | tee /dev/stderr | diff /dev/null -
      nixfmt --check *.nix
    '';
    installPhase = "touch $out";
  };

  go-bindings = pkgs.buildGoModule {
    pname = "scip-bindings-go";
    inherit version;
    src = ./.;
    modRoot = "./bindings/go/scip";
    vendorHash = "sha256-7R+qrgZCcoJ9oy5VhLsdskC/oyJRrqkcrI0JOiMAR0w=";
    env.GOWORK = "off";
    buildTags = [ "asserts" ];
    subPackages = [
      "."
      "memtest"
      "testutil"
    ];
    installPhase = "touch $out";
  };

  haskell-bindings =
    let
      cabal = pkgs.haskellPackages.callCabal2nix "scip" ./bindings/haskell { };
    in
    assert pkgs.lib.assertMsg (
      cabal.version == version
    ) "Version mismatch in bindings/haskell/scip.cabal: expected ${version}, got ${cabal.version}";
    cabal.overrideAttrs {
      prePatch = ''
        cp --remove-destination ${./LICENSE} LICENSE
      '';
    };

  java-bindings =
    let
      pomVersion = extractPomVersion ./bindings/java/pom.xml;
    in
    assert pkgs.lib.assertMsg (
      pomVersion == version
    ) "Version mismatch in bindings/java/pom.xml: expected ${version}, got ${pomVersion}";
    pkgs.maven.buildMavenPackage {
      pname = "scip-bindings-java";
      inherit version;
      src = ./bindings/java;
      mvnHash = "sha256-qh+F+aNYKBDkdW4fZVnjN03F0bxTMhlZmWM51EnXF6Y=";
      doCheck = false;
      installPhase = "touch $out";
    };

  kotlin-bindings =
    let
      pomVersion = extractPomVersion ./bindings/kotlin/pom.xml;
    in
    assert pkgs.lib.assertMsg (
      pomVersion == version
    ) "Version mismatch in bindings/kotlin/pom.xml: expected ${version}, got ${pomVersion}";
    pkgs.maven.buildMavenPackage {
      pname = "scip-bindings-kotlin";
      inherit version;
      src = ./bindings/kotlin;
      mvnHash = "sha256-sLoAmf+p/UGVYOxbTT8u+zfzfZzckdxTUktWmbZFg/A=";
      doCheck = false;
      installPhase = "touch $out";
    };

  reprolang =
    let
      reprolangVersion = (builtins.fromJSON (builtins.readFile ./reprolang/package.json)).version;
    in
    assert pkgs.lib.assertMsg (
      reprolangVersion == version
    ) "Version mismatch in reprolang/package.json: expected ${version}, got ${reprolangVersion}";
    pkgs.buildGoModule {
      pname = "scip-reprolang";
      inherit version;
      src = ./.;
      modRoot = "./reprolang";
      vendorHash = "sha256-RnXZMTHrIr02jA4GI1kX4D94GiHu7XbLLCk1RBtPVQc=";
      proxyVendor = true;
      env.GOWORK = "off";
      buildInputs = [ pkgs.tree-sitter ];
      subPackages = [
        "grammar"
        "repro"
      ];
      installPhase = "touch $out";
    };

  reprolang-generated = pkgs.stdenv.mkDerivation {
    pname = "scip-reprolang-generated";
    inherit version;
    src = ./.;
    nativeBuildInputs = with pkgs; [
      nodejs
      prettier
      tree-sitter
    ];
    buildPhase = ''
      cd reprolang
      cp -r grammar grammar-before
      tree-sitter generate --abi 14 --output grammar
      prettier --write 'grammar/grammar.json' 'grammar/node-types.json'
      diff -rq grammar-before grammar
    '';
    installPhase = "touch $out";
  };

  rust-bindings =
    let
      cargoTomlVersion =
        (builtins.fromTOML (builtins.readFile ./bindings/rust/Cargo.toml)).package.version;
    in
    assert pkgs.lib.assertMsg (
      cargoTomlVersion == version
    ) "Version mismatch in bindings/rust/Cargo.toml: expected ${version}, got ${cargoTomlVersion}";
    pkgs.rustPlatform.buildRustPackage {
      pname = "scip-bindings-rust";
      inherit version;
      src = ./bindings/rust;
      cargoLock = {
        lockFile = ./bindings/rust/Cargo.lock;
      };
    };

  typescript-bindings =
    let
      packageJsonVersion =
        (builtins.fromJSON (builtins.readFile ./bindings/typescript/package.json)).version;
    in
    assert pkgs.lib.assertMsg (packageJsonVersion == version)
      "Version mismatch in bindings/typescript/package.json: expected ${version}, got ${packageJsonVersion}";
    pkgs.buildNpmPackage {
      pname = "scip-bindings-typescript";
      inherit version;
      src = ./bindings/typescript;
      npmDepsHash = "sha256-84LQkN7bJB5q6hGc8TKXC8yD2P+qzBVh34a2L9K2Ji8=";
      buildPhase = ''
        runHook preBuild
        npx tsc --noEmit
        runHook postBuild
      '';
      installPhase = "touch $out";
    };
}
