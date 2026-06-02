# `@scip-code/scip`

Generated TypeScript definitions for the [SCIP Code Intelligence Protocol].

## Install

``` sh
npm install @scip-code/scip
```

The package is ESM-only and ships with `.d.ts` declarations. Peer-style runtime
dependency on [`@bufbuild/protobuf`], which is installed transitively.

## Use

``` ts
import { fromBinary, toBinary } from "@bufbuild/protobuf";
import { IndexSchema } from "@scip-code/scip";
import { readFileSync } from "node:fs";

const bytes = readFileSync("index.scip");
const index = fromBinary(IndexSchema, bytes);

console.log(index.metadata?.projectRoot);
for (const doc of index.documents) {
  console.log(doc.relativePath, doc.occurrences.length);
}
```

  [SCIP Code Intelligence Protocol]: https://github.com/scip-code/scip
  [`@bufbuild/protobuf`]: https://www.npmjs.com/package/@bufbuild/protobuf
