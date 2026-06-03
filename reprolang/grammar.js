// Reprolang grammar. See README.md for an overview and ./testdata/snapshots
// for examples.
module.exports = grammar({
  name: 'reprolang',
  extras: $ => [/\s+/],
  word: $ => $.name,

  rules: {
    source_file: $ => repeat($._statement),
    _statement: $ =>
      choice(
        $.definition_statement,
        $.reference_statement,
        $.relationships_statement,
        $.comment
      ),
    definition_statement: $ =>
      seq(
        field('docstring', optional($.docstring)),
        'definition',
        field('name', $.identifier),
        field('relations', repeat($._definition_relations))
      ),
    reference_statement: $ =>
      seq(
        'reference',
        field('forward_definition', optional($.forward_definition)),
        field('name', $.identifier)
      ),
    forward_definition: $ => 'forward_definition',
    _definition_relations: $ =>
      choice($.implements, $.type_defines, $.references),
    implements: $ => seq('implements', field('name', $.identifier)),
    type_defines: $ => seq('type_defines', field('name', $.identifier)),
    references: $ => seq('references', field('name', $.identifier)),
    // Meant to be used primarily when trying to construct indexes with
    // relationships for symbols which lack a definition themselves,
    // and are defined by some other symbol.
    relationships_statement: $ =>
      seq(
        'relationships',
        field('name', $.identifier),
        field('relations', repeat($._all_relations))
      ),
    _all_relations: $ => choice($._definition_relations, $.defined_by),
    defined_by: $ => seq('defined_by', field('name', $.identifier)),
    comment: $ => seq('#', /.*/),
    docstring: $ => seq('# docstring:', /.*/),
    identifier: $ =>
      choice(
        field('local', $.local_identifier),
        field('global', $.global_identifier),
        $.name
      ),
    local_identifier: $ => seq('local', field('name', $.name)),
    global_identifier: $ =>
      seq(field('project_name', $.name), field('descriptors', $.name)),
    name: $ => /[^\s]+/,
  },
})
