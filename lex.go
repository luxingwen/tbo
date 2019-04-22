package tbo

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type itemType int

const (
	itemError itemType = iota
	itemNIL            // used in the parser to indicate no type
	itemEOF

	itemCommentStart
	itemText
	itemExt

	itemGroupStart
	itemGroupEnd

	itemExeclStart
	itemExeclEnd

	itemPath

	itemDefinedName
	itemDefinedTypeStart
	itemDefinedTypeEnd

	itemBindStart
	itemBindMin
	itemBindMax
	itemBindValue
	itemBindEnd

	itemField

	itemIndexStart
	itemIndexBind
	itemIndexEnd

	itemAllSetStart
	itemAllSetEnd

	itemTypeName

	itemPrintStart
	itemPrinter
	itemPrintOp
	itemPrintEnd
)

func (it itemType) String() (s string) {
	switch it {
	case itemError:
		s = "itemError"
	case itemNIL:
		s = "itemNIL"
	case itemEOF:
		s = "itemEOF"
	case itemCommentStart:
		s = "itemCommentStart"
	case itemExt:
		s = "itemExt"
	case itemText:
		s = "itemText"
	case itemPath:
		s = "itemPath"
	case itemGroupStart:
		s = "itemGroupStart"
	case itemGroupEnd:
		s = "itemGroupEnd"
	case itemExeclStart:
		s = "itemExeclStart"
	case itemExeclEnd:
		s = "itemExeclEnd"
	case itemDefinedName:
		s = "itemDefinedName"
	case itemDefinedTypeStart:
		s = "itemDefinedTypeStart"
	case itemDefinedTypeEnd:
		s = "itemDefinedTypeEnd"
	case itemBindStart:
		s = "itemBindStart"
	case itemBindMin:
		s = "itemBindMin"
	case itemBindMax:
		s = "itemBindMax"
	case itemBindValue:
		s = "itemBindValue"
	case itemBindEnd:
		s = "itemBindEnd"
	case itemField:
		s = "itemField"
	case itemIndexStart:
		s = "itemIndexStart"
	case itemIndexBind:
		s = "itemIndexBind"
	case itemIndexEnd:
		s = "itemIndexEnd"
	case itemTypeName:
		s = "itemTypeName"
	case itemPrintStart:
		s = "itemPrintStart"
	case itemPrinter:
		s = "itemPrinter"
	case itemPrintOp:
		s = "itemPrintOp"
	case itemPrintEnd:
		s = "itemPrintEnd"
	case itemAllSetStart:
		s = "itemAllSetStart"
	case itemAllSetEnd:
		s = "itemAllSetEnd"
	}
	return
}

const (
	eof               = 0
	space             = ' '
	comma             = ','
	dot               = '.'
	colon             = ':'
	equal             = '='
	underline         = '_'
	out               = '>'
	doubleQuotes      = '"'
	anySplit          = '|'
	commentStart      = '#'
	objectStart       = '@'
	execlPathsStart   = '['
	execlPathsEnd     = ']'
	bindRangeStart    = '['
	bindRangeEnd      = ']'
	printStart        = '['
	printEnd          = ']'
	bodyStart         = '{'
	bodyEnd           = '}'
	anyChildTypeStart = '('
	anyChildTypeEnd   = ')'
	extStart          = '`'
	extEnd            = '`'
	bindStart         = '<'
	bindEnd           = '>'
	indexStart        = '<'
	indexEnd          = '>'
)

type stateFn func(lx *lexer) stateFn

type lexer struct {
	input string
	start int
	pos   int
	line  int
	state stateFn
	items chan item

	currLines [3]int

	// Allow for backing up up to three runes.
	// This is necessary because TOML contains 3-rune tokens (""" and ''').
	prevWidths [3]int
	nprev      int // how many of prevWidths are in use
	// If we emit an eof, we can still back up, but it is not OK to call
	// next again.
	atEOF bool

	// A stack of state functions used to maintain context.
	// The idea is to reuse parts of the state machine in various places.
	// For example, values can appear at the top level or within arbitrarily
	// nested arrays. The last state on the stack is used after a value has
	// been lexed. Similarly for comments.
	stack []stateFn
}

type item struct {
	typ  itemType
	val  string
	line int
	pos  int
}

func (lx *lexer) nextItem() item {
	return <-lx.items
}

func lex(s string) *lexer {
	lx := &lexer{
		state: lexTop,
		line:  1,
		items: make(chan item, global.lexCache),
		stack: make([]stateFn, 0, global.lexCache),
	}
	var (
		bs   []byte
		skip bool
	)
	for _, b := range []byte(s) {
		if b == '\\' {
			skip = true
		}
		if !skip {
			bs = append(bs, b)
		}
		if b == '\n' {
			skip = false
		}
	}
	lx.input = string(bs)
	go lx.run()
	return lx
}

func (lx *lexer) run() {
	defer func() {
		if r := recover(); r != nil {
			switch rr := r.(type) {
			case error:
				lx.errorf("%s", rr.Error())
			case string:
				lx.errorf("%s", rr)
			default:
				lx.errorf("%v", rr)
			}
		}
	}()
	for lx.state != nil {
		lx.state = lx.state(lx)
	}
}

func (lx *lexer) push(state stateFn) {
	lx.stack = append(lx.stack, state)
}

func (lx *lexer) pop() stateFn {
	if len(lx.stack) == 0 {
		return lx.errorf("BUG in lexer: no states to pop")
	}
	last := lx.stack[len(lx.stack)-1]
	lx.stack = lx.stack[0 : len(lx.stack)-1]
	return last
}

func (lx *lexer) current() string {
	return lx.input[lx.start:lx.pos]
}

func (lx *lexer) emit(typ itemType) {
	lx.items <- item{typ, lx.current(), lx.line, lx.currLines[0]}
	lx.start = lx.pos
}

func (lx *lexer) emitTrim(typ itemType) {
	lx.items <- item{typ, strings.TrimSpace(lx.current()), lx.line, lx.currLines[0]}
	lx.start = lx.pos
}

func (lx *lexer) next() (r rune) {
	if lx.atEOF {
		panic("next called after EOF")
	}
	if lx.pos >= len(lx.input) {
		lx.atEOF = true
		return eof
	}

	if lx.input[lx.pos] == '\n' {
		lx.line++
		lx.currLines[2] = lx.currLines[1]
		lx.currLines[1] = lx.currLines[0]
		lx.currLines[0] = 0
	}
	lx.prevWidths[2] = lx.prevWidths[1]
	lx.prevWidths[1] = lx.prevWidths[0]
	if lx.nprev < 3 {
		lx.nprev++
	}
	r, w := utf8.DecodeRuneInString(lx.input[lx.pos:])
	lx.prevWidths[0] = w
	lx.pos += w
	lx.currLines[0] += w
	return r
}

// ignore skips over the pending input before this point.
func (lx *lexer) ignore() {
	lx.start = lx.pos
}

// backup steps back one rune. Can be called only twice between calls to next.
func (lx *lexer) backup() {
	if lx.atEOF {
		lx.atEOF = false
		return
	}
	if lx.nprev < 1 {
		panic("backed up too far")
	}
	w := lx.prevWidths[0]
	lx.prevWidths[0] = lx.prevWidths[1]
	lx.prevWidths[1] = lx.prevWidths[2]
	lx.nprev--
	lx.pos -= w
	if lx.pos < len(lx.input) && lx.input[lx.pos] == '\n' {
		lx.line--
		lx.currLines[0] = lx.currLines[1]
		lx.currLines[1] = lx.currLines[2]
		lx.currLines[2] = 0
	}
}

// accept consumes the next rune if it's equal to `valid`.
func (lx *lexer) accept(valid rune) bool {
	if lx.next() == valid {
		return true
	}
	lx.backup()
	return false
}

// peek returns but does not consume the next rune in the input.
func (lx *lexer) peek() rune {
	r := lx.next()
	lx.backup()
	return r
}

// skip ignores all input that matches the given predicate.
func (lx *lexer) skip(pred func(rune) bool) {
	for {
		r := lx.next()
		if pred(r) {
			continue
		}
		lx.backup()
		lx.ignore()
		return
	}
}

// split all input that matches the given predicate.
func (lx *lexer) forward(stopped func(rune) bool) string {
	defer lx.ignore()
	for {
		r := lx.next()
		if !stopped(r) {
			continue
		}
		lx.backup()
		return lx.current()
	}
}

// errorf stops all lexing by emitting an error and returning `nil`.
// Note that any value that is a character is escaped if it's a special
// character (newlines, tabs, etc.).
func (lx *lexer) errorf(format string, values ...interface{}) stateFn {
	lx.items <- item{
		itemError,
		fmt.Sprintf(format, values...),
		lx.line,
		lx.currLines[0],
	}
	return nil
}

// lexTop consumes elements at the top level of TBO data.
func lexTop(lx *lexer) stateFn {
	lx.skip(func(r rune) bool {
		return isWhitespace(r) || isNL(r)
	})
	r := lx.next()
	lx.ignore()
	switch r {
	case commentStart:
		lx.push(lexTop)
		return lexCommentStart
	case objectStart:
		lx.push(lexTop)
		obj := lx.forward(isWhitespace)
		lx.skip(isWhitespace)
		switch obj {
		case "execl":
			return lexExeclStart
		case "type":
			return lexDefinedTypeStart
		case "print":
			return lexPrintStart
		}
		return lx.errorf("Bad Top Object %s", obj)
	case eof:
		if lx.pos > lx.start {
			return lx.errorf("Unexpected EOF")
		}
		lx.emit(itemEOF)
		return nil
	}

	return lx.errorf("Bad Top %s", string(r))
}

// lexDefinedName
func lexDefinedName(lx *lexer) stateFn {
	lx.skip(isWhitespace)

	if !isUpper(lx.next()) {
		return lx.errorf("Defined Name First Char Upper %s", lx.current())
	}

	var r rune

	for {
		r = lx.next()
		if isLower(r) || isDigit(r) || isUpper(r) || r == underline {
			continue
		}
		break
	}

	lx.backup()
	lx.emitTrim(itemDefinedName)
	return lx.pop()
}

// lexField
func lexField(lx *lexer) stateFn {
	lx.skip(isWhitespace)

	var (
		r rune
		l uint8
	)
	for {
		r = lx.next()
		switch {
		case isLower(r), isUpper(r):
			l++
			continue
		case isDigit(r) || r == underline:
			if l == 0 {
				return lx.errorf("bad bind field name %s", lx.current())
			}
			continue
		}
		break
	}

	lx.backup()
	lx.emitTrim(itemField)
	return lx.pop()
}

// lexPath
func lexPath(lx *lexer) stateFn {
	lx.skip(isWhitespace)

	if lx.next() != doubleQuotes {
		return lx.errorf("bad path %s", lx.current())
	}
	lx.ignore()

	for {
		if lx.next() == doubleQuotes {
			break
		}
	}

	lx.backup()
	lx.emitTrim(itemPath)
	lx.next()
	lx.ignore()

	return lx.pop()
}

// lexExeclStart
func lexExeclStart(lx *lexer) stateFn {
	lx.emit(itemExeclStart)
	lx.push(lexExeclEnd)
	lx.push(lexExeclPathStart)
	return lexDefinedName
}

// lexExeclPathStart
func lexExeclPathStart(lx *lexer) stateFn {
	lx.skip(isWhitespace)

	if lx.next() != execlPathsStart {
		return lx.errorf("bad execl defined %s", lx.current())
	}
	lx.ignore()

	lx.push(lexExeclPathEnd)
	return lexPath
}

// lexExeclPathEnd
func lexExeclPathEnd(lx *lexer) stateFn {
	lx.skip(isWhitespace)

	if lx.next() != execlPathsEnd {
		lx.backup()
		lx.push(lexExeclPathEnd)
		return lexPath
	}
	lx.ignore()

	return lx.pop()
}

// lexExeclEnd
func lexExeclEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemExeclEnd)
	return lx.pop()
}

// lexDefinedTypeStart
func lexDefinedTypeStart(lx *lexer) stateFn {
	lx.emit(itemDefinedTypeStart)
	lx.push(lexDefinedTypeEnd)
	lx.push(lexCheckDefined)
	return lexDefinedName
}

// lexDefinedTypeEnd
func lexDefinedTypeEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemDefinedTypeEnd)
	return lx.pop()
}

// lexCheckDefinedTypeFields
func lexCheckDefined(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	switch lx.next() {
	case bindStart:
		lx.ignore()
		return lexBindStart
	case bodyStart:
		lx.ignore()
		return lexDefinedTypeFieldsStart
	}
	return lx.errorf("Defined Type Error %s", lx.current())
}

// lexDefinedTypeFieldsStart
func lexDefinedTypeFieldsStart(lx *lexer) stateFn {
	lx.emit(itemGroupStart)
	lx.push(lexDefinedTypeFieldsEnd)
	return lexCheckDefinedTypeField
}

// lexCheckDefinedTypeField
func lexCheckDefinedTypeField(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	r := lx.next()
	switch {
	case isNL(r):
		lx.skip(isNL)
		lx.skip(isWhitespace)
		r = lx.next()
		switch r {
		case bodyEnd:
			lx.ignore()
			return lx.pop()
		case commentStart:
			lx.ignore()
			lx.push(lexCheckDefinedTypeField)
			return lexCommentStart
		}
		lx.backup()
		lx.push(lexCheckDefinedTypeField)
		return lexDefinedTypeField
	case r == commentStart:
		lx.ignore()
		lx.push(lexCheckDefinedTypeField)
		return lexCommentStart
	}
	lx.backup()
	return lx.pop()
}

// lexDefinedTypeField
func lexDefinedTypeField(lx *lexer) stateFn {
	lx.push(lexCheckExt)
	lx.push(lexFieldTypeStart)
	lx.push(lexCheckClonn)
	return lexField
}

// lexFieldTypeStart
func lexFieldTypeStart(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	lx.emit(itemGroupStart)
	lx.push(lexFieldTypeEnd)
	lx.push(lexCheckFieldType)
	return lexType
}

// lexCheckFieldType
func lexCheckFieldType(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	switch lx.next() {
	case anySplit:
		lx.ignore()
		lx.push(lexCheckFieldType)
		return lexType
	default:
		lx.backup()
	}
	return lx.pop()
}

// lexFieldTypeEnd
func lexFieldTypeEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemGroupEnd)
	return lx.pop()
}

// lexDefinedTypeFieldsStart
func lexDefinedTypeFieldsEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemGroupEnd)
	return lx.pop()
}

// lexBindStart
func lexBindStart(lx *lexer) stateFn {
	lx.emit(itemBindStart)
	lx.push(lexBindEnd)
	lx.push(lexType)
	lx.push(lexCheckEqual)
	lx.push(lexCheckBindRange)
	lx.push(lexDefinedName)
	lx.push(lexCheckDot)

	return lexDefinedName
}

// lexCheckBindRange
func lexCheckBindRange(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	if lx.next() == bindRangeStart {
		lx.ignore()
		lx.emit(itemGroupStart)
		lx.push(lexCheckBindRange)
		return lexBindRange
	}
	lx.backup()
	return lx.pop()
}

// lexBindRange
func lexBindRange(lx *lexer) stateFn {

	var (
		r   rune
		row bool
		col bool
		st  uint8
	)

BindRange:
	lx.skip(isWhitespace)
	for {
		r = lx.next()

		switch {
		case isUpper(r):
			if row {
				return lx.errorf("Bad Range Col %s", r)
			}
			col = true
			continue
		case isDigit(r):
			if col {
				return lx.errorf("Bad Range Row %s", r)
			}
			row = true
			continue
		case r == dot:
			st = 1
		}
		lx.backup()
		break
	}

	if lx.start != lx.pos {
		switch st {
		case 1:
			lx.emitTrim(itemBindMin)
			st = 2
		case 2:
			lx.emitTrim(itemBindMax)
			st = 0
		default:
			lx.emitTrim(itemBindValue)
		}
	}

	lx.next()
	lx.ignore()

	switch r {
	case bindRangeStart:
		lx.emit(itemGroupStart)
		lx.push(lexBindRange)
		return lexBindRange
	case bindRangeEnd:
		lx.emit(itemGroupEnd)
		return lx.pop()
	case dot, space:
		goto BindRange
	}

	return lx.errorf("Bad Bind range %s", string(r))

}

// lexBindEnd
func lexBindEnd(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	if lx.next() != bindEnd {
		return lx.errorf("Bad Bind End %s", lx.current())
	}
	lx.ignore()
	lx.emit(itemBindEnd)
	return lx.pop()
}

// lexType
func lexType(lx *lexer) stateFn {
	lx.skip(isWhitespace)

	switch lx.next() {
	case indexStart:
		lx.ignore()
		return lexIndexStart
	case anyChildTypeStart:
		lx.ignore()
		return lexAllSetStart
	default:
		lx.backup()
		return lexTypeStart
	}
}

// lexTypeStart
func lexTypeStart(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	var (
		r       rune
		l, u, d uint8
	)

	for {
		r = lx.next()
		switch {
		case isLower(r):
			l++
			continue
		case isUpper(r):
			u++
			continue
		case isDigit(r):
			if l+u == 0 {
				return lx.errorf("bad type %s", lx.current())
			}
			continue
		case r == dot:
			if d > 0 {
				return lx.errorf("bad type %s", lx.current())
			}
			d++
			continue
		case r == underline:
			continue
		}
		break
	}

	lx.backup()
	lx.emitTrim(itemTypeName)

	if r == anyChildTypeStart {
		lx.next()
		lx.ignore()
		return lexTypeGroupStart
	}

	return lx.pop()
}

// lexCheckType
func lexCheckType(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	switch lx.next() {
	case colon:
		lx.ignore()
		lx.emit(itemGroupEnd)
		lx.emit(itemGroupStart)
		lx.push(lexCheckType)
		return lexType
	case anySplit:
		lx.ignore()
		lx.push(lexCheckType)
		return lexType
	case anyChildTypeEnd:
		lx.ignore()
	default:
		lx.backup()
	}
	return lx.pop()
}

// lexTypeGroupEnd
func lexTypeGroupStart(lx *lexer) stateFn {
	lx.emit(itemGroupStart)
	lx.push(lexTypeGroupEnd)
	lx.push(lexCheckType)
	return lexType
}

// lexTypeGroupEnd
func lexTypeGroupEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemGroupEnd)
	return lx.pop()
}

// lexAllSetStart
func lexAllSetStart(lx *lexer) stateFn {
	lx.emit(itemAllSetStart)
	lx.push(lexAllSetEnd)
	return lexAllSet
}

// lexAllSet
func lexAllSet(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	switch lx.next() {
	case comma:
		lx.ignore()
		lx.push(lexAllSet)
	case anyChildTypeEnd:
		lx.ignore()
		return lx.pop()
	default:
		lx.backup()
		lx.push(lexAllSet)
	}
	return lexDefinedName
}

// lexAllSetEnd
func lexAllSetEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemAllSetEnd)
	return lx.pop()
}

// lexIndexStart
func lexIndexStart(lx *lexer) stateFn {
	lx.emit(itemIndexStart)
	lx.push(lexIndexEnd)
	return lexIndexBinds
}

// lexIndexBinds
func lexIndexBinds(lx *lexer) stateFn {
	lx.skip(isWhitespace)

	lx.emit(itemGroupStart)
	for r := lx.next(); r != indexEnd && r != equal; r = lx.next() {
		if r == anySplit {
			lx.backup()
			lx.emitTrim(itemIndexBind)
			lx.next()
			lx.ignore()
			lx.skip(isWhitespace)
		}
	}
	lx.backup()
	lx.emitTrim(itemIndexBind)
	lx.emit(itemGroupEnd)

	if lx.next() == indexEnd {
		lx.ignore()
		return lx.pop()
	}
	lx.ignore()

	return lexIndexTypeStart
}

func lexIndexTypeStart(lx *lexer) stateFn {
	lx.emit(itemGroupStart)
	lx.push(lexIndexTypeEnd)
	lx.push(lexCheckIndexType)
	return lexTypeStart
}

// lexCheckIndexType
func lexCheckIndexType(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	switch lx.next() {
	case anySplit:
		lx.ignore()
		lx.skip(isWhitespace)
		lx.push(lexCheckIndexType)
		return lexTypeStart
	case indexEnd:
		lx.ignore()
	default:
		lx.backup()
	}
	return lx.pop()
}

func lexIndexTypeEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemGroupEnd)
	return lx.pop()
}

// lexIndexEnd
func lexIndexEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemIndexEnd)
	return lx.pop()
}

// lexPrintStart
func lexPrintStart(lx *lexer) stateFn {
	lx.emit(itemPrintStart)
	lx.push(lexPrintEnd)
	lx.push(lexPrinterStart)

	return lexPrintTypeStart
}

// lexPrintTypeStart
func lexPrintTypeStart(lx *lexer) stateFn {
	lx.emit(itemGroupStart)
	lx.push(lexPrintTypeEnd)
	return lexCheckPrintType
}

// lexCheckPrintType
func lexCheckPrintType(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	switch lx.next() {
	case bodyStart:
		return lx.pop()
	case comma:
		lx.ignore()
	default:
		lx.backup()
	}
	lx.push(lexCheckPrintType)
	return lexDefinedName
}

// lexPrintTypeEnd
func lexPrintTypeEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemGroupEnd)
	return lx.pop()
}

// lexPrinterStart
func lexPrinterStart(lx *lexer) stateFn {
	lx.emit(itemGroupStart)
	lx.push(lexPrinterEnd)
	return lexCheckPrinter
}

// lexCheckPrinter
func lexCheckPrinter(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	r := lx.next()
	switch {
	case isNL(r):
		lx.skip(isNL)
		lx.skip(isWhitespace)
		r = lx.next()
		switch r {
		case bodyEnd:
			lx.ignore()
			return lx.pop()
		case commentStart:
			lx.ignore()
			lx.push(lexCheckPrinter)
			return lexCommentStart
		}
		lx.backup()
		lx.push(lexCheckPrinter)
		return lexPrinter
	case r == commentStart:
		lx.ignore()
		lx.push(lexCheckPrinter)
		return lexCommentStart
	}
	lx.backup()
	return lx.pop()
}

// lexPrinter
func lexPrinter(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	for lx.next() != space {
	}
	lx.emitTrim(itemPrinter)

	lx.skip(isWhitespace)
	for lx.next() != space {
	}
	lx.emitTrim(itemPrintOp)

	lx.push(lexCheckExt)

	return lexPath
}

// lexPrinterEnd
func lexPrinterEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemGroupEnd)
	return lx.pop()
}

func lexPrintEnd(lx *lexer) stateFn {
	lx.ignore()
	lx.emitTrim(itemPrintEnd)
	return lx.pop()
}

// lexCheckExt
func lexCheckExt(lx *lexer) stateFn {
	lx.skip(isWhitespace)
	if lx.next() == extStart {
		lx.ignore()
		return lexExt
	}
	lx.backup()
	return lx.pop()
}

// lexExt
func lexExt(lx *lexer) stateFn {
	var r rune
	for {
		r = lx.next()
		if r == extEnd {
			lx.backup()
			lx.emit(itemExt)
			break
		}
	}

	lx.next()
	lx.ignore()
	return lx.pop()
}

// lexCheckClonn
func lexCheckClonn(lx *lexer) stateFn {
	for {
		if lx.next() != colon {
			lx.ignore()
			continue
		}
		break
	}
	return lx.pop()
}

// lexCheckEqual
func lexCheckEqual(lx *lexer) stateFn {
	for {
		if lx.next() != equal {
			lx.ignore()
			continue
		}
		break
	}
	return lx.pop()
}

// lexCheckDot
func lexCheckDot(lx *lexer) stateFn {
	for {
		if lx.next() != dot {
			lx.ignore()
			continue
		}
		break
	}
	return lx.pop()
}

// lexCommentStart begins the lexing of a comment. It will emit
// itemCommentStart and consume no characters, passing control to lexComment.
func lexCommentStart(lx *lexer) stateFn {
	lx.ignore()
	lx.emit(itemCommentStart)
	return lexComment
}

// lexComment lexes an entire comment. It assumes that '#' has been consumed.
// It will consume *up to* the first newline character, and pass control
// back to the last state on the stack.
func lexComment(lx *lexer) stateFn {
	var r rune
	for {
		r = lx.next()
		if isNL(r) || r == eof {
			lx.backup()
			lx.emit(itemText)
			break
		}
	}
	return lx.pop()
}

// lexSkip ignores all slurped input and moves on to the next state.
func lexSkip(lx *lexer, nextState stateFn) stateFn {
	return func(lx *lexer) stateFn {
		lx.ignore()
		return nextState
	}
}

// isWhitespace returns true if `r` is a whitespace character according
// to the spec.
func isWhitespace(r rune) bool {
	return r == '\t' || r == ' '
}

func isNL(r rune) bool {
	return r == '\n' || r == '\r'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isLower(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isBareKeyChar(r rune) bool {
	return isLower(r) || isDigit(r)
}

func isHexadecimal(r rune) bool {
	return (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'f') ||
		(r >= 'A' && r <= 'F')
}
