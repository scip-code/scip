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

using var stream = File.OpenRead("index.scip");
var index = Index.Parser.ParseFrom(stream);

Console.WriteLine(index.Metadata.ProjectRoot);
foreach (var document in index.Documents)
{
    Console.WriteLine($"{document.RelativePath} {document.Occurrences.Count}");
}
```

[SCIP Code Intelligence Protocol]: https://github.com/scip-code/scip
[`Google.Protobuf`]: https://www.nuget.org/packages/Google.Protobuf
