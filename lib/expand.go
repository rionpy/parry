/*
Copyright © 2022 Martti Leino <rionpy@gmail.com>
GNU General Public License v3.0+ (see LICENSE or https://www.gnu.org/licenses/gpl-3.0.txt)
*/
package lib

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

type Param struct {
	Id       string
	Position []int
}

type AssocArray map[string]string

type ParamJson struct {
	Param string
	Index int
}

type Parser func(string) string

var paramName = `[A-Za-z_][A-Za-z0-9_]*`
var paramDefaults = `(?<defaultsOperation>:?[-+?])`

var unescapedToken = `(?<=(?:[^\\]|^)(?:[\\]{2})*)`
var unescapedSingleQuote = fmt.Sprintf(`%s'`, unescapedToken)
var unescapedDoubleQuote = fmt.Sprintf(`%s"`, unescapedToken)
var escapedSingleQuote = fmt.Sprintf(`%s\\'`, unescapedToken)
var escapedDoubleQuote = fmt.Sprintf(`%s\\"`, unescapedToken)

var resolveDoubleQuotesRegex = regexp2.MustCompile(unescapedDoubleQuote, 0)

var resolveAllQuotesRegex = regexp2.MustCompile(fmt.Sprintf(`%[1]s|%[2]s`, unescapedSingleQuote, unescapedDoubleQuote), 0)

var bracedParam = fmt.Sprintf(`(?:%[1]s\$\{(?<braced>%[2]s))(?:(?<braceDepth>%[1]s\$\{%[2]s)|(?:%[1]s[$](?!\{)|[^$}]|%[1]s\\[$])|(?<-braceDepth>\}))*(?(braceDepth)(?!))\}`, unescapedToken, paramName)
var paramFinderPattern = fmt.Sprintf(`%[1]s\$(?<bare>%[2]s)|%[3]s`, unescapedToken, paramName, bracedParam)

var paramParserPattern = fmt.Sprintf(`(?:\$\{%[1]s(?<expansion>(?<defaults>(%[2]s)(?<defaultsValue>.*?)))?\}$)`, paramName, paramDefaults)

var unescapeAllQuotesRegex = regexp2.MustCompile(fmt.Sprintf(`%[1]s|%[2]s`, escapedSingleQuote, escapedDoubleQuote), 0)

var unescapedDollarRegex = regexp2.MustCompile(fmt.Sprintf(`%s\$`, unescapedToken), 0)

var envVarPattern = fmt.Sprintf(`(?<name>%s)=(?<value>.*)`, paramName)
var envParserPattern = fmt.Sprintf(`^%s`, envVarPattern)
var envFileParserPattern = fmt.Sprintf(`^(?:export\s+)?%s`, envVarPattern)

var escapeSequenceRegex = regexp2.MustCompile(fmt.Sprintf(`%s(?:\\\$|(?<quotable>\\(?:[\\abfnrtv]|[0-7]{3}|x[0-9a-fA-F]{2}|u[0-9a-fA-F]{4}|U[0-9a-fA-F]{8})))`, unescapedToken), 0)

var paramFinderRegex = regexp2.MustCompile(paramFinderPattern, 0)
var paramParserRegex = regexp2.MustCompile(paramParserPattern, 0)

type SegmentType int64

const (
	unQuoted SegmentType = iota
	singleQuoted
	doubleQuoted
)

type Segment struct {
	Position []int
	SegmentType
}

var ignoreQuotes = false

// Single quote matching should start with an unescaped single quote and end in any single quote,
// as escape sequences are not evaluated for string literals
var quoteTokenizerPattern = fmt.Sprintf(`(?<singleQuoted>%[1]s([\n\r]|.)*?')|(?<doubleQuoted>%[2]s([\n\r]|.)*?%[2]s)`, unescapedSingleQuote, unescapedDoubleQuote)

func tokenizeByQuotes(payload []rune) []Segment {
	re := regexp2.MustCompile(quoteTokenizerPattern, 0)
	m, _ := re.FindRunesMatch(payload)
	var segments []Segment
	lastIndex := 0
	for m != nil {
		sType := unQuoted
		if lastIndex != m.Index {
			segments = append(segments, Segment{Position: []int{lastIndex, m.Index}, SegmentType: sType})
		}
		if g := m.GroupByName("singleQuoted"); g.Length > 0 {
			sType = singleQuoted
		} else if g := m.GroupByName("doubleQuoted"); g.Length > 0 {
			sType = doubleQuoted
		} else {
			panic("could not parse quotes")
		}
		lastIndex = m.Index + m.Length
		segments = append(segments, Segment{Position: []int{m.Index, lastIndex}, SegmentType: sType})

		m, _ = re.FindNextMatch(m)
	}
	if lastIndex != len(payload) {
		segments = append(segments, Segment{Position: []int{lastIndex, len(payload)}, SegmentType: unQuoted})
	}
	return segments
}

func handleDefaults(match *regexp2.Match, param string) (string, bool, bool) {
	operation := match.GroupByName("defaultsOperation").String()
	value, isSet := os.LookupEnv(param)
	emptyEqualsUnset := operation[0:1] == ":"
	resolved := true
	failing := false

	switch operation[len(operation)-1:] {
	case "-":
		if (len(value) == 0 && emptyEqualsUnset) || !isSet {
			resolved = false
		}
	case "+":
		if len(value) > 0 || (isSet && !emptyEqualsUnset) {
			resolved = false
		}
	case "?":
		if (len(value) == 0 && emptyEqualsUnset) || !isSet {
			resolved = false
			failing = true
		}
	}

	return value, resolved, failing
}

func unescaper(m regexp2.Match) string {
	return m.String()[1:]
}

func escapeLiteralDollars(param string, parent SegmentType) string {
	switch parent {
	case singleQuoted:
		param = `'` + param + `'`
	case doubleQuoted:
		param = `"` + param + `"`
	}
	payload := []rune(param)
	segments := tokenizeByQuotes(payload)
	var results []string
	var runner *string

	for _, segment := range segments {
		results = append(results, string(payload[segment.Position[0]:segment.Position[1]]))
		runner = &results[len(results)-1]
		if segment.SegmentType == singleQuoted {
			*runner, _ = unescapedDollarRegex.Replace(*runner, `\$`, -1, -1)
		}
	}

	param = strings.Join(results, ``)
	if parent != unQuoted {
		param = param[1 : len(param)-1]
	}
	return param
}

func escapeHandler(param string) string {
	runes := []rune(param)
	m, _ := escapeSequenceRegex.FindRunesMatch(runes)
	var result string
	var replacementRune rune
	var replacement string
	lastIndex := 0
	for m != nil {
		matchStr := m.String()
		if matchStr == `\$` {
			replacement = `$`
		} else if m.GroupByName(`quotable`).Length > 0 {
			replacementRune, _, _, _ = strconv.UnquoteChar(matchStr, 0)
			replacement = string(replacementRune)
		} else {
			replacement = matchStr
		}
		result += string(runes[lastIndex:m.Index]) + replacement
		lastIndex = m.Index + m.Length
		m, _ = escapeSequenceRegex.FindNextMatch(m)
	}
	result += string(runes[lastIndex:])
	return result
}

func quoteHandler(param string, parent SegmentType) string {
	switch parent {
	case singleQuoted:
		param = `'` + param + `'`
	case doubleQuoted:
		param = `"` + param + `"`
	}
	payload := []rune(param)
	segments := tokenizeByQuotes(payload)
	var results []string
	var runner *string

	for _, segment := range segments {
		results = append(results, string(payload[segment.Position[0]:segment.Position[1]]))
		runner = &results[len(results)-1]
		if segment.SegmentType == unQuoted {
			if m, _ := resolveAllQuotesRegex.FindStringMatch(*runner); m != nil {
				panic(fmt.Sprintf(`unmatched quote %[1]s in: %[2]s`, m.String(), param))
			}
			re := regexp2.MustCompile(`\\.`, 0)
			*runner, _ = resolveAllQuotesRegex.Replace(*runner, ``, -1, -1)
			*runner, _ = re.ReplaceFunc(*runner, unescaper, -1, -1)
		} else {
			// Strip helper quotes
			if parent == segment.SegmentType || parent == unQuoted {
				*runner = (*runner)[1 : len(*runner)-1]
			}
			if segment.SegmentType == doubleQuoted {
				*runner, _ = resolveDoubleQuotesRegex.Replace(*runner, ``, -1, -1)
				*runner, _ = unescapeAllQuotesRegex.ReplaceFunc(*runner, unescaper, -1, -1)
			}
		}
	}
	result := strings.Join(results, ``)
	return escapeHandler(result)
}

func embeddedParser(m regexp2.Match) string {
	return parseParam(m.String())
}

func parseEmbeddedParams(value string) string {
	re := regexp2.MustCompile(paramFinderPattern, 0)
	value, _ = re.ReplaceFunc(value, embeddedParser, -1, -1)

	return value
}

func parserHandler(param string, parent SegmentType) string {
	param = escapeLiteralDollars(param, parent)
	param = parseParam(param)

	param = quoteHandler(param, parent)
	return param
}

func parseParam(param string) string {
	runes := []rune(param)
	var value string
	var resolved bool
	var failing bool
	if finderMatch, _ := paramFinderRegex.FindStringMatch(param); finderMatch != nil {
		if bare := finderMatch.GroupByName("bare"); bare.Length > 0 {
			value = os.Getenv(bare.String())
		} else if braced := finderMatch.GroupByName("braced"); braced.Length > 0 {
			value = os.Getenv(braced.String())
			if parserMatch, _ := paramParserRegex.FindStringMatch(finderMatch.String()); parserMatch != nil {
				if defaults := parserMatch.GroupByName("defaults"); defaults.Length > 0 {
					value, resolved, failing = handleDefaults(parserMatch, braced.String())
					if !resolved {
						value = parseEmbeddedParams(parserMatch.GroupByName("defaultsValue").String())
					}
					if failing {
						panic(value)
					}
				}
			}
		} else {
			value = param
		}
		value = string(runes[:finderMatch.Index]) + value + string(runes[finderMatch.Index+finderMatch.Length:])
	} else {
		value = param
	}

	return value
}

func mapperHandler(params []Param) AssocArray {
	return mapParamValues(params, parseParam)
}

func mapParamValues(params []Param, parser Parser) AssocArray {
	values := AssocArray{}
	for _, param := range params {
		if _, ok := values[param.Id]; !ok {
			values[param.Id] = parser(param.Id)
		}
	}
	return values
}

func MatchesToIndices(re *regexp2.Regexp, payload []rune) [][]int {
	var slices [][]int
	m, _ := re.FindRunesMatch(payload)
	for m != nil {
		slices = append(slices, []int{m.Index, m.Index + m.Length})
		m, _ = re.FindNextMatch(m)
	}
	return slices
}

func findParams(payload []rune, validSlices [][]int) []Param {
	var params []Param
	re := regexp2.MustCompile(paramFinderPattern, 0)
	paramMatches := MatchesToIndices(re, payload)
	for _, param := range paramMatches {
		for _, validSlice := range validSlices {
			if InRange(param[0], validSlice) {
				params = append(params, Param{string(payload[param[0]:param[1]]), param})
				break
			}
		}
	}
	return params
}

func readToRunes(path string, stdIn bool) []rune {
	var result string
	if stdIn {
		bytes, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}
		result = string(bytes)
	} else {
		bytes, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		result = string(bytes)
	}

	return []rune(result)
}

func listParams(params []Param) string {
	jsonParams := []ParamJson{}
	for _, param := range params {
		jsonParams = append(jsonParams, ParamJson{Param: param.Id, Index: param.Position[0]})
	}
	result, err := json.MarshalIndent(jsonParams, "", "  ")

	if err != nil {
		panic(err)
	}

	return string(result)
}

type Config struct {
	file          string
	readFromStdin bool
	list          bool
	preserve      bool
	ignoreQuotes  bool
	envOverrides  []string
	envFiles      []string
	interpret     string
	editInPlace   bool
}

func (c *Config) Validate() {
	if _, err := os.Stat(c.file); errors.Is(err, os.ErrNotExist) {
		c.editInPlace = false

		if in, _ := os.Stdin.Stat(); in.Mode()&os.ModeNamedPipe == 0 {
			panic(`file missing`)
		} else {
			c.readFromStdin = true
		}
	}
}

func (c *Config) SetList() {
	c.list = true
}

func (c *Config) SetPreserve() {
	c.preserve = true
}

func (c *Config) SetIgnore() {
	c.ignoreQuotes = true
}

func (c *Config) SetInterpret(val string) {
	c.interpret = val
}

func (c *Config) SetEditInPlace() {
	c.editInPlace = true
}

func (c *Config) AddOverride(payload string) {
	c.envOverrides = append(c.envOverrides, payload)
}

func (c *Config) AddEnvFile(path string) {
	c.envFiles = append(c.envFiles, path)
}

func (c *Config) AddFile(path string) {
	c.file = path
}

func setEnv(payload string, isFile bool) {
	var re *regexp2.Regexp
	if isFile {
		re = regexp2.MustCompile(envFileParserPattern, regexp2.Multiline)
	} else {
		re = regexp2.MustCompile(envParserPattern, regexp2.Multiline)
	}
	m, _ := re.FindStringMatch(payload)
	if m == nil {
		panic("Invalid env assignment syntax")
	}
	for m != nil {
		os.Setenv(m.GroupByName("name").String(), parserHandler(m.GroupByName("value").String(), unQuoted))
		m, _ = re.FindNextMatch(m)
	}
}

func GetOutput(config Config) {
	var validSlices [][]int
	payload := readToRunes(config.file, config.readFromStdin)
	ignoreQuotes = config.ignoreQuotes
	if config.ignoreQuotes {
		validSlices = [][]int{{0, len(payload)}}
	} else {
		validSlices = getValidSlices(tokenizeByQuotes(payload))
	}
	for _, envFilePath := range config.envFiles {
		envFile, envFileErr := os.ReadFile(envFilePath)
		if envFileErr != nil {
			panic(envFileErr)
		}
		setEnv(string(envFile), true)
	}
	if overrides := config.envOverrides; len(overrides) > 0 {
		setEnv(strings.Join(overrides, "\n"), false)
	}
	params := findParams(payload, validSlices)
	if config.list {
		fmt.Print(listParams(params))
	} else {
		file := os.Stdout
		if len(params) > 0 {
			values := mapperHandler(params)
			if config.editInPlace {
				file, _ = os.Create(config.file)
			}
			firstIndex := 0
			for _, param := range params {
				if param.Position[0] != firstIndex {
					fmt.Fprint(file, string(payload[firstIndex:param.Position[0]]))
				}
				if value := values[param.Id]; len(value) == 0 && config.preserve {
					fmt.Fprint(file, param.Id)
				} else {
					fmt.Fprint(file, values[param.Id])
				}
				firstIndex = param.Position[1]
			}
			fmt.Fprint(file, string(payload[firstIndex:]))
		} else if !config.editInPlace {
			fmt.Print(string(payload))
		}
	}
}

func getValidSlices(segments []Segment) [][]int {
	var valid [][]int
	for _, segment := range segments {
		if segment.SegmentType != singleQuoted {
			if len(valid) > 0 && valid[len(valid)-1][1] == segment.Position[0] {
				valid[len(valid)-1][1] = segment.Position[1]
			} else {
				valid = append(valid, segment.Position)
			}
		}
	}
	return valid
}

func InRange(i int, slice []int) bool {
	return (i >= slice[0]) && (i < slice[1])
}
