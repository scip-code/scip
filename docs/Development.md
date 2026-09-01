# Developing SCIP

- [Project structure](#project-structure)
- [Code generation](#code-generation)
- [Debugging](#debugging)
- [Testing and adding new SCIP semantics](#testing-and-adding-new-scip-semantics)
- [Release a new version](#release-a-new-version)

## Project structure

- [bindings/](./bindings/): Contains a mix of generated and hand-written
  bindings for different languages.
  - The TypeScript, Rust, Haskell, JVM and .NET bindings are auto-generated.
  - The Go bindings include protoc-generated code as well as extra
    functionality. This is used by the CLI below as well as the
    [Sourcegraph CLI](https://github.com/sourcegraph/src-cli).
- [cmd/scip](./cmd/scip): CLI for SCIP.
  - [cmd/scip/tests/](./cmd/scip/tests/): Test data and packages for SCIP.
- [reprolang/](./reprolang/): A verbose, small language
  which consists of declarations, references, imports and other minor bits
  of functionality, which is used to test the SCIP CLI. The language is
  defined using a [tree-sitter grammar](reprolang/grammar.js).
  This functionality is not meant for use outside of this repository.
- [docs/](./docs/): Auto-generated documentation.

## Code generation

1. Regenerating definitions after changing the schema in [scip.proto](./scip.proto).

   ```bash
   nix run .#proto-generate
   ```

   The only dependency you need is Nix.

2. Regenerating snapshots after making changes to the CLI.
   ```
   go test ./cmd/scip -update-snapshots
   ```
3. Regenerating parser for Repro after editing its grammar.
   ```
   cd reprolang
   ./generate-tree-sitter-parser.sh
   ```

## Debugging

Protobuf output can be inspected using `scip print`:

```
scip print /path/to/index.scip
```

This may be a bit verbose. The default Protobuf output is more compact,
and can be inspected using `protoc`:

```
protoc --decode=scip.Index -I /path/to/scip scip.proto < index.scip
```

There is also a `lint` subcommand which performs various well-formedness
checks on a SCIP index. It is meant primarily for people working on a SCIP indexer,
and is not recommended for use in other settings.

```
scip lint /path/to/index.scip
```

## Testing and adding new SCIP semantics

It is helpful to use reprolang to check the existing code navigation behavior
or to design new code navigation behavior.

To do this, add a test file (and implement any new functionality) first.
Then, regenerate the snapshots.

```bash
go test ./cmd/scip -update-snapshots
```

## Release a new version

Update the version in `cmd/scip/version.txt`, `bindings/rust/Cargo.toml`,
`bindings/rust/Cargo.lock`, `bindings/java/pom.xml`, `bindings/kotlin/pom.xml`,
`bindings/dotnet/Scip.csproj`, and `docs/CLI.md`, then land a commit with those
changes. The [jvm-bindings workflow](/.github/workflows/jvm-bindings.yaml)
fails the PR if the two `pom.xml` versions don't match `cmd/scip/version.txt`,
and the [dotnet-bindings workflow](/.github/workflows/dotnet-bindings.yaml)
does the same for `Scip.csproj`.

After the commit is on `main`, trigger the
[release workflow](/.github/workflows/release.yaml) from the
Actions tab on GitHub, providing the version number (e.g. `0.7.0`).
The workflow will validate version.txt, create and push tags, create a draft
GitHub release (with auto-generated notes), publish the Rust crate, publish the
Java/Kotlin bindings to Maven Central, publish the .NET bindings to NuGet,
build and upload CLI binaries, and finally mark the release as non-draft.

### JVM bindings publishing

The Java and Kotlin bindings are published to Maven Central under the
`org.scip-code` namespace via the
[Sonatype Central Portal](https://central.sonatype.com), driven by the
`release` profile in `bindings/{java,kotlin}/pom.xml` and the
`publish-jvm-bindings` job in the release workflow.

Required GitHub Actions secrets:

| Secret                  | Source                                                                                  |
| ----------------------- | --------------------------------------------------------------------------------------- |
| `MAVEN_USERNAME`        | Token name from the [Central Portal account page](https://central.sonatype.com/account) |
| `MAVEN_PASSWORD`        | Token secret from the same page                                                         |
| `MAVEN_GPG_PRIVATE_KEY` | `gpg --armor --export-secret-keys $KEYID` of a passphrase-less primary signing key      |

`scip-kotlin-bindings` depends on `scip-java-bindings`, so the Java
deploy uses `<waitUntil>published</waitUntil>` (~10–30 min) before the
Kotlin deploy runs. Publications are irreversible — bad releases are
fixed by bumping `cmd/scip/version.txt`.

### .NET bindings publishing

The .NET bindings are published to [nuget.org](https://www.nuget.org) as the
`Scip` package by the `publish-dotnet-bindings` job in the release workflow,
which packs the project through `nix develop` so the SDK matches the
`dotnet-bindings` check.

The job authenticates with
[trusted publishing](https://learn.microsoft.com/en-us/nuget/nuget-org/trusted-publishing)
rather than a stored API key: it asks GitHub for an OIDC token
(`permissions: id-token: write`) and `NuGet/login` exchanges that token for an
API key that expires after an hour. Nothing long-lived has to be rotated, which
matters because nuget.org now caps manually created keys at 30 days.

On nuget.org, under your username → Trusted Publishing, add a policy:

| Field            | Value                                                         |
| ---------------- | ------------------------------------------------------------- |
| Policy owner     | the user or organization that owns the `Scip` package         |
| Repository owner | `scip-code`                                                   |
| Repository       | `scip`                                                        |
| Workflow file    | `release.yaml` (file name only, no `.github/workflows/` path) |
| Environment      | leave empty; the job uses no environment                      |

A policy covers every package its owner owns, so it works for the first
publication, which creates the `Scip` package id. Policies on private
repositories start out temporarily active for 7 days and become permanent on
the first successful publish, which is when nuget.org learns the GitHub
repository and owner IDs.

Required GitHub Actions secret:

| Secret       | Source                                                                      |
| ------------ | --------------------------------------------------------------------------- |
| `NUGET_USER` | The nuget.org username (profile name, not email) that owns the trust policy |

NuGet publications are irreversible (versions can be unlisted, not deleted), so
bad releases are fixed by bumping `cmd/scip/version.txt`.
