/*
Copyright © 2022 Martti Leino <rionpy@gmail.com>
GNU General Public License v3.0+ (see LICENSE or https://www.gnu.org/licenses/gpl-3.0.txt)
*/
package lib

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/dlclark/regexp2"
	"gotest.tools/assert"
)

var loremQuotesPath = "../lorem_quotes.txt"

func TestReadToRunes(t *testing.T) {
	rPayload := readToRunes(loremQuotesPath, false)
	testStr := `Lorem ipsum dolor sit amet, "consectetur adipiscing elit". Cras ${BAZ:-$BAR} sem tellus, sed lobortis tellus faucibus eu. Vestibulum eu tortor mauris. 'Vestibulum in $FOO urna'. In auctor sollicitudin malesuada. Ut ${Q} malesuada erat. Mauris viverra convallis eros, ${Q} tincidunt ligula egestas a. "Vivamus ${BAR}, metus a pulvinar blandit", metus leo hendrerit lacus, "non '${BAZ:-${BAR}}' ${FOO:+ipsum}" nulla at sem. Sed vel viverra eros. Duis eget condimentum felis, $FOO ornare est. Nunc maximus hendrerit orci ${Q} porttitor. Curabitur id posuere lorem.`
	assert.Equal(t, reflect.TypeOf(rPayload).String(), "[]int32")
	assert.Equal(t, string(rPayload[0:11]), "Lorem ipsum")
	assert.Equal(t, string(rPayload), testStr)
}

var payload = readToRunes(loremQuotesPath, false)

func TestTokenizeByQuotes(t *testing.T) {
	s := map[string][]Segment{
		`oh "hi\" to" 'you'`: {
			{[]int{0, 3}, unQuoted},
			{[]int{3, 12}, doubleQuoted},
			{[]int{12, 13}, unQuoted},
			{[]int{13, 18}, singleQuoted},
		},
		`oh "'hi'" to \"'you\"`: {
			{[]int{0, 3}, unQuoted},
			{[]int{3, 9}, doubleQuoted},
			{[]int{9, 21}, unQuoted},
		},
		`oh 'hi' to "you"`: {
			{[]int{0, 3}, unQuoted},
			{[]int{3, 7}, singleQuoted},
			{[]int{7, 11}, unQuoted},
			{[]int{11, 16}, doubleQuoted},
		},
		`oh "\'hi\'" to you`: {
			{[]int{0, 3}, unQuoted},
			{[]int{3, 11}, doubleQuoted},
			{[]int{11, 18}, unQuoted},
		},
		`oh \'hi\' to you`: {
			{[]int{0, 16}, unQuoted},
		},
		`"oh 'hi' to you"`: {
			{[]int{0, 16}, doubleQuoted},
		},
		`"hello '${TO:-y"ou}"`: {
			{[]int{0, 16}, doubleQuoted},
			{[]int{16, 20}, unQuoted},
		},
		`'hello "${TO:-y'ou}'`: {
			{[]int{0, 16}, singleQuoted},
			{[]int{16, 20}, unQuoted},
		},
		`"oh" \\"hi\\" \\'to\\' \\\'you\\\'`: {
			{[]int{0, 4}, doubleQuoted},
			{[]int{4, 7}, unQuoted},
			{[]int{7, 13}, doubleQuoted},
			{[]int{13, 16}, unQuoted},
			{[]int{16, 22}, singleQuoted},
			{[]int{22, 34}, unQuoted},
		},
	}
	for i, o := range s {
		assert.DeepEqual(t, tokenizeByQuotes([]rune(i)), o)
	}
}

var tokenizedPayload = tokenizeByQuotes(payload)

type temp struct {
	file string
}

func (t *temp) testFile(content string) func() {
	f, err := os.CreateTemp("", fmt.Sprintf(`lipsum%d.tmp`, time.Now().UnixNano()))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	t.file = f.Name()
	return func() { os.Remove(f.Name()) }
}

func resetEnv(params []string) func() {
	rescue := make(map[string]any)
	for _, param := range params {
		rescue[param] = getEnv(param)
	}
	return func() {
		for name, value := range rescue {
			if value == nil {
				os.Unsetenv(name)
			} else {
				os.Setenv(name, value.(string))
			}
		}
	}
}

func getEnv(param string) any {
	if value, isSet := os.LookupEnv(param); isSet {
		return value
	} else {
		return nil
	}
}

func assertPanic(t *testing.T, f func(), msg string) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code didn't panic as expected")
		} else if fmt.Sprintf(`%s`, r) != msg {
			t.Errorf("Code panicked with incorrect message\nExpected: '%s'\nReceived: '%s'", msg, r)
		}
	}()
	f()
}

func captureOutput(f func()) string {
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	out, _ := ioutil.ReadAll(r)
	os.Stdout = stdout
	return string(out)
}

func TestMatchesToIndices(t *testing.T) {
	runes := []rune("test")
	re := regexp2.MustCompile(".", 0)
	assert.DeepEqual(t, MatchesToIndices(re, runes), [][]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}})
}

func TestInRange(t *testing.T) {
	assert.Assert(t, InRange(1, []int{0, 2}))
	assert.Assert(t, !InRange(1, []int{2, 4}))
	assert.Assert(t, !InRange(5, []int{2, 4}))
}

func TestGetValidSlices(t *testing.T) {
	expected := [][]int{{0, 151}, {176, 561}}
	result := getValidSlices(tokenizedPayload)
	assert.DeepEqual(t, expected, result)
}

func TestFindParamsInSingleString(t *testing.T) {
	sPayload := []rune(`$FOO`)
	validSlices := getValidSlices(tokenizeByQuotes(sPayload))
	assert.DeepEqual(t, validSlices, [][]int{{0, 4}})
	params := findParams(sPayload, validSlices)
	assert.DeepEqual(t, params, []Param{{Id: `$FOO`, Position: []int{0, 4}}})
}

func TestFindParamsWithQuotesInParams(t *testing.T) {
	qPayload := []rune(`foo${BAR:-'baz'}foo`)
	segments := tokenizeByQuotes(qPayload)
	assert.DeepEqual(t, segments, []Segment{
		{[]int{0, 10}, unQuoted},
		{[]int{10, 15}, singleQuoted},
		{[]int{15, 19}, unQuoted},
	})
	params := findParams(qPayload, getValidSlices(tokenizeByQuotes(qPayload)))
	assert.DeepEqual(t, params, []Param{{Id: `${BAR:-'baz'}`, Position: []int{3, 16}}})
}

func TestFindParamsWithEnclosingQuotesInParams(t *testing.T) {
	qPayload := []rune(`'foo${BAR:-'baz'}foo'`)
	segments := tokenizeByQuotes(qPayload)
	assert.DeepEqual(t, segments, []Segment{
		{[]int{0, 12}, singleQuoted},
		{[]int{12, 15}, unQuoted},
		{[]int{15, 21}, singleQuoted},
	})
	params := findParams(qPayload, getValidSlices(tokenizeByQuotes(qPayload)))
	assert.DeepEqual(t, params, []Param(nil))
}

func TestFindNestedParam(t *testing.T) {
	nPayload := []rune(`12${FOO:+${BAR:-${FOO-${BAZ%stahp}}}}34`)
	nValidSlices := getValidSlices(tokenizeByQuotes(nPayload))
	params := findParams(nPayload, nValidSlices)
	assert.DeepEqual(t, params, []Param{{Id: `${FOO:+${BAR:-${FOO-${BAZ%stahp}}}}`, Position: []int{2, 37}}})
}

func TestFindParamsWithEscapedDollar(t *testing.T) {
	ePayload := []rune(`ö\$BARöö${FOO:+\${BAZ}}ö\${FOO:-$BAR}ö${FOO:+\$BAZ}`)
	eValidSlices := getValidSlices(tokenizeByQuotes(ePayload))
	params := findParams(ePayload, eValidSlices)
	assert.DeepEqual(t, params, []Param{
		{Id: `${FOO:+\${BAZ}`, Position: []int{8, 22}},
		{Id: `$BAR`, Position: []int{32, 36}},
		{Id: `${FOO:+\$BAZ}`, Position: []int{38, 51}},
	})
}

func TestFindParams(t *testing.T) {
	validSlices := getValidSlices(tokenizedPayload)
	params := findParams(payload, validSlices)
	expected := []Param{
		{Id: "${BAZ:-$BAR}", Position: []int{64, 76}},
		{Id: "${Q}", Position: []int{215, 219}},
		{Id: "${Q}", Position: []int{267, 271}},
		{Id: "${BAR}", Position: []int{309, 315}},
		{Id: "${BAZ:-${BAR}}", Position: []int{377, 391}},
		{Id: "${FOO:+ipsum}", Position: []int{393, 406}},
		{Id: "$FOO", Position: []int{473, 477}},
		{Id: "${Q}", Position: []int{518, 522}},
	}
	assert.DeepEqual(t, params, expected)
}

func TestFindParamsInOnlyParams(t *testing.T) {
	fString := `$FOO` +
		`${FOO}` +
		`${FOO-foo}` +
		`${FOO:-foo}` +
		`${FOO+foo}` +
		`${FOO:+foo}` +
		`${FOO?foo}` +
		`${FOO:?foo}` +
		`${FOO#foo}` +
		`${FOO##foo}` +
		`${FOO%foo}` +
		`$FOO` +
		`${FOO%%foo}` +
		`$F` +
		`${FOO:-$BAR${BAR}$BAR}`
	fPayload := []rune(fString)
	fValidSlices := getValidSlices(tokenizeByQuotes(fPayload))
	assert.DeepEqual(t, fValidSlices, [][]int{{0, 143}})
	fParams := findParams(fPayload, fValidSlices)
	fExpected := []Param{
		{Id: `$FOO`, Position: []int{0, 4}},
		{Id: `${FOO}`, Position: []int{4, 10}},
		{Id: `${FOO-foo}`, Position: []int{10, 20}},
		{Id: `${FOO:-foo}`, Position: []int{20, 31}},
		{Id: `${FOO+foo}`, Position: []int{31, 41}},
		{Id: `${FOO:+foo}`, Position: []int{41, 52}},
		{Id: `${FOO?foo}`, Position: []int{52, 62}},
		{Id: `${FOO:?foo}`, Position: []int{62, 73}},
		{Id: `${FOO#foo}`, Position: []int{73, 83}},
		{Id: `${FOO##foo}`, Position: []int{83, 94}},
		{Id: `${FOO%foo}`, Position: []int{94, 104}},
		{Id: `$FOO`, Position: []int{104, 108}},
		{Id: `${FOO%%foo}`, Position: []int{108, 119}},
		{Id: `$F`, Position: []int{119, 121}},
		{Id: "${FOO:-$BAR${BAR}$BAR}", Position: []int{121, 143}},
	}

	assert.DeepEqual(t, fParams, fExpected)
}

func TestFindParamsInMultiline(t *testing.T) {
	mPayload := readToRunes("../multi_lorem.txt", false)
	assert.Equal(t, len(mPayload), 3010)
	segments := tokenizeByQuotes(mPayload)
	assert.DeepEqual(t, segments, []Segment{
		{[]int{0, 841}, unQuoted},
		{[]int{841, 912}, singleQuoted},
		{[]int{912, 1054}, unQuoted},
		{[]int{1054, 1868}, doubleQuoted},
		{[]int{1868, 2394}, unQuoted},
		{[]int{2394, 2493}, singleQuoted},
		{[]int{2493, 3010}, unQuoted},
	})
	slices := getValidSlices(segments)
	assert.DeepEqual(t, slices, [][]int{{0, 841}, {912, 2394}, {2493, 3010}})
	params := findParams(mPayload, slices)
	assert.DeepEqual(t, params, []Param{
		{Id: "$FOUND", Position: []int{184, 190}},
		{Id: "$FOUND", Position: []int{311, 317}},
		{Id: "$FOUND", Position: []int{757, 763}},
		{Id: "${FOUND:+.\n\nFusce}", Position: []int{1198, 1216}},
		{Id: "$FOUND", Position: []int{1339, 1345}},
		{Id: "$FOUND", Position: []int{1462, 1468}},
		{Id: "$FOUND", Position: []int{2144, 2150}},
		{Id: "$FOUND", Position: []int{2224, 2230}},
		{Id: "$FOUND", Position: []int{2747, 2753}},
	})
}

func TestEscapeHandler(t *testing.T) {
	for _, val := range []string{
		`foo\\bar`,
		`foo\tbar`,
		`foo\nbar`,
		`foo\abar`,
		`foo\bbar`,
		`foo\fbar`,
		`foo\rbar`,
		`foo\vbar`,
		`foo\$bar`,
	} {
		assert.Assert(t, val != escapeHandler(val))
	}
	assert.Equal(t, escapeHandler(`foo\c\e\"\'bar`), `foo\c\e\"\'bar`)
	assert.Equal(t, escapeHandler(`\t`), `	`)
	assert.Equal(t, escapeHandler(`\\t`), `\t`)
	assert.Equal(t, escapeHandler(`\\\t`), `\	`)
	assert.Equal(t, escapeHandler(`\\\\t`), `\\t`)
	assert.Equal(t, escapeHandler(`ö, \xf6, h\u00F6 \U000000f6`), `ö, ö, hö ö`)
	assert.Equal(t, escapeHandler(`ö|\xf|\xf61|\u00F60|\u0F6|\U00000f6|\U000000f60|\123ö`), `ö|\xf|ö1|ö0|\u0F6|\U00000f6|ö0|Sö`)
}

func TestEscapedSingleQuoteInSingleQuotes(t *testing.T) {
	quote := tokenizeByQuotes([]rune(`'\''`))
	assert.DeepEqual(t, quote, []Segment{
		{[]int{0, 3}, singleQuoted},
		{[]int{3, 4}, unQuoted},
	})
}

func TestQuoteHandler(t *testing.T) {
	value := `"\'"`
	assert.Equal(t, quoteHandler(value, unQuoted), `'`)
	assertPanic(t, func() { quoteHandler(value, singleQuoted) }, `unmatched quote " in: '"\'"'`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `'`)

	value = `oh "hi" tö 'you'`
	assert.Equal(t, quoteHandler(value, unQuoted), `oh hi tö you`)
	assert.Equal(t, quoteHandler(value, singleQuoted), `oh "hi" tö you`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `oh hi tö 'you'`)

	value = `oh "\"hi\'" to '\"you\''`
	assertPanic(t, func() { quoteHandler(value, unQuoted) }, `unmatched quote ' in: oh "\"hi\'" to '\"you\''`)
	assertPanic(t, func() { quoteHandler(value, singleQuoted) }, `unmatched quote " in: 'oh "\"hi\'" to '\"you\'''`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `oh "hi' to '"you''`)

	value = `oh "'hi'" to \"'you\"`
	assertPanic(t, func() { quoteHandler(value, unQuoted) }, `unmatched quote ' in: oh "'hi'" to \"'you\"`)
	assertPanic(t, func() { quoteHandler(value, singleQuoted) }, `unmatched quote ' in: 'oh "'hi'" to \"'you\"'`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `oh 'hi' to "'you"`)

	value = `"oh hi"\'" to you"`
	assert.Equal(t, quoteHandler(value, unQuoted), `oh hi' to you`)
	assertPanic(t, func() { quoteHandler(value, singleQuoted) }, `unmatched quote ' in: '"oh hi"\'" to you"'`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `oh hi' to you`)

	value = `"oh hi\' to you"`
	assert.Equal(t, quoteHandler(value, unQuoted), `oh hi' to you`)
	assertPanic(t, func() { quoteHandler(value, singleQuoted) }, `unmatched quote " in: '"oh hi\' to you"'`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `oh hi' to you`)

	value = `'oh hi\' to you'`
	assertPanic(t, func() { quoteHandler(value, unQuoted) }, `unmatched quote ' in: 'oh hi\' to you'`)
	assert.Equal(t, quoteHandler(value, singleQuoted), `oh hi' to you`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `'oh hi' to you'`)
}

func TestQuoteHandlerWithEscapes(t *testing.T) {
	value := `ö, \xf6, h\u00F6 \U000000f6`
	assert.Equal(t, quoteHandler(value, unQuoted), `ö, xf6, hu00F6 U000000f6`)
	assert.Equal(t, quoteHandler(value, singleQuoted), `ö, ö, hö ö`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `ö, ö, hö ö`)

	value = `ö, \\xf6, h\\u00F6 \\U000000f6`
	assert.Equal(t, quoteHandler(value, unQuoted), `ö, ö, hö ö`)
	assert.Equal(t, quoteHandler(value, singleQuoted), `ö, \xf6, h\u00F6 \U000000f6`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `ö, \xf6, h\u00F6 \U000000f6`)

	value = `ö, '\xf6, h\u00F6' \U000000f6`
	assert.Equal(t, quoteHandler(value, unQuoted), `ö, ö, hö U000000f6`)
	assert.Equal(t, quoteHandler(value, singleQuoted), `ö, xf6, hu00F6 ö`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `ö, 'ö, hö' ö`)

	value = `\x''f6`
	assert.Equal(t, quoteHandler(value, unQuoted), `xf6`)
	assert.Equal(t, quoteHandler(value, singleQuoted), `ö`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `\x''f6`)

	value = `\x""f6`
	assert.Equal(t, quoteHandler(value, unQuoted), `xf6`)
	assert.Equal(t, quoteHandler(value, singleQuoted), `\x""f6`)
	assert.Equal(t, quoteHandler(value, doubleQuoted), `ö`)
}

func TestEscapeLiteralDollars(t *testing.T) {
	assert.Equal(t, escapeLiteralDollars(`\$foo$bar$$`, unQuoted), `\$foo$bar$$`)
	assert.Equal(t, escapeLiteralDollars(`'\$foo$bar$$'`, unQuoted), `'\$foo\$bar\$\$'`)
	assert.Equal(t, escapeLiteralDollars(`'\$foo$bar$$'`, doubleQuoted), `'\$foo$bar$$'`)
	assert.Equal(t, escapeLiteralDollars(`'\$foo$bar$$'`, singleQuoted), `'\$foo$bar$$'`)
	assert.Equal(t, escapeLiteralDollars(`\$foo'$bar$'$`, unQuoted), `\$foo'\$bar\$'$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo'$bar$'$`, doubleQuoted), `\$foo'$bar$'$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo'$bar$'$`, singleQuoted), `\$foo'$bar$'\$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo"$bar$"$`, unQuoted), `\$foo"$bar$"$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo"$bar$"$`, doubleQuoted), `\$foo"$bar$"$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo"$bar$"$`, singleQuoted), `\$foo"\$bar\$"\$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo"'$bar$'"$`, unQuoted), `\$foo"'$bar$'"$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo"'$bar$'"$`, doubleQuoted), `\$foo"'\$bar\$'"$`)
	assert.Equal(t, escapeLiteralDollars(`\$foo"'$bar$'"$`, singleQuoted), `\$foo"'$bar$'"\$`)
}

func TestHandleDefaults(t *testing.T) {
	re := regexp2.MustCompile(paramParserPattern, 0)
	type DefaultParamResults struct {
		Param     string
		ParamName string
		Value     string
		Resolved  bool
		Failing   bool
	}

	t.Setenv("FOO", "foo")
	t.Setenv("BAZ", "")
	for _, param := range []DefaultParamResults{
		{"${FOO:-bar}", "FOO", "foo", true, false},
		{"${FOO-bar}", "FOO", "foo", true, false},
		{"${FOO:+bar}", "FOO", "foo", false, false},
		{"${FOO+bar}", "FOO", "foo", false, false},
		{"${FOO:?bar}", "FOO", "foo", true, false},
		{"${FOO?bar}", "FOO", "foo", true, false},
		{"${BAR:-bar}", "BAR", "", false, false},
		{"${BAR-bar}", "BAR", "", false, false},
		{"${BAR:+bar}", "BAR", "", true, false},
		{"${BAR+bar}", "BAR", "", true, false},
		{"${BAR:?bar}", "BAR", "", false, true},
		{"${BAR?bar}", "BAR", "", false, true},
		{"${BAZ:-baz}", "BAZ", "", false, false},
		{"${BAZ-baz}", "BAZ", "", true, false},
		{"${BAZ:+baz}", "BAZ", "", true, false},
		{"${BAZ+baz}", "BAZ", "", false, false},
		{"${BAZ:?baz}", "BAZ", "", false, true},
		{"${BAZ?baz}", "BAZ", "", true, false},
	} {
		m, _ := re.FindStringMatch((param.Param))
		value, resolved, failing := handleDefaults(m, param.ParamName)
		expected := DefaultParamResults{param.Param, param.ParamName, value, resolved, failing}
		assert.DeepEqual(t, expected, param)
	}

}

func TestParseParamWithDefaults(t *testing.T) {
	assert.Equal(t, parseParam(`$FOO`), ``)
	t.Setenv(`FOO`, `foo`)
	t.Setenv(`BAZ`, ``)
	// Standard
	assert.Equal(t, parseParam(`$FOO`), `foo`)
	assert.Equal(t, parseParam(`${FOO}`), `foo`)
	assert.Equal(t, parseParam(`$BAR`), ``)
	assert.Equal(t, parseParam(`${BAR}`), ``)
	// Undefined operations
	assert.Equal(t, parseParam(`${FOO:-bar}`), `foo`)
	assert.Equal(t, parseParam(`${FOO-bar}`), `foo`)
	assert.Equal(t, parseParam(`${BAR:-bar}`), `bar`)
	assert.Equal(t, parseParam(`${BAR-bar}`), `bar`)
	assert.Equal(t, parseParam(`${BAZ:-bar}`), `bar`)
	assert.Equal(t, parseParam(`${BAZ-bar}`), ``)
	// Defined operations
	assert.Equal(t, parseParam(`${FOO:+bar}`), `bar`)
	assert.Equal(t, parseParam(`${FOO+bar}`), `bar`)
	assert.Equal(t, parseParam(`${BAR:+bar}`), ``)
	assert.Equal(t, parseParam(`${BAR+bar}`), ``)
	assert.Equal(t, parseParam(`${BAZ:+bar}`), ``)
	assert.Equal(t, parseParam(`${BAZ+bar}`), `bar`)
	// Error operations
	assert.Equal(t, parseParam(`${FOO:?bar}`), `foo`)
	assert.Equal(t, parseParam(`${FOO?bar}`), `foo`)
	assertPanic(t, func() { parseParam(`${BAR:?bar}`) }, `bar`)
	assertPanic(t, func() { parseParam(`${BAR?bar}`) }, `bar`)
	assertPanic(t, func() { parseParam(`${BAZ:?bar}`) }, `bar`)
	assert.Equal(t, parseParam(`${BAZ?bar}`), ``)
	// Nested operations
	assert.Equal(t, parseParam(`${FOO:+${BAR-bar}}`), `bar`)
	assert.Equal(t, parseParam(`${FOO+${BAR-${BAZ?baz}}}`), ``)
	assertPanic(t, func() { parseParam(`${FOO+${BAR-${BAZ:?baz}}}`) }, `baz`)
}

func TestParseParamWithCompositeDefaults(t *testing.T) {
	t.Setenv(`FOO`, `foo`)
	assert.Equal(t, parseParam(`${FOO:+${BAR-${FOO}bar}}`), `foobar`)
	assert.Equal(t, parseParam(`${FOO:+${BAR-bar$FOO}}`), `barfoo`)
}

func TestParseParamWithQuotes(t *testing.T) {
	t.Setenv("BAZ", "baz")
	// Bare values
	assert.Equal(t, parserHandler(`'foo'`, unQuoted), `foo`)
	assert.Equal(t, parserHandler(`"bar"`, unQuoted), `bar`)
	assert.Equal(t, parserHandler(`\"baz\"`, unQuoted), `"baz"`)
	// Parsed params
	assert.Equal(t, parserHandler(`${BAR-\"baz\"}`, unQuoted), `"baz"`)
	assert.Equal(t, parserHandler(`${BAR-\"$BAZ\"}`, unQuoted), `"baz"`)
	assert.Equal(t, parserHandler(`${BAR-'$BAZ'}`, unQuoted), `$BAZ`)
	assert.Equal(t, parserHandler(`${BAR-\'$BAZ\'}`, unQuoted), `'baz'`)
	assert.Equal(t, parserHandler(`${BAR-\\"$BAZ\\"}`, unQuoted), "\baz\\")
	assert.Equal(t, parserHandler(`${BAR-\\'$BAZ\\'}`, unQuoted), `\$BAZ\`)
	assert.Equal(t, parserHandler(`${BAR-foo\'$BAZ\'}`, unQuoted), `foo'baz'`)
	assert.Equal(t, parserHandler(`foo${BAR-\'$BAZ\'}`, unQuoted), `foo'baz'`)
	assert.Equal(t, parserHandler(`${BAR-"\'$BAZ\'"}`, unQuoted), `'baz'`)
	assert.Equal(t, parserHandler(`"$BAZ"`, unQuoted), `baz`)
	assert.Equal(t, parserHandler(`${BAR-\'$BAZ\'}`, doubleQuoted), `'baz'`)
	assert.Equal(t, parserHandler(`"${BAR-\'$BAZ\'}"`, unQuoted), `'baz'`)
	assert.Equal(t, parserHandler(`${BAR-"$BAZ"}`, unQuoted), `baz`)
	assert.Equal(t, parserHandler(`${BAR-"$BAZ"}`, doubleQuoted), `baz`)
}

func TestSingleMapParamValues(t *testing.T) {
	t.Setenv("FOO", "foobar")
	values := mapperHandler([]Param{{Id: "$FOO", Position: []int{0, 0}}})
	assert.DeepEqual(t, values, AssocArray{"$FOO": "foobar"})
}

type MockParser struct {
	counter int
}

func (m *MockParser) parseParam(string) string {
	m.counter++
	return "foo"
}

func TestRepeatingMapParamValues(t *testing.T) {
	mock := &MockParser{}
	assert.DeepEqual(
		t,
		mapParamValues([]Param{{Id: "$FOO"}}, mock.parseParam),
		AssocArray{"$FOO": "foo"},
	)
	assert.Equal(t, mock.counter, 1)

	mock.counter = 0
	assert.DeepEqual(
		t,
		mapParamValues([]Param{
			{Id: "$FOO"},
			{Id: "$FOO"},
			{Id: "$BAR"},
			{Id: "$FOO"},
			{Id: "$FOO"},
		}, mock.parseParam),
		AssocArray{"$FOO": "foo", "$BAR": "foo"},
	)
	assert.Equal(t, mock.counter, 2)
}

func TestMapParamValues(t *testing.T) {
	t.Setenv("Q", "quis")
	t.Setenv("FOO", "iaculis")
	t.Setenv("BAR", "fringilla")
	validSlices := getValidSlices(tokenizedPayload)
	params := findParams(payload, validSlices)
	expected := AssocArray{
		"${BAZ:-$BAR}":   "fringilla",
		"${Q}":           "quis",
		"${BAR}":         "fringilla",
		"${BAZ:-${BAR}}": "fringilla",
		"${FOO:+ipsum}":  "ipsum",
		"$FOO":           "iaculis",
	}
	values := mapperHandler(params)
	assert.DeepEqual(t, expected, values)
}

func TestListParams(t *testing.T) {
	params := []Param{
		{Id: "$FOO", Position: []int{0, 0}},
		{Id: "${BAR}", Position: []int{1, 1}},
		{Id: "${BAZ:?${FOO}}", Position: []int{2, 2}},
	}
	expected := `[
  {
    "Param": "$FOO",
    "Index": 0
  },
  {
    "Param": "${BAR}",
    "Index": 1
  },
  {
    "Param": "${BAZ:?${FOO}}",
    "Index": 2
  }
]`
	assert.DeepEqual(t, expected, listParams(params))
}

func TestOutputList(t *testing.T) {
	expected := `[
  {
    "Param": "${BAZ:-$BAR}",
    "Index": 64
  },
  {
    "Param": "${Q}",
    "Index": 215
  },
  {
    "Param": "${Q}",
    "Index": 267
  },
  {
    "Param": "${BAR}",
    "Index": 309
  },
  {
    "Param": "${BAZ:-${BAR}}",
    "Index": 377
  },
  {
    "Param": "${FOO:+ipsum}",
    "Index": 393
  },
  {
    "Param": "$FOO",
    "Index": 473
  },
  {
    "Param": "${Q}",
    "Index": 518
  }
]`
	config := Config{file: loremQuotesPath, list: true}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, expected, output)
}

func TestOutputListEmpty(t *testing.T) {
	noParams := temp{}
	defer noParams.testFile("Ö ö, Hö-ö, Hö-öns Mö.")()
	config := Config{file: noParams.file, list: true}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, `[]`, output)
}

func TestOutputUnset(t *testing.T) {
	expected, _ := os.ReadFile("../lorem_quotes_unset.txt")
	config := Config{file: loremQuotesPath}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)
}

func TestOutputPreserve(t *testing.T) {
	t.Setenv("BAR", "fringilla")
	expected, _ := os.ReadFile("../lorem_quotes_preserve.txt")
	config := Config{file: loremQuotesPath, preserve: true}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)
}

func TestOutput(t *testing.T) {
	t.Setenv("Q", "quis")
	t.Setenv("FOO", "iaculis")
	t.Setenv("BAR", "fringilla")
	expected, _ := os.ReadFile("../lorem_quotes_parsed.txt")
	config := Config{file: loremQuotesPath}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)
}

func TestOutputIgnoreQuotes(t *testing.T) {
	t.Setenv("FOO", "bar")
	quotes := temp{}
	defer quotes.testFile("Lorem '$FOO' ipsum")()
	expected, _ := os.ReadFile(quotes.file)
	config := Config{file: quotes.file}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)

	ignoreQuotes := "Lorem 'bar' ipsum"
	config.ignoreQuotes = true
	ignoreQuotesOutput := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, ignoreQuotes, ignoreQuotesOutput)
}

func TestOutputWithParamlessFile(t *testing.T) {
	temp := temp{}
	defer temp.testFile("Lorem ipsum dolor sit amet\n")()
	expected, _ := os.ReadFile(temp.file)
	config := Config{file: temp.file}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)
}

func TestSetEnv(t *testing.T) {
	defer resetEnv([]string{"FOO", "BAR", "BAZ", "FOOBAR", "INVALID", "OEOE"})()
	t.Setenv("FOO", "bar")
	assert.Equal(t, getEnv("FOO"), "bar")
	assert.Equal(t, getEnv("BAR"), nil)
	assert.Equal(t, getEnv("BAZ"), nil)
	setEnv("FOO=baz\nBAR=${FOO:+$BAZ}\nBAZ=${BAR:-${FOO:+bar}}", false)
	assert.Equal(t, getEnv("FOO"), "baz")
	assert.Equal(t, getEnv("BAR"), "")
	assert.Equal(t, getEnv("BAZ"), "bar")

	assertPanic(t, func() { setEnv("FOOBAR=${UNSET:?bar}", false) }, "bar")
	assert.Equal(t, getEnv("FOOBAR"), nil)
	assertPanic(t, func() { setEnv("FOOBAR=${BAR:?bar}", false) }, "bar")
	assert.Equal(t, getEnv("FOOBAR"), nil)
	setEnv("FOOBAR=${BAR?bar}", false)
	assert.Equal(t, getEnv("FOOBAR"), "")
	assertPanic(t, func() { setEnv("$INVALID=foo", false) }, "Invalid env assignment syntax")
	assertPanic(t, func() { setEnv("INVALID = foo", false) }, "Invalid env assignment syntax")

	assert.Equal(t, getEnv("OEOE"), nil)
	setEnv("OEOE=öfooÖbarö", false)
	assert.Equal(t, getEnv("OEOE"), `öfooÖbarö`)

	setEnv("OEOE=ö'foo$FOOÖ'barö", false)
	assert.Equal(t, getEnv("OEOE"), `öfoo$FOOÖbarö`)
}

func TestSetEnvWithFile(t *testing.T) {
	defer resetEnv([]string{"Q", "FOO", "BAR"})()
	t.Setenv("FOO", "foo")
	assert.Equal(t, getEnv("FOO"), "foo")
	assert.Equal(t, getEnv("Q"), nil)
	assert.Equal(t, getEnv("BAR"), nil)
	envTest, _ := os.ReadFile("../lorem.envtest")
	setEnv(string(envTest), true)
	assert.Equal(t, getEnv("FOO"), "iaculis")
	assert.Equal(t, getEnv("Q"), "quis")
	assert.Equal(t, getEnv("BAR"), "fringilla")
}

func TestSetEnvWithAdvancedFile(t *testing.T) {
	defer resetEnv([]string{"QUIS", "Q", "FOO", "BAR", "BAZ", "BAZ1"})()
	t.Setenv("FOO", "foo")
	assert.Equal(t, getEnv("FOO"), "foo")
	assert.Equal(t, getEnv("Q"), nil)
	assert.Equal(t, getEnv("QQ"), nil)
	assert.Equal(t, getEnv("BAR"), nil)
	assert.Equal(t, getEnv("QUIS"), nil)
	assert.Equal(t, getEnv("BAZ"), nil)
	assert.Equal(t, getEnv("BAZ1"), nil)
	envTest, _ := os.ReadFile("../lorem_advanced.envtest")
	setEnv(string(envTest), true)
	assert.Equal(t, getEnv("FOO"), `'iaculis'`)
	assert.Equal(t, getEnv("Q"), "${QUIS}")
	assert.Equal(t, getEnv("QQ"), "'quis'")
	assert.Equal(t, getEnv("BAR"), "fringilla")
	assert.Equal(t, getEnv("QUIS"), "quis")
	assert.Equal(t, getEnv("BAZ0"), "foobar")
	assert.Equal(t, getEnv("BAZ1"), "foo'bar'")
}

func TestOutputWithEnvFileError(t *testing.T) {
	emptyOutput := temp{}
	emptyEnvFile := temp{}
	defer emptyOutput.testFile(``)()
	defer emptyEnvFile.testFile(``)()
	config := Config{file: emptyOutput.file, envFiles: []string{`iDontExist.txt`}}
	assertPanic(t, func() { GetOutput(config) }, `open iDontExist.txt: no such file or directory`)

	config = Config{file: emptyOutput.file, envFiles: []string{emptyEnvFile.file}}
	assertPanic(t, func() { GetOutput(config) }, `Invalid env assignment syntax`)
}

func TestOutputWithEnvFiles(t *testing.T) {
	defer resetEnv([]string{"QUIS", "Q", "FOO", "BAR", "BAZ0", "BAZ1"})()
	t.Setenv("Q", "wrong")
	t.Setenv("FOO", "wrong")
	t.Setenv("BAR", "wrong")
	config := Config{file: loremQuotesPath, envFiles: []string{`../lorem_advanced.envtest`, `../lorem.envtest`}}
	expected, _ := os.ReadFile("../lorem_quotes_parsed.txt")
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)
	assert.Equal(t, getEnv("FOO"), `iaculis`)
	assert.Equal(t, getEnv("Q"), `quis`)
	assert.Equal(t, getEnv("QQ"), `'quis'`)
	assert.Equal(t, getEnv("BAR"), `fringilla`)
	assert.Equal(t, getEnv("QUIS"), `quis`)
	assert.Equal(t, getEnv("BAZ0"), `foobar`)
	assert.Equal(t, getEnv("BAZ1"), `foo'bar'`)
}

func TestOutputEnvOverrides(t *testing.T) {
	defer resetEnv([]string{"Q", "FOO", "BAR"})()
	t.Setenv("Q", "wrong")
	t.Setenv("FOO", "iaculis")
	t.Setenv("BAR", "wrong")
	overrides := []string{"Q=quis", "BAR=${FOO:+fringilla}"}
	expected, _ := os.ReadFile("../lorem_quotes_parsed.txt")
	config := Config{file: loremQuotesPath, envOverrides: overrides}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)
}

func TestOutputWithEnvFilesAndOverrides(t *testing.T) {
	defer resetEnv([]string{"Q", "FOO", "BAR"})()
	inputFile := temp{}
	defer inputFile.testFile(`Ö, ${BAR+ö,} $FOO ö, ${Q}`)()
	envOverride := []string{`FOO=Hö`, `Q=${BAZ:-Hö-öns Mö}`}
	expected := `Ö, ö, Hö ö, Hö-öns Mö`
	config := Config{file: inputFile.file, envOverrides: envOverride, envFiles: []string{`../lorem.envtest`}}
	output := captureOutput(func() {
		GetOutput(config)
	})
	assert.Equal(t, string(expected), output)
}
