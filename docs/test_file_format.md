# `scip test` file format

The `scip test` command validates whether a provided SCIP index contains the data specified in a human-readable test file.
The test file syntax is inspired by [Sublime Text's syntax highlighting tests](https://www.sublimetext.com/docs/syntax.html#testing).

## File Format

Test cases are made up of a range, type, and data attribute.

### Ranges

Three range selection comment formats are supported:

- `// ^^^` (2 or more `^`): enforces the length of the occurrence. Will fail if the range at this location does not equal 3 characters
- `// ^`: ignore length, `^` can occur at any point to any character in the occurrence
- `// <-`: ignore length, and treat the character above the first comment character as the start of the occurrence, similar to Sublime Text

```js
function someFunction() {
  //     ^ ...
  //     ^^^^^^^^^^^^ ...
  // <- ...
}
```

The `_` marker behaves exactly like `^` but is reserved for `synthetic_definition`
test cases (see below), mirroring the `scip snapshot` output.

### Type and Data

There are five possible types of test cases. The chosen test case is determined by the first word after the range selection

- `definition [symbol]` - validates that the specified range has a symbol with the role of "definition" with the specified `[symbol]`
- `reference [symbol]` - validates that the specified range has a symbol with the role of "reference" with the specified `[symbol]`
- `forward_definition [symbol]` - validates that the specified range has a symbol with the role of "forward_definition" with the specified `[symbol]`
- `synthetic_definition [symbol]` - validates that the specified range has a synthetic definition for `[symbol]`. Synthetic definitions are derived from `SymbolInformation` entries that declare an `is_definition` relationship, and are projected onto the range of the related definition occurrence. Use the `_` range marker for these.
- `diagnostic [severity] [message]` - validates that the specified range has a diagnostic with the given `[severity]` and `[message]`

```js
function someFunction() {
  //     ^ definition scip-typescript npm test_package 1.0.0 lib/`test.js`/someFunction().

  someOtherFunction()
  // <- reference scip-typescript npm test_package 1.0.0 lib/`test.js`/someOtherFunction().
}
```

The message for diagnostics can be specified on the following line using `>`,
and may span over multiple lines.

```js
function someFn() {
  let someVar = ''
  //   ^ diagnostic Warning
  //   > someVar is unused.
  //   > remove it or use it.
}
```

### Multiline occurrences

Occurrences whose range spans multiple lines are anchored to their start line.
The markers cover the start line, and an optional `<lineDelta>:<endCharacter>`
suffix after the symbol asserts the end position, where `lineDelta` is the
number of lines between the start and end line. When the suffix is omitted, the
end position is not checked.

```js
const value = compute(
  //          ^^^^^^^ reference scip-typescript npm test_package 1.0.0 lib/`test.js`/compute(). 1:1
  arg
)
```

### Ignored lines

`scip test` only validates occurrence-level assertions and diagnostics. Snapshot
detail lines for symbol metadata such as `kind`, `display_name`, `documentation`,
`signature_documentation`, `relationship`, and `enclosing_symbol` carry no range
marker and are ignored, so a block of `scip snapshot` output can be pasted into a
test file without those lines causing failures.
