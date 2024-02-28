// This file is Free Software under the MIT License
// without warranty, see README.md and LICENSES/MIT.txt for details.
//
// SPDX-License-Identifier: Apache-2.0
//
// SPDX-FileCopyrightText: 2024 German Federal Office for Information Security (BSI) <https://www.bsi.bund.de>
// Software-Engineering: 2024 Intevation GmbH <https://intevation.de>

package database

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type parseError string

type exprType int

const (
	cnst exprType = iota
	cast
	and
	or
	not
	eq
	ne
	gt
	lt
	ge
	le
	access
	search
)

type valueType int

const (
	intType valueType = iota
	floatType
	boolType
	stringType
	timeType
	workflowType
)

// Expr encapsulates a parsed expression to be converted to an SQL WHERE clause.
type Expr struct {
	exprType  exprType
	valueType valueType

	stringValue string
	intValue    int64
	floatValue  float64
	timeValue   time.Time
	boolValue   bool
	langValue   string
	alias       string

	children []*Expr
}

type documentColumn struct {
	name           string
	valueType      valueType
	projectionOnly bool
}

// String implements [fmt.Stringer].
func (vt valueType) String() string {
	switch vt {
	case intType:
		return "int"
	case floatType:
		return "float"
	case boolType:
		return "bool"
	case stringType:
		return "string"
	case timeType:
		return "time"
	case workflowType:
		return "workflow"
	default:
		return fmt.Sprintf("unknown value type %d", vt)
	}
}

// String implements [fmt.Stringer].
func (et exprType) String() string {
	switch et {
	case cnst:
		return "constant"
	case cast:
		return "cast"
	case and:
		return "and"
	case or:
		return "or"
	case not:
		return "not"
	case eq:
		return "eq"
	case ne:
		return "ne"
	case gt:
		return "gt"
	case lt:
		return "lt"
	case ge:
		return "ge"
	case le:
		return "le"
	case access:
		return "access"
	case search:
		return "search"
	default:
		return fmt.Sprintf("unknown expression type %d", et)
	}
}

func (pe parseError) Error() string {
	return string(pe)
}

var columns = []documentColumn{
	{"id", intType, false},
	{"state", workflowType, false},
	{"tracking_id", stringType, false},
	{"version", stringType, false},
	{"publisher", stringType, false},
	{"current_release_date", timeType, false},
	{"initial_release_date", timeType, false},
	{"title", stringType, false},
	{"tlp", stringType, false},
	{"cvss_v2_score", floatType, false},
	{"cvss_v3_score", floatType, false},
	{"four_cves", stringType, true},
}

// TODO: make this configurable?
var supportedLangs = []string{
	"english",
	"german",
}

// CreateOrder returns a ORDER BY clause for given columns.
func CreateOrder(
	fields []string,
	aliases map[string]string,
) (string, error) {
	var b strings.Builder
	for _, field := range fields {
		desc := strings.HasPrefix(field, "-")
		if desc {
			field = field[1:]
		}
		if _, found := aliases[field]; !found && !ExistsDocumentColumn(field) {
			return "", fmt.Errorf("order field %q does not exists", field)
		}
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(field)
		if desc {
			b.WriteString(" DESC")
		} else {
			b.WriteString(" ASC")
		}
	}
	return b.String(), nil
}

// CheckProjections checks if the requested projections are valid.
func CheckProjections(proj []string, aliases map[string]string) error {
	for _, p := range proj {
		if _, found := aliases[p]; found {
			continue
		}
		if !ExistsDocumentColumn(p) {
			return fmt.Errorf("column %q does not exists", p)
		}
	}
	return nil
}

// CreateCountSQL returns an SQL count statement to count
// the number of rows which are possible to fetch by the
// given filter.
func CreateCountSQL(where string, hasAliases bool) string {
	var from string
	if hasAliases {
		from = `extended_documents JOIN documents_texts ON id = documents_id`
	} else {
		from = `extended_documents`
	}
	return "SELECT count(*) FROM " + from + " WHERE " + where
}

// CreateQuerySQL creates an SQL statement to query the documents
// table and the associated texts if needed.
// WARN: Make sure that the iput is vetted against injections.
func CreateQuerySQL(
	fields []string,
	aliases map[string]string,
	where string,
	order string,
	limit, offset int64,
) string {
	projs := projectionsWithCasts(fields, aliases)

	var from string
	if len(aliases) == 0 {
		from = `extended_documents`
	} else {
		from = `extended_documents JOIN documents_texts ON id = documents_id`
	}

	sql := "SELECT " + projs + " FROM " + from + " WHERE " + where

	if order != "" {
		sql += " ORDER BY " + order
	}

	if limit > 0 {
		sql += " LIMIT " + strconv.FormatInt(limit, 10)
	}
	if offset > 0 {
		sql += " OFFSET " + strconv.FormatInt(offset, 10)
	}

	return sql
}

// projectionsWithCasts joins given projection adding casts if needed.
func projectionsWithCasts(proj []string, aliases map[string]string) string {
	var b strings.Builder
	for i, p := range proj {
		if i > 0 {
			b.WriteByte(',')
		}
		if alias, found := aliases[p]; found {
			b.WriteString(alias)
			continue
		}
		b.WriteString(p)
		if p == "state" {
			b.WriteString("::text")
		}
	}
	return b.String()
}

// ExistsDocumentColumn returns true if a column in document exists.
func ExistsDocumentColumn(name string) bool {
	return findDocumentColumn(name) != nil
}

func findDocumentColumn(name string) *documentColumn {
	for i := range columns {
		if col := &columns[i]; col.name == name {
			return col
		}
	}
	return nil
}

// And concats two expressions and-wise.
func (e *Expr) And(o *Expr) *Expr {
	if e.valueType != boolType || o.valueType != boolType {
		return falseExpr()
	}
	if e.exprType == cnst {
		if !e.boolValue {
			return falseExpr()
		}
		return o
	}
	if o.exprType == cnst {
		if !o.boolValue {
			return falseExpr()
		}
		return e
	}
	return &Expr{
		exprType:  and,
		valueType: boolType,
		children:  []*Expr{e, o},
	}
}

// Where returns an SQL WHERE clause and a list of string replacements
// to be fed as separate args to the SQL statement to prevent injections.
func (e *Expr) Where() (string, []any, map[string]string) {
	var b strings.Builder
	var replacements []any
	stringToReplacement := map[string]int{}
	var aliases map[string]string

	replacementIndex := func(s string) int {
		if idx, ok := stringToReplacement[s]; ok {
			return idx
		}
		idx := len(replacements)
		stringToReplacement[s] = idx
		replacements = append(replacements, s)
		return idx
	}

	var recurse func(*Expr)

	writeSearch := func(e *Expr) {
		const tsquery = `websearch_to_tsquery`

		b.WriteString(`ts @@ ` + tsquery + `('`)
		b.WriteString(e.langValue)
		b.WriteString("',$")
		idx := replacementIndex(e.stringValue)
		b.WriteString(strconv.Itoa(idx + 1))
		b.WriteByte(')')
		// Handle alias
		if e.alias == "" {
			return
		}
		repl := fmt.Sprintf(
			"ts_headline('%[1]s',txt,"+tsquery+"('%[1]s', $%[2]d))",
			e.langValue, idx+1)
		if aliases == nil {
			aliases = map[string]string{}
		}
		aliases[e.alias] = repl
	}

	writeCast := func(e *Expr) {
		b.WriteString("CAST(")
		recurse(e.children[0])
		b.WriteString(" AS ")
		switch e.valueType {
		case stringType:
			b.WriteString("text")
		case intType:
			b.WriteString("int")
		case floatType:
			b.WriteString("float")
		case timeType:
			b.WriteString("timestamptz")
		case boolType:
			b.WriteString("boolean")
		case workflowType:
			b.WriteString("workflow")
		}
		b.WriteByte(')')
	}

	writeCnst := func(e *Expr) {
		switch e.valueType {
		case stringType:
			b.WriteByte('$')
			idx := replacementIndex(e.stringValue)
			b.WriteString(strconv.Itoa(idx + 1))
		case intType:
			b.WriteString(strconv.FormatInt(e.intValue, 10))
		case floatType:
			b.WriteString(strconv.FormatFloat(e.floatValue, 'f', -1, 64))
		case timeType:
			b.WriteByte('\'')
			utc := e.timeValue.UTC()
			b.WriteString(utc.Format("2006-01-02T15:04:05-0700"))
			b.WriteString("'::timestamptz")
		case boolType:
			if e.boolValue {
				b.WriteString("TRUE")
			} else {
				b.WriteString("FALSE")
			}
		case workflowType:
			b.WriteByte('\'')
			b.WriteString(e.stringValue)
			b.WriteString("'::workflow")
		}
	}

	writeBinary := func(e *Expr, op string) {
		b.WriteByte('(')
		recurse(e.children[0])
		b.WriteString(op)
		recurse(e.children[1])
		b.WriteByte(')')
	}

	writeNot := func(e *Expr) {
		b.WriteString("(NOT ")
		recurse(e.children[0])
		b.WriteByte(')')
	}

	recurse = func(e *Expr) {
		b.WriteByte('(')
		switch e.exprType {
		case access:
			b.WriteString(e.stringValue)
		case cnst:
			writeCnst(e)
		case cast:
			writeCast(e)
		case eq:
			writeBinary(e, "=")
		case ne:
			writeBinary(e, "<>")
		case lt:
			writeBinary(e, "<")
		case gt:
			writeBinary(e, ">")
		case le:
			writeBinary(e, "<=")
		case ge:
			writeBinary(e, ">=")
		case not:
			writeNot(e)
		case and:
			writeBinary(e, "AND")
		case or:
			writeBinary(e, "OR")
		case search:
			writeSearch(e)
		}
		b.WriteByte(')')
	}
	recurse(e)
	return b.String(), replacements, aliases
}

type stack []*Expr

func (st *stack) push(v *Expr) {
	*st = append(*st, v)
}

func (st *stack) pop() *Expr {
	if l := len(*st); l > 0 {
		x := (*st)[l-1]
		(*st)[l-1] = nil
		*st = (*st)[:l-1]
		return x
	}
	panic(parseError("stack empty"))
}

func (st stack) top() *Expr {
	if l := len(st); l > 0 {
		return st[l-1]
	}
	panic(parseError("stack empty"))
}

func falseExpr() *Expr {
	return &Expr{
		exprType:  cnst,
		valueType: boolType,
		boolValue: false,
	}
}

func trueExpr() *Expr {
	return &Expr{
		exprType:  cnst,
		valueType: boolType,
		boolValue: true,
	}
}

func (st *stack) pushTrue()  { st.push(trueExpr()) }
func (st *stack) pushFalse() { st.push(falseExpr()) }

func (st *stack) pushString(s string) {
	st.push(&Expr{
		exprType:    cnst,
		valueType:   stringType,
		stringValue: s,
	})
}

func (e *Expr) checkValueType(vt valueType) {
	if e.valueType != vt {
		panic(parseError(
			fmt.Sprintf("value type mismatch: %s %s", e.valueType, vt)))
	}
}

func (e *Expr) checkExprType(et exprType) {
	if e.exprType != et {
		panic(parseError(
			fmt.Sprintf("expression type mismatch: %s %s", e.exprType, et)))
	}
}

func (st *stack) not() {
	e := st.pop()
	e.checkValueType(boolType)
	st.push(&Expr{
		exprType:  not,
		valueType: boolType,
		children:  []*Expr{e},
	})
}

func (st *stack) binary(et exprType) {
	right := st.pop()
	left := st.pop()
	left.checkValueType(boolType)
	right.checkValueType(boolType)
	st.push(&Expr{
		exprType:  et,
		valueType: boolType,
		children:  []*Expr{left, right},
	})
}

func (st *stack) access(field string) {
	col := findDocumentColumn(field)
	if col == nil {
		panic(parseError(fmt.Sprintf("unknown column %q", field)))
	}
	if col.projectionOnly {
		panic(parseError(fmt.Sprintf("column %q is for projection only", field)))
	}
	st.push(&Expr{
		exprType:    access,
		valueType:   col.valueType,
		stringValue: field,
	})
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(parseError(fmt.Sprintf("%q is not a float: %v", s, err)))
	}
	return v
}

func parseInt(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(parseError(fmt.Sprintf("%q is not an int: %v", s, err)))
	}
	return v
}

func (st *stack) float() {
	if st.top().valueType == floatType {
		return
	}
	switch e := st.pop(); e.exprType {
	case cnst:
		switch e.valueType {
		case stringType:
			st.push(&Expr{
				exprType:   cnst,
				valueType:  floatType,
				floatValue: parseFloat(e.stringValue),
			})
		case intType:
			st.push(&Expr{
				exprType:   cnst,
				valueType:  intType,
				floatValue: float64(e.intValue),
			})
		}
	default:
		switch e.valueType {
		case stringType, intType:
			st.push(&Expr{
				exprType:  cast,
				valueType: floatType,
				children:  []*Expr{e},
			})
		default:
			panic(parseError("unsupported cast"))
		}
	}
}

func (st *stack) integer() {
	if st.top().valueType == intType {
		return
	}
	switch e := st.pop(); e.exprType {
	case cnst:
		switch e.valueType {
		case stringType:
			st.push(&Expr{
				exprType:  cnst,
				valueType: intType,
				intValue:  parseInt(e.stringValue),
			})
		case floatType:
			st.push(&Expr{
				exprType:  cnst,
				valueType: intType,
				intValue:  int64(e.floatValue),
			})
		}
	default:
		switch e.valueType {
		case stringType, floatType:
			st.push(&Expr{
				exprType:  cast,
				valueType: intType,
				children:  []*Expr{e},
			})
		default:
			panic(parseError("unsupported cast"))
		}
	}
}

func parseTime(s string) time.Time {
	for _, format := range []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01-02T15:04:05-0700",
		"2006-01-02 15:04:05-0700",
	} {
		t, err := time.Parse(format, s)
		if err == nil {
			return t
		}
	}
	panic(parseError(fmt.Sprintf("cannot parse %q as time", s)))
}

func (st *stack) time() {
	if st.top().valueType == timeType {
		return
	}
	switch e := st.pop(); e.exprType {
	case cnst:
		switch e.valueType {
		case stringType:
			st.push(&Expr{
				exprType:  cnst,
				valueType: timeType,
				timeValue: parseTime(e.stringValue),
			})
		}
	default:
		switch e.valueType {
		case stringType:
			st.push(&Expr{
				exprType:  cast,
				valueType: timeType,
				children:  []*Expr{e},
			})
		default:
			panic(parseError("unsupported cast"))
		}
	}
}

func (st *stack) cmp(et exprType) {
	right := st.pop()
	left := st.pop()
	if right.valueType != left.valueType {
		panic(parseError("incompatible types"))
	}
	st.push(&Expr{
		exprType:  et,
		valueType: boolType,
		children:  []*Expr{left, right},
	})
}

var validWorkflows = []string{
	"new", "read", "assessing",
	"review", "archive", "delete",
}

func parseWorkflow(s string) string {
	if !slices.Contains(validWorkflows, s) {
		panic(parseError(fmt.Sprintf("%q is not a valid workflow", s)))
	}
	return s
}

func (st *stack) workflow() {
	if st.top().valueType == workflowType {
		return
	}
	switch e := st.pop(); e.exprType {
	case cnst:
		switch e.valueType {
		case stringType:
			st.push(&Expr{
				exprType:    cnst,
				valueType:   workflowType,
				stringValue: parseWorkflow(e.stringValue),
			})
		}
	default:
		switch e.valueType {
		case stringType:
			st.push(&Expr{
				exprType:  cast,
				valueType: workflowType,
				children:  []*Expr{e},
			})
		default:
			panic(parseError("unsupported cast"))
		}
	}
}

func (st *stack) search() {
	lang := st.pop()
	term := st.pop()
	lang.checkValueType(stringType)
	term.checkValueType(stringType)
	if !slices.Contains(supportedLangs, lang.stringValue) {
		panic(parseError(
			fmt.Sprintf("unsupported search language %q", lang.stringValue)))
	}
	st.push(&Expr{
		exprType:    search,
		valueType:   boolType,
		langValue:   lang.stringValue,
		stringValue: term.stringValue,
	})
}

var aliasRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z_0-9]*`)

func validAlias(s string) {
	if !aliasRe.MatchString(s) {
		panic(parseError(fmt.Sprintf("invalid alias %q", s)))
	}
}

func (st *stack) as(aliases map[string]struct{}) {
	alias := st.pop()
	srch := st.top()
	alias.checkValueType(stringType)
	srch.checkExprType(search)
	validAlias(alias.stringValue)
	if _, already := aliases[alias.stringValue]; already {
		panic(parseError(fmt.Sprintf("duplicate alias %q", alias.stringValue)))
	}
	aliases[alias.stringValue] = struct{}{}
	srch.alias = alias.stringValue
}

func split(input string, fn func(string, bool)) {
	var b strings.Builder
	state := 0
	for _, r := range input {
		switch state {
		case 0: // white space
			switch r {
			case '"':
				state = 1
			case '\\':
				state = 2
			default:
				if !unicode.IsSpace(r) {
					b.WriteRune(r)
					state = 3
				}
			}
		case 1: // quoted string
			switch r {
			case '\\':
				state = 5
			case '"':
				fn(b.String(), true)
				b.Reset()
				state = 0
			default:
				b.WriteRune(r)
			}
		case 2: // \ in white space
			b.WriteRune(r)
			state = 3
		case 3: // unquoted string
			if r == '\\' {
				state = 4
			} else if unicode.IsSpace(r) {
				fn(b.String(), false)
				b.Reset()
				state = 0
			} else {
				b.WriteRune(r)
			}
		case 4: // \ in unquoted string
			b.WriteRune(r)
			state = 3
		case 5: // \ in quoted string
			b.WriteRune(r)
			state = 1
		}
	}
	if state != 0 {
		fn(b.String(), state == 1 || state == 5)
	}
}

func parse(input string) (*Expr, error) {
	st := stack{}
	aliases := map[string]struct{}{}

	split(input, func(field string, isString bool) {

		if isString {
			st.pushString(field)
			return
		}

		switch field {
		case "true":
			st.pushTrue()
		case "false":
			st.pushFalse()
		case "not":
			st.not()
		case "and":
			st.binary(and)
		case "or":
			st.binary(or)
		case "float":
			st.float()
		case "int":
			st.integer()
		case "time":
			st.time()
		case "workflow":
			st.workflow()
		case "=":
			st.cmp(eq)
		case "!=":
			st.cmp(ne)
		case "<":
			st.cmp(lt)
		case "<=":
			st.cmp(le)
		case ">":
			st.cmp(gt)
		case ">=":
			st.cmp(ge)
		case "search":
			st.search()
		case "as":
			st.as(aliases)
		default:
			if strings.HasPrefix(field, "$") {
				st.access(field[1:])
			} else {
				st.pushString(field)
			}
		}
	})

	if len(st) != 1 {
		return nil, parseError(fmt.Sprintf(
			"invalid number of expression roots: expected 1 have %d", len(st)))
	}
	e := st[len(st)-1]
	if e.valueType != boolType {
		return nil, parseError("not a boolean expression")
	}
	return e, nil
}

// Parse returns an expression.
func Parse(input string) (expr *Expr, err error) {
	defer func() {
		if x := recover(); x != nil {
			if pe, ok := x.(parseError); ok {
				err = pe
			} else {
				panic(x)
			}
		}
	}()
	return parse(input)
}

// MustParse parses the given input to an expression.
// If the parsing failed it panics.
func MustParse(input string) *Expr {
	expr, err := Parse(input)
	if err != nil {
		panic(fmt.Sprintf("parsing %q failed: %v", input, err))
	}
	return expr
}
