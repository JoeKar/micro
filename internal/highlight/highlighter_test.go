package highlight

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const syntax = `
filetype: test
detect:
    filename: "test"
rules:
    - type: "\\b(one|two|three)\\b"
    - constant.string:
        start: "\""
        end: "\""
    - constant.string:
        start: "'"
        end: "'"
        rules:
            - error: "[[:print:]][[:print:]]+"
    - comment:
        start: "//"
        end: "$"
        rules:
            - todo: "(TODO):?"
    - comment:
        start: "/\\*"
        end: "\\*/"
        rules:
            - todo: "(TODO):?"
    - symbol:
        start: "<"
        end: ">"
        rules:
            - identifier: "\\b(one|two|three)\\b"
            - constant.string:
                start: "\""
                end: "\""
`

const content = `
one two three
// comment
one
two
three
/*
multi
line
// comment
with
TODO
*/
"string"
one two three
"
multi
line
str'ng
"
one two three
'o'
one two three
// '
one "string" two /*rule*/ three //all
/* " */
<one="one" two="two" three="three">
'one' "string" 'two
rule
three' "all" '!' "!"
`

var expectation = []map[int]string{
	{},
	{0: "type", 3: "", 4: "type", 7: "", 8: "type"},
	{0: "comment"},
	{0: "type"},
	{0: "type"},
	{0: "type"},
	{0: "comment"},
	{0: "comment"},
	{0: "comment"},
	{0: "comment"},
	{0: "comment"},
	{0: "todo"},
	{0: "comment"},
	{0: "constant.string"},
	{0: "type", 3: "", 4: "type", 7: "", 8: "type"},
	{0: "constant.string"},
	{0: "constant.string"},
	{0: "constant.string"},
	{0: "constant.string"},
	{0: "constant.string"},
	{0: "type", 3: "", 4: "type", 7: "", 8: "type"},
	{0: "constant.string"},
	{0: "type", 3: "", 4: "type", 7: "", 8: "type"},
	{0: "comment"},
	{0: "type", 3: "", 4: "constant.string", 12: "", 13: "type", 16: "", 17: "comment", 25: "", 26: "type", 31: "", 32: "comment"},
	{0: "comment"},
	{0: "symbol", 1: "identifier", 4: "symbol", 5: "constant.string", 10: "symbol", 11: "identifier", 14: "symbol", 15: "constant.string", 20: "symbol", 21: "identifier", 26: "symbol", 27: "constant.string", 34: "symbol"},
	{0: "constant.string", 1: "error", 4: "constant.string", 5: "", 6: "constant.string", 14: "", 15: "constant.string", 16: "error"},
	{0: "error"},
	{0: "error", 5: "constant.string", 6: "", 7: "constant.string", 12: "", 13: "constant.string", 16: "", 17: "constant.string"},
	{},
}

func TestHighlightString(t *testing.T) {
	header, err := MakeHeaderYaml([]byte(syntax))
	if !assert.NoError(t, err) {
		return
	}

	file, err := ParseFile([]byte(syntax))
	if !assert.NoError(t, err) {
		return
	}

	def, err := ParseDef(file, header)
	if !assert.NoError(t, err) {
		return
	}

	highlighter := NewHighlighter(def)
	matches := highlighter.HighlightString(content)

	var actual []map[int]string
	for _, m := range matches {
		line := map[int]string{}
		for k, g := range m {
			line[k] = g.String()
		}
		actual = append(actual, line)
	}
	assert.Equal(t, expectation, actual)
}
