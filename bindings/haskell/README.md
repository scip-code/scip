# scip --- Haskell bindings for SCIP

[SCIP] (pronounced "skip") is a language-agnostic protocol for indexing source
code. This package exposes Haskell types and lenses generated from
[`scip.proto`] via [`proto-lens`].

## Usage

```haskell
import Data.ProtoLens (decodeMessage)
import qualified Data.ByteString as BS
import qualified Proto.Scip as Scip
import           Proto.Scip_Fields (documents, occurrences, symbol)
import           Lens.Family2 ((^.))

main :: IO ()
main = do
  bytes <- BS.readFile "index.scip"
  case decodeMessage bytes :: Either String Scip.Index of
    Left  err -> putStrLn $ "parse error: " ++ err
    Right idx -> mapM_ print
      [ occ ^. symbol
      | doc <- idx ^. documents
      , occ <- doc ^. occurrences
      ]
```

[SCIP]: https://github.com/scip-code/scip
[`scip.proto`]: https://github.com/scip-code/scip/blob/main/scip.proto
[`proto-lens`]: https://github.com/google/proto-lens
