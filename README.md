# SCIP Code Intelligence Protocol

SCIP (pronunciation: "skip") is a language-agnostic protocol
for indexing source code,
which can be used to power code navigation functionality
such as Go to definition, Find references, and Find implementations.

This repository includes:

- A [Protobuf schema for SCIP](./scip.proto).
- Rich Go and Rust bindings for SCIP: These include many utility functions
  to help build tooling on top of SCIP.
- Auto-generated bindings for TypeScript and Haskell.
- The [`scip` CLI](./docs/CLI.md), which makes SCIP indexes
  a breeze to work with.

If you're interested in better understanding the motivation behind SCIP,
check out the [announcement blog post](https://about.sourcegraph.com/blog/announcing-scip) and the [design doc](./docs/DESIGN.md).

If you're interested in writing a new indexer that emits SCIP,
check out our documentation on
[how to write an indexer](https://docs.sourcegraph.com/code_intelligence/explanations/writing_an_indexer).
Also, check out the [Debugging section][] in the Development docs.

If you're interested in consuming SCIP data,
you can either use one of the [provided language bindings](https://github.com/scip-code/scip/tree/main/bindings),
or generate code for the [SCIP Protobuf schema](./scip.proto)
using the Protobuf toolchain for your language ecosystem.
Also, check out the [Debugging section][] in the Development docs.

[debugging section]: ./docs/Development.md#debugging

## Tools using SCIP

Several indexers currently emit SCIP data:

- [scip-java](https://github.com/sourcegraph/scip-java): Java, Scala, Kotlin
- [scip-typescript](https://github.com/sourcegraph/scip-typescript): TypeScript, JavaScript
- [rust-analyzer](https://github.com/rust-lang/rust-analyzer): Rust
- [scip-clang](https://github.com/sourcegraph/scip-clang): C++, C
- [scip-ruby](https://github.com/sourcegraph/scip-ruby): Ruby
- [scip-python](https://github.com/sourcegraph/scip-python): Python
- [scip-dotnet](https://github.com/sourcegraph/scip-dotnet): C#, Visual Basic
- [scip-dart](https://github.com/Workiva/scip-dart): Dart
- [scip-php](https://github.com/davidrjenni/scip-php): PHP
- [debian-lsp](https://github.com/jelmer/debian-lsp): Debian packaging files

For more details about indexers,
see the [Sourcegraph documentation](https://docs.sourcegraph.com/code_navigation/references/indexers).

Other tools which use SCIP include the [Sourcegraph CLI](https://github.com/sourcegraph/src-cli),
and the SCIP CLI in this repo.

## Installing the `scip` CLI

You can find binaries for the `scip` CLI tool [here](https://github.com/scip-code/scip/releases).
You can also compile a binary locally using:

```sh
git clone https://github.com/scip-code/scip.git --depth=1
cd scip
go build ./cmd/scip
```

You can consult the [CLI reference](docs/CLI.md) or `--help` for usage information.

## Contributing

We welcome questions, suggestions as well as code and docs contributions.

For submitting contributions, check out [Development.md](./docs/Development.md)
to better understand project structure and common workflows.

Contributors should abide by the [Code of Conduct](https://scip-code.org/code-of-conduct.html).
