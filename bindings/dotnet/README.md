# `Scip`

Generated .NET bindings for the [SCIP Code Intelligence Protocol].

## Install

```sh
dotnet add package Scip
```

The package targets `netstandard2.0` and depends on [`Google.Protobuf`], which
is installed transitively.

## Use

```csharp
using Scip;
// Alias `Scip.Index`, which is ambiguous with `System.Index` under
// implicit usings.
using ScipIndex = Scip.Index;

using var stream = File.OpenRead("index.scip");
var index = ScipIndex.Parser.ParseFrom(stream);

Console.WriteLine(index.Metadata.ProjectRoot);
foreach (var document in index.Documents)
{
    Console.WriteLine($"{document.RelativePath} {document.Occurrences.Count}");
}
```

## Naming

The `scip.Descriptor` message is generated as `Scip.SymbolDescriptor`: C#
rejects a class whose member has the same name as the class itself, and every
generated message carries a static `Descriptor` property. The name matches the
schema's own comments and the [scip-dotnet] indexer. Nothing else is renamed,
and the Protobuf descriptor still reports `scip.Descriptor`.

[SCIP Code Intelligence Protocol]: https://github.com/scip-code/scip
[scip-dotnet]: https://github.com/sourcegraph/scip-dotnet
[`Google.Protobuf`]: https://www.nuget.org/packages/Google.Protobuf
