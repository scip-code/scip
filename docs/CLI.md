# SCIP CLI Reference

<!--toc:start-->

- [SCIP CLI Reference](#scip-cli-reference)
  - [`scip lint`](#scip-lint)
  - [`scip print`](#scip-print)
  - [`scip snapshot`](#scip-snapshot)
  - [`scip stats`](#scip-stats)
  - [`scip expt-convert`](#scip-convert)
  <!--toc:end-->

```
NAME:
   scip - SCIP Code Intelligence Protocol CLI

USAGE:
   scip [global options] [command [command options]]

VERSION:
   v0.7.1

DESCRIPTION:
   For more details, see the project README at:

     https://github.com/scip-code/scip

COMMANDS:
   lint          Flag potential issues with a SCIP index
   print         Print a SCIP index for debugging
   snapshot      Generate snapshot files for golden testing
   stats         Output useful statistics about a SCIP index
   test          Validate a SCIP index against test files
   expt-convert  [EXPERIMENTAL] Convert a SCIP index to a SQLite database
   help, h       Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help
   --version, -v  print the version
```

## `scip lint`

```
NAME:
   scip lint - Flag potential issues with a SCIP index

USAGE:
   scip lint [options]

DESCRIPTION:
   Example usage:

     scip lint /path/to/index.scip

   You may want to filter the output using `grep -v <pattern>`
   to narrow down on certain classes of errors.

OPTIONS:
   --help, -h  show help
```

## `scip print`

```
NAME:
   scip print - Print a SCIP index for debugging

USAGE:
   scip print [options]

DESCRIPTION:
   WARNING: The TTY output may change over time.
   Do not rely on non-JSON output in scripts

OPTIONS:
   --json      Output in JSON format
   --color     Enable color output for TTY (no effect for JSON) (default: true)
   --help, -h  show help
```

## `scip snapshot`

```
NAME:
   scip snapshot - Generate snapshot files for golden testing

USAGE:
   scip snapshot [options]

DESCRIPTION:
   The snapshot subcommand generates snapshot files which
   can be use for inspecting the output of an index in a
   visual way. Occurrences are marked with caret signs (^)
   and symbol information.

   For testing a SCIP indexer, you can either use this subcommand
   along with 'git diff' or equivalent, or you can use the dedicated
   'test' subcommand for more targeted checks.


OPTIONS:
   --from string            Path to SCIP index file (default: "index.scip")
   --to string              Path to output directory for snapshot files (default: "scip-snapshot")
   --project-root string    Override project root in the SCIP file. This can be helpful when the SCIP index was created on another computer
   --strict                 If true, fail fast on errors
   --comment-syntax string  Comment syntax to use for snapshot files (default: "//")
   --help, -h               show help
```

## `scip test`

```
NAME:
   scip test - Validate a SCIP index against test files

USAGE:
   scip test [options]

DESCRIPTION:
   Validates whether the SCIP data present in an index
   matches that specified in human-readable test files, using syntax
   similar to the 'snapshot' subcommand. Test file syntax reference:

       https://github.com/scip-code/scip/blob/v0.7.1
   /docs/test_file_format.md

   The test files are located based on the relative_path field
   in the SCIP document, interpreted relative to the the directory
   the CLI is invoked in.

   If you want to instead check all the data in a SCIP index,
   use the 'snapshot' subcommand.

OPTIONS:
   --from string                                              Path to SCIP index file (default: "index.scip")
   --comment-syntax string                                    Comment syntax to use for snapshot files (default: "//")
   --filter string, -f string [ --filter string, -f string ]  Explicit list of test files to check. Can be specified multiple times. If not specified, all files are tested.
   --check-documents                                          Whether or not to validate whether every file in the test directory has a correlating document in the SCIP index.
   --help, -h                                                 show help
```

## `scip stats`

```
NAME:
   scip stats - Output useful statistics about a SCIP index

USAGE:
   scip stats [options]

OPTIONS:
   --from string          Path to SCIP index file (default: "index.scip")
   --project-root string  Override project root in the SCIP file. This can be helpful when the SCIP index was created on another computer
   --help, -h             show help
```

## `scip expt-convert`

```
NAME:
   scip expt-convert - [EXPERIMENTAL] Convert a SCIP index to a SQLite database

USAGE:
   scip expt-convert [options]

DESCRIPTION:
   Converts a SCIP index to a SQLite database.

   For inspecting the data, use the SQLite CLI.
   For inspecting the schema, use .schema.

   Occurrences are stored opaquely as a blob to prevent the DB size from growing very quickly.

OPTIONS:
   --output string       Path to output SQLite database file (default: "index.db")
   --cpu-profile string  Path to output prof file
   --help, -h            show help
```
