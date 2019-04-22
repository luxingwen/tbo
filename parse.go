package tbo

import (
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
)

type parser struct {
	lx *lexer

	execls map[string]*tboExecl
	types  map[string]tboType
	prints []*tboPrint
	// rough approximation of line number
	approxLine int
	approxPos  int
}

type parseError string

func (pe parseError) Error() string {
	return string(pe)
}

func parse(data string) (p *parser, err error) {
	defer func() {
		if r := recover(); r != nil {
			if er, ok := r.(parseError); ok {
				err = errors.New(er.Error())
			} else {
				err = errors.New(fmt.Sprintf("%v", r))
			}
		}
	}()

	p = &parser{
		execls: make(map[string]*tboExecl),
		types:  make(map[string]tboType),

		lx: lex(data),
	}
	for {
		item := p.next()
		if item.typ == itemEOF {
			break
		}
		p.topLevel(item)
	}

	return
}

func (p *parser) panicf(format string, v ...interface{}) {
	panic(parseError(fmt.Sprintf(format, v...)))
}

func (p *parser) next() item {
	it := p.lx.nextItem()
	p.approxLine = it.line
	p.approxPos = it.pos
	log.Debugf("Type: %s Value: [ %s ]", it.typ.String(), it.val)
	if it.typ == itemError {
		p.panicf("Line %d:%d Lexer Error:\n\t\t%s", it.line, it.pos, it.val)
	}
	return it
}

func (p *parser) bug(format string, v ...interface{}) {
	p.panicf("Near Line %d:%d Has Bug:\n\t\t%s", p.approxLine, p.approxPos, fmt.Sprintf(format, v...))
}

func (p *parser) expect(typ itemType) item {
	it := p.next()
	p.assertEqual(typ, it.typ)
	return it
}

func (p *parser) assertEqual(expected, got itemType) {
	if expected != got {
		p.bug("Expected '%s' but got '%s'.", expected, got)
	}
}

func (p *parser) topLevel(it item) {
	switch it.typ {
	case itemCommentStart:
		p.expect(itemText)
	case itemExeclStart:
		name := p.expect(itemDefinedName).val

		if _, ok := p.execls[name]; ok {
			p.bug("Already defined The Execl Set %s", name)
		}

		var paths []string
		for it = p.next(); it.typ != itemExeclEnd; it = p.next() {
			paths = append(paths, it.val)
		}

		p.execls[name] = &tboExecl{
			name:  name,
			paths: paths,
		}

	case itemDefinedTypeStart:
		name := p.expect(itemDefinedName).val

		if _, ok := p.types[name]; ok {
			p.bug("Already defined Type %s", name)
		}

		if p.next().typ == itemBindStart {
			p.types[name] = p.setType(name)
		} else {
			p.types[name] = p.structType(name)
		}

		p.expect(itemDefinedTypeEnd)

	case itemPrintStart:
		pr := new(tboPrint)

		p.expect(itemGroupStart)
		for it = p.next(); it.typ != itemGroupEnd; it = p.next() {
			if it.typ != itemDefinedName {
				p.bug("Unexpected Print Type %s(%s)", it.typ.String(), it.val)
			}
			pr.ts = append(pr.ts, p.tboDefineName(it.val))
		}

		p.expect(itemGroupStart)
		it = p.next()
		for it.typ != itemGroupEnd {
			if it.typ == itemCommentStart {
				p.expect(itemText)
				it = p.next()
				continue
			}

			printer := &printer{
				typ: it.val,
			}
			printer.op = p.expect(itemPrintOp).val
			printer.path = p.expect(itemPath).val

			it = p.next()

			if it.typ == itemExt {
				printer.ext = tboExt(fmt.Sprintf("%s:%s", printer.typ, it.val))
				it = p.next()
			}

			pr.ps = append(pr.ps, printer)
		}
		p.expect(itemPrintEnd)

		p.prints = append(p.prints, pr)
	default:
		p.bug("Unexpected Type %s at top level", it.typ.String())
	}
}

func (p *parser) setType(name string) tboType {
	set := p.expect(itemDefinedName).val
	sheet := p.expect(itemDefinedName).val
	d := p.tboSheet(set, sheet)
	d.cols = p.bindCols(p.next())
	d.rows = p.bindRows(p.next())

	it := p.next()
	var t tboType
	switch it.val {
	case "enum":
		p.expect(itemGroupStart)
		t = p.tboEnum(p.tboBaseType(p.expect(itemTypeName).val))
		p.expect(itemGroupEnd)
	case "array":
		p.expect(itemGroupStart)
		ct := p.tboDefineName(p.expect(itemTypeName).val)
		t = p.tboArray(ct)
		p.expect(itemGroupEnd)
	case "map":
		t = p.tboType(it, "map")
	default:
		p.bug("Unexpected Bind Type %s", it.val)
	}

	p.expect(itemBindEnd)

	return p.tboSet(name, t, d)

}

// Gets a string for a key (or part of a key in a table name).
func (p *parser) bindRows(it item) *groupValue {
	if it.typ != itemGroupStart {
		p.bug("Unexpected Bind Row %s(%s)", it.typ.String(), it.val)
	}
	var (
		rs []sheetValue
		tv *rangeValue
	)
	for it = p.next(); it.typ != itemGroupEnd; it = p.next() {
		switch it.typ {
		case itemBindMin:
			tv = &rangeValue{min: int(alphabet2Index(it.val)), max: -1, i: true}
		case itemBindMax:
			tv.max = int(alphabet2Index(it.val))
			rs = append(rs, tv)
			tv = nil
		case itemBindValue:
			rs = append(rs, &rangeValue{min: int(alphabet2Index(it.val)), max: int(alphabet2Index(it.val)), i: true})
		default:
			p.bug("Unexpected sheet Row: %s", it.typ)
		}
	}
	if tv != nil {
		rs = append(rs, tv)
	}
	return p.tboRows(rs)
}

func (p *parser) bindCols(it item) *groupValue {
	return p.tboCols(p.bindGroup(it).rs)
}

// Gets a string for a key (or part of a key in a table name).
func (p *parser) bindGroup(it item) *groupValue {
	if it.typ != itemGroupStart {
		p.bug("Unexpected Bind Col %s(%s)", it.typ.String(), it.val)
	}
	var (
		rs []sheetValue
		tv *rangeValue
	)
	for it = p.next(); it.typ != itemGroupEnd; it = p.next() {
		switch it.typ {
		case itemBindMin:
			tv = &rangeValue{min: int(alphabet2Index(it.val)), max: -1}
		case itemBindMax:
			tv.max = int(alphabet2Index(it.val))
			rs = append(rs, tv)
			tv = nil
		case itemBindValue:
			rs = append(rs, &rangeValue{min: int(alphabet2Index(it.val)), max: int(alphabet2Index(it.val))})
		case itemGroupStart:
			rs = append(rs, p.bindGroup(it))
		default:
			p.bug("Unexpected sheet Col: %s", it.typ)
		}
	}
	if tv != nil {
		rs = append(rs, tv)
	}
	return p.tboGroupValue(rs)
}

//
func (p *parser) structType(name string) tboType {
	it := p.next()

	var fileds []structFiled
	for it.typ != itemGroupEnd {

		if it.typ == itemCommentStart {
			p.expect(itemText)
			it = p.next()
			continue
		}

		if it.typ != itemField {
			p.bug("Unexpected struct %s field Type %s", name, it.typ)
		}

		f := structFiled{}

		f.name = it.val

		p.expect(itemGroupStart)

		var ts []tboType
		for it = p.next(); it.typ != itemGroupEnd; it = p.next() {
			log.Debugf("field type %s", it.typ.String())
			ts = append(ts, p.tboType(it, "struct"))
		}

		switch len(ts) {
		case 0:
			p.bug("Unexpected struct %s field: %s Type Error", name, f.name)
		case 1:
			f.t = ts[0]
		default:
			f.t = p.tboAny(ts)
		}

		it = p.next()

		if it.typ == itemExt {
			f.ext = tboExt(it.val)
			it = p.next()
		}

		if it.typ == itemCommentStart {
			f.desc = p.expect(itemText).val
			it = p.next()
		}

		fileds = append(fileds, f)
	}

	return p.tboStruct(name, fileds)
}

//
func (p *parser) indexType(it item) tboType {
	if it.typ != itemGroupStart {
		p.bug("Bad Index Type %s", it.typ.String())
	}
	var binds []indexBind
	for it = p.next(); it.typ != itemGroupEnd; it = p.next() {
		if it.typ != itemIndexBind {
			p.bug("Unexpected struct field Index Type %s(%s)", it.typ.String(), it.val)
		}
		b := indexBind{
			b: it.val,
		}
		binds = append(binds, b)
	}

	it = p.next()

	if it.typ == itemIndexEnd {
		return p.tboIndex(nil, binds)
	}

	p.assertEqual(itemGroupStart, it.typ)

	var bs []baseType
	for it = p.next(); it.typ != itemGroupEnd; it = p.next() {
		switch it.val {
		case "int", "uint", "string", "bytes":
			bs = append(bs, p.tboBaseType(it.val))
		default:
			p.bug("Unexpected index match Type %s Error", it.val)
		}
	}
	if len(bs) == 0 {
		p.bug("Unexpected index match Type Error")
	}

	p.expect(itemIndexEnd)

	return p.tboIndex(p.tboSimple(bs), binds)
}

func (p *parser) allset(it item) tboType {
	var all []defineTypeName
	for it.typ != itemAllSetEnd {
		dn, ok := p.tboDefineName(it.val).(defineTypeName)
		if !ok {
			p.bug("Bad all Set Name %s ", it.val)
		}
		all = append(all, dn)
		it = p.next()
	}
	return p.tboAllNameSet(all)
}

//
func (p *parser) tboType(it item, parent string) tboType {
	switch it.typ {
	case itemIndexStart:
		return p.indexType(p.next())
	case itemAllSetStart:
		return p.allset(p.next())
	case itemTypeName:
	default:
		p.bug("Bad Type %s(%s)", it.typ.String(), it.val)
	}

	switch it.val {
	case "int", "uint", "float", "string", "bytes", "bool":
		return p.tboBaseType(it.val)
	case "simple":
		return p.tboDefaultSimple()
	case "array":
		p.expect(itemGroupStart)
		var element []tboType
		for ct := p.next(); ct.typ != itemGroupEnd; ct = p.next() {
			element = append(element, p.tboType(ct, "array"))
		}
		switch len(element) {
		case 0:
			p.bug("The Array Element Type Is Empty")
		case 1:
			return p.tboArray(element[0])
		}
		return p.tboArray(p.tboAny(element))
	case "map":
		var simple, any tboType
		p.expect(itemGroupStart)
		var key []baseType
		for ct := p.next(); ct.typ != itemGroupEnd; ct = p.next() {
			key = append(key, p.tboBaseType(ct.val))
		}
		switch len(key) {
		case 0:
			p.bug("The Map Key Type Is Empty")
		case 1:
			simple = key[0]
		default:
			simple = p.tboSimple(key)
		}
		p.expect(itemGroupStart)
		var val []tboType
		for ct := p.next(); ct.typ != itemGroupEnd; ct = p.next() {
			val = append(val, p.tboType(ct, "map"))
		}
		switch len(val) {
		case 0:
			p.bug("The Map Value Type Is Empty")
		case 1:
			any = val[0]
		default:
			any = p.tboAny(val)
		}
		return p.tboMap(simple, any)
	default:
		t := p.tboDefineName(it.val)
		if _, ok := t.(structIndexName); ok && parent != "array" {
			p.bug("StructIndexName Only Can is Array element")
		}
		return t
	}
}
