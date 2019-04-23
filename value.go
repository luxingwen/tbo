package tbo

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

var (
	chunkPool = &sync.Pool{
		New: func() interface{} {
			return &chunk{}
		},
	}
	tablePool = &sync.Pool{
		New: func() interface{} {
			return &table{}
		},
	}
	recordPool = &sync.Pool{
		New: func() interface{} {
			return &record{}
		},
	}
	bufferPool = &sync.Pool{
		New: func() interface{} {
			return &buffer{}
		},
	}
)

func recede(i interface{}) {
	//log.Debugf("recede:%v", i)
	switch t := i.(type) {
	case *chunk:
		if t.snapshot != nil {
			switch s := t.snapshot.(type) {
			case *tableSnapshot:
				recede(s.v)
			case *recordSnapshot:
				recede(s.v)
			case *bufferSnapshot:
				if _, ok := s.v.(*table); ok {
					recede(s.v)
				}
			}
		}
		for _, c := range t.cs {
			recede(c)
		}
		chunkPool.Put(t)
	case *table:
		if t.v != nil && t.v != t {
			recede(t.v)
		}
		tablePool.Put(t)
	case *record:
		if t.v != nil && t.v != t {
			recede(t.v)
		}
		recordPool.Put(t)
	case *buffer:
		if t.v != nil && t.v != t {
			recede(t.v)
		}
		bufferPool.Put(t)
	}
}

func tboChunk(flag chunkFlag, args ...interface{}) (c *chunk) {
	c = chunkPool.Get().(*chunk)

	c.flag = flag
	c.args = args

	c.p, c.cs, c.snapshot, c.e = nil, nil, nil, nil

	return c
}

func tboTable(d *sheet) (g *table) {
	g = tablePool.Get().(*table)

	g.d = d
	g.row, g.b, g.c, g.v, g.child = 0, nil, nil, nil, g

	return g
}

func tboRecord(d *sheet, row int) (r *record) {
	r = recordPool.Get().(*record)

	r.d = d
	r.row = row

	r.col, r.b, r.c, r.v, r.cg, r.child = 0, nil, nil, nil, d.cols, r

	return r
}

func tboBuffer(s string) (b *buffer) {
	b = bufferPool.Get().(*buffer)

	s = strings.TrimSpace(s)

	b.input = s
	b.l = len(s)

	b.start, b.pos, b.width, b.split, b.end, b.s = 0, 0, nil, eof, eof, ""
	b.b, b.c, b.v, b.child = nil, nil, nil, b

	return
}

type chunkFlag uint8

const (
	chunkBase = iota

	chunkEnum
	chunkEnumElement
	chunkEnumName
	chunkEnumValue
	chunkEnumDesc

	chunkArray

	chunkMap
	chunkMapElement
	chunkKey
	chunkValue

	chunkStructIndex
	chunkStruct
	chunkFiled

	chunkSets
	chunkSet
	chunkIndex

	chunkTry

	chunkBegin
	chunkEnd
)

func (cf chunkFlag) String() string {
	switch cf {
	case chunkBase:
		return "chunkBase"
	case chunkEnum:
		return "chunkEnum"
	case chunkEnumElement:
		return "chunkEnumElement"
	case chunkEnumName:
		return "chunkEnumName"
	case chunkEnumValue:
		return "chunkEnumValue"
	case chunkEnumDesc:
		return "chunkEnumDesc"
	case chunkArray:
		return "chunkArray"
	case chunkMap:
		return "chunkMap"
	case chunkMapElement:
		return "chunkMapElement"
	case chunkStruct:
		return "chunkStruct"
	case chunkStructIndex:
		return "chunkStructIndex"
	case chunkKey:
		return "chunkKey"
	case chunkValue:
		return "chunkValue"
	case chunkFiled:
		return "chunkFiled"
	case chunkSets:
		return "chunkSets"
	case chunkSet:
		return "chunkSet"
	case chunkIndex:
		return "chunkIndex"
	case chunkTry:
		return "chunkTry"
	case chunkBegin:
		return "chunkBegin"
	case chunkEnd:
		return "chunkEnd"
	}
	return ""
}

type chunk struct {
	flag chunkFlag
	args []interface{}

	p  *chunk
	cs []*chunk

	snapshot snapshot

	e interface{}
}

func (c *chunk) String() string {
	var as []string
	for _, a := range c.args {
		if s, ok := a.(string); ok && s != "" {
			as = append(as, s)
		}
	}
	if c.snapshot != nil {
		as = append(as, c.snapshot.String())
	}
	as = append(as, fmt.Sprintf("len.%d", len(c.cs)))
	return strings.Join([]string{c.flag.String(), "(", strings.Join(as, ", "), ")"}, "")
}

type chunkError struct {
	c   *chunk
	err string
}

func (ce chunkError) Error() string {
	msgs := []string{ce.err}
	in := "      "
	for cc, format := ce.c, in; cc.p != nil; cc, format = cc.p, strings.Join([]string{in, format}, "") {
		msgs = append(msgs, strings.Join([]string{format, cc.String()}, ""))
	}
	return strings.Join(msgs, "\n")
}

type snapshot interface {
	String() string
	recover()
}

type joinFunc func() tboValue
type tryFunc func()
type catchFunc func(interface{})

type tboValue interface {
	push(flag chunkFlag, args ...interface{}) tboValue
	append(args ...interface{}) tboValue
	pop(link bool) *chunk
	base() *chunk
	current() *chunk
	join(joinFunc) tboValue
	try(tryFunc)
	catch(catchFunc)
	throw(format string, v ...interface{})
	snapshot() snapshot
	next() (tboValue, bool)
	value() string
	String() string
}

type baseSnapshot struct {
	b *baseValue

	c *chunk
	v tboValue
}

func (bs *baseSnapshot) recover() {
	bs.b.c = bs.c
	bs.b.v = bs.v
}

func (bs *baseSnapshot) String() string {
	return ""
}

type baseValue struct {
	b     *chunk
	c     *chunk
	v     tboValue
	child tboValue
}

func (b *baseValue) push(flag chunkFlag, args ...interface{}) tboValue {
	c := tboChunk(flag, args...)
	log.Debugf("push %s", c.String())

	c.snapshot = b.snapshot()

	if b.b == nil {
		b.b = c
	}

	c.p = b.c
	b.c = c

	return b
}

func (b *baseValue) append(args ...interface{}) tboValue {
	if b.c != nil {
		b.c.args = append(b.c.args, args...)
	}

	return b
}

func (b *baseValue) pop(link bool) *chunk {
	if b.c == nil {
		b.throw("pop(true) Chunk Nil")
	}
	log.Debugf("pop(true) %s", b.c.String())
	c := b.c
	b.c = c.p

	if b.c != nil && c.flag != chunkTry && link {
		b.c.cs = append(b.c.cs, c)
	}

	return c
}

func (b *baseValue) base() *chunk {
	return b.b
}

func (b *baseValue) current() *chunk {
	return b.c
}

func (b *baseValue) join(f joinFunc) (v tboValue) {
	switch bb := f().(type) {
	case *table:
		bb.b, bb.c, b.v, v = b.c, b.c, bb, bb
	case *record:
		bb.b, bb.c, b.v, v = b.c, b.c, bb, bb
	case *buffer:
		bb.b, bb.c, b.v, v = b.c, b.c, bb, bb
	default:
		b.throw("Join tboValue %s Fail", b.child.String())
	}

	return
}

func (b *baseValue) try(f tryFunc) {

	b.push(chunkTry)

	c := b.c

	defer func() {
		c.e = recover()
	}()

	f()
}

func (b *baseValue) catch(f catchFunc) {
	c := b.c
	for c.flag != chunkTry {
		c = c.p
	}

	defer recede(c)

	log.Debugf("catch e:%v", c.e)

	if c.e == nil {
		if c.p != nil {
			b.c = c.p
			b.c.cs = append(b.c.cs, c.cs[0])
			c.cs = c.cs[1:]
		} else {
			b.c = c.cs[0]
			c.cs = c.cs[1:]
			b.b = b.c
		}
		return
	}
	c.snapshot.recover()

	f(c.e)
}

func (b *baseValue) throw(format string, v ...interface{}) {
	log.Debugf(format, v...)
	panic(chunkError{c: b.c, err: fmt.Sprintf(format, v...)})
}

func (b *baseValue) snapshot() snapshot {
	if b.child != nil {
		return b.child.snapshot()
	}
	return nil
}

func (b *baseValue) next() (tboValue, bool) {
	return nil, false
}

func (b *baseValue) value() string {
	return ""
}

func (b *baseValue) String() string {
	return "base"
}

type tableSnapshot struct {
	baseSnapshot

	t   *table
	row int
}

func (ts *tableSnapshot) recover() {
	ts.baseSnapshot.recover()
	ts.t.row = ts.row
}

func (ts *tableSnapshot) String() string {
	row, _ := ts.t.d.rows.index(ts.row)
	return fmt.Sprintf("table<%s.%s.%d>", ts.t.d.set, ts.t.d.sheet, row+1)
}

type table struct {
	baseValue
	d *sheet

	row int
}

func (t *table) next() (tboValue, bool) {
	if t.row >= t.d.rows.size() {
		return nil, false
	}
	ri, _ := t.d.rows.index(t.row)
	r := tboRecord(t.d, ri)
	r.b, r.c = t.c, t.c
	if t.v != nil {
		recede(t.v)
	}
	t.v = r
	t.row++
	return t.v, true
}

func (t *table) snapshot() snapshot {
	row := t.row
	if row >= t.d.rows.size() {
		row--
	}
	defer func() {
		t.v = nil
	}()
	return &tableSnapshot{
		baseSnapshot: baseSnapshot{b: &t.baseValue, c: t.c, v: t.v},
		t:            t,
		row:          row,
	}
}

func (t *table) value() string {
	return t.v.value()
}

func (t *table) String() string {
	return fmt.Sprintf("table<%s.%s>", t.d.set, t.d.sheet)
}

type recordSnapshot struct {
	baseSnapshot

	r        *record
	row, col int
}

func (rs *recordSnapshot) recover() {
	rs.baseSnapshot.recover()
	rs.r.row = rs.row
	rs.r.col = rs.col
}

func (rs *recordSnapshot) String() string {
	col, _ := rs.r.d.cols.index(rs.col)
	return fmt.Sprintf("record<%s.%s.%s.%s%d>", rs.r.d.set, rs.r.d.sheet, rs.r.cg, index2Alphabet(col+1), rs.row+1)
}

type record struct {
	baseValue
	d   *sheet
	row int
	cg  *groupValue

	col int
}

func (r *record) next() (tboValue, bool) {
	log.Debugf("%s col:%d r.cg.size:%d", r, r.col, r.cg.size())

	if r.col >= r.cg.size() {
		return nil, false
	}

	if r.c.flag == chunkMap {
		return r, true
	}

	ci, c := r.cg.index(r.col)
	if ci == -1 {
		rr := tboRecord(r.d, r.row)
		rr.cg = c.(*groupValue)
		log.Debugf("record row:%d col:%d v %s ", r.row, r.col, rr.String())
		rr.b, rr.c = r.c, r.c
		if r.v != nil {
			recede(r.v)
		}
		r.v = rr
	} else {
		// 把 [{a; b}, {b, 4}] 转换成 ((a;b),(b;a))
		str := strings.ReplaceAll(r.d.data.Cell(r.row, ci).String(), "[", "(")
		str = strings.ReplaceAll(str, "]", ")")
		str = strings.ReplaceAll(str, "{", "(")
		s := strings.ReplaceAll(str, "}", ")")

		if s != "" {
			b := tboBuffer(s)
			log.Debugf("record row:%d col:%d v %s ", r.row, r.col, b.String())
			b.b, b.c = r.c, r.c
			if r.v != nil {
				recede(r.v)
			}
			r.v = b
		} else {
			r.v = nil
		}
	}
	r.col++
	return r.v, true
}

func (r *record) snapshot() snapshot {
	col := r.col
	if col >= r.d.cols.size() {
		col--
	}
	defer func() {
		r.v = nil
	}()
	return &recordSnapshot{
		baseSnapshot: baseSnapshot{b: &r.baseValue, c: r.c, v: r.v},
		r:            r,
		row:          r.row,
		col:          col,
	}
}

func (r *record) value() string {
	return r.v.value()
}

func (r *record) String() string {
	return fmt.Sprintf("record<%s.%s.%s.%d>", r.d.set, r.d.sheet, r.cg, r.row+1)
}

const (
	structSplit  = ';'
	complexBegin = '('
	complexEnd   = ')'
	bytesBegin   = '\''
	bytesEnd     = '\''
)

type bufferSnapshot struct {
	baseSnapshot

	b     *buffer
	start int
	pos   int
	w     int
	split rune
	end   rune
}

func (bs *bufferSnapshot) recover() {
	bs.baseSnapshot.recover()
	bs.b.start = bs.start
	bs.b.pos = bs.pos
	bs.b.width = bs.b.width[0:bs.w]
	bs.b.split = bs.split
	bs.b.end = bs.end
}

func (bs *bufferSnapshot) String() string {
	return fmt.Sprintf("buffer<'%s' %d:%d '%s' '%s'>", bs.b.input, bs.start, bs.pos, string(bs.split), string(bs.end))
}

type buffer struct {
	baseValue

	input string
	l     int

	start int
	pos   int
	width []int
	split rune
	end   rune
	s     string
}

func (b *buffer) forward() rune {
	// log.Debugf("start forward pos:%d", b.pos)
	// defer log.Debugf("end forward pos:%d", b.pos)
	if b.pos >= b.l {
		b.width = append(b.width, 0)
		return eof
	}
	r, w := utf8.DecodeRuneInString(b.input[b.pos:])
	b.pos += w
	b.width = append(b.width, w)
	return r
}

// backward steps back one rune. Can be called only twice between calls to next.
func (b *buffer) backward() {
	// log.Debugf("start backward pos:%d", b.pos)
	// defer log.Debugf("end backward pos:%d", b.pos)
	l := len(b.width)
	b.pos -= b.width[l-1]
	b.width = b.width[:l-1]
}

// ignore skips over the pending input before this point.
func (b *buffer) ignore() {
	// log.Debugf("start ignore pos:%d", b.pos)
	// defer log.Debugf("end ignore pos:%d", b.pos)
	b.start = b.pos
	b.width = b.width[0:0]
}

func (b *buffer) skip(r rune) {
	log.Debugf("skip r '%s'", string(r))
	for {
		if b.forward() != r {
			b.backward()
			b.ignore()
			return
		}
	}
}

func (b *buffer) to(r rune) (s string) {
	log.Debugf("to r '%s'", string(r))
	for {
		switch b.forward() {
		case r:
			s = b.input[b.start : b.pos-1]
			b.ignore()
			return
		case eof:
			s = b.input[b.start:b.pos]
			b.ignore()
			return
		}
	}
}

func (b *buffer) prefetch() string {
	pos := b.pos
	s := b.s
	log.Debugf("prefetch")
	v := b.fetch()
	for b.pos > pos {
		b.backward()
	}
	b.s = s
	return v
}

func (b *buffer) fetch() string {
	for {
		switch b.forward() {
		case b.split, b.end:
			b.backward()
			b.s = b.input[b.start:b.pos]
			log.Debugf("fetch v:%s split:'%s' end:'%s' s:%s", b, string(b.split), string(b.end), b.s)
			return b.s
		}
	}
}

func (b *buffer) push(flag chunkFlag, args ...interface{}) tboValue {
	b.baseValue.push(flag, args...)

	switch flag {
	case chunkArray, chunkMap, chunkSets:
		b.split = comma
		b.end = complexEnd
		b.to(complexBegin)
	case chunkKey:
		b.split = colon
	case chunkBase:
		switch toString(args[0]) {
		case "string", "bytes":
			if b.forward() == '\'' {
				b.ignore()
				b.end = '\''
				b.split = b.end
			} else {
				b.backward()
			}
		}
	case chunkStruct:
		switch b.forward() {
		case '!':
			b.ignore()
			if b.to(complexBegin) != b.c.args[0] {
				b.throw("Bad %s Match %s", b.c.flag.String(), toString(b.c.args[0]))
			}
		case complexBegin:
			b.ignore()
		default:
			b.throw("Bad %s Match %s Bad Begin %s", b.c.flag.String(), toString(b.c.args[0]), b.input[b.start:b.pos])
		}
		b.end = complexEnd
	case chunkFiled:
		b.split = structSplit
	}

	return b
}

func (b *buffer) pop(link bool) *chunk {
	c := b.baseValue.pop(link)

	if b.c != nil {

		log.Debugf("pop(%t) buff flag:%s", link, b.c.flag)

		switch b.c.flag {
		case chunkArray, chunkSets, chunkMap, chunkKey, chunkStruct:
			b.skip(space)
			r := b.forward()
			log.Debugf("pop(true) r '%s'", string(r))
			switch r {
			case b.end, b.split:
				b.ignore()
			default:
				b.throw("Bad %s Bad End %s", b.c.flag.String(), b.input[b.start:b.pos])
			}
		}
	}

	if c != nil {
		b.split = c.snapshot.(*bufferSnapshot).split
		b.end = c.snapshot.(*bufferSnapshot).end
	}

	return c
}

func (b *buffer) snapshot() snapshot {
	return &bufferSnapshot{
		baseSnapshot: baseSnapshot{b: &b.baseValue, c: b.c, v: b.v},
		b:            b,
		start:        b.start,
		pos:          b.pos,
		w:            len(b.width),
		split:        b.split,
		end:          b.end,
	}
}

func (b *buffer) next() (tboValue, bool) {
	if b.pos >= b.l {
		return nil, false
	}
	log.Debugf("buffer %s next pos:%d split:%s", b.input, b.pos, string(b.split))
	b.skip(space)
	b.v = b
	switch b.c.flag {
	case chunkFiled:
		if b.prefetch() == "" {
			return nil, true
		}
	case chunkBase, chunkIndex:
		b.fetch()
		if b.end == '\'' {
			b.forward()
		}
		if b.s == "" {
			return nil, false
		}
	case chunkSet:
		switch b.forward() {
		case '!':
			b.ignore()
			if b.fetch() != b.c.args[0] {
				b.throw("Bad Set %s No Match %s", b.s, toString(b.c.args[0]))
			}
			d, ok := b.c.args[1].(*sheet)
			if !ok {
				b.throw("Bad Set Sheet Arg")
			}
			t := tboTable(d)
			t.b, t.c = b.c, b.c
			if b.v != b {
				recede(b.v)
			}
			b.v = t
		default:
			b.throw("Bad Set Match %s Bad Value %s", toString(b.c.args[0]), b.input[b.start:b.pos])
		}
	default:
	}

	return b.v, true
}

func (b *buffer) value() string {
	return b.s
}

func (b *buffer) String() string {
	return fmt.Sprintf("buffer<%s>", b.input)
}

func (i *indexType) Value(v tboValue) {
	v.push(chunkIndex)
	defer v.pop(true)

	var s string
	if vv, ok := v.next(); ok {
		s = vv.value()
	}
	if s == "" {
		v.throw("Empty Index")
	}

	for _, kct := range i.kt {
		switch string(kct) {
		case "int":
			if _, err := strconv.ParseInt(s, 10, 64); err == nil {
				v.append("int", s)
				goto IndexValue
			}
		case "uint":
			if _, err := strconv.ParseUint(s, 10, 64); err == nil {
				v.append("uint", s)
				goto IndexValue
			}
		case "bytes":
			if s[0] == '\'' && s[len(s)-1] == '\'' {
				v.append("bytes", s)
				goto IndexValue
			}
		case "string":
			v.append("string", s)
			goto IndexValue
		}
	}
	v.throw("Bad Index Type Value %s", s)

IndexValue:
	for _, b := range i.binds {
		log.Debugf("bind %s %v", b.t.name, b.t.i)
		if row, ok := b.t.i[s]; ok {
			v.append(b.t.name)
			vv := v.join(func() tboValue {
				return b.f(row)
			})
			b.t.vt.Value(vv)
			return
		}
	}
	v.throw("Not Index Value %s", s)
}

func (st *setType) Value(v tboValue) {
	v.push(chunkSet, st.name, st.d)
	vv, ok := v.next()
	if !ok {
		v.throw("Bad Set %s", st.name)
	}
	st.t.Value(vv)
	v.pop(true)
}

func (all allset) Value(v tboValue) {
	v.push(chunkSets)
	for vv, ok := v.next(); ok; vv, ok = v.next() {
		var (
			ee interface{}
			ac int
			ss snapshot
		)
		for _, set := range all {
			ss = vv.snapshot()
			ee =
				func() (er interface{}) {
					defer func() {
						er = recover()
					}()
					set.Value(vv)
					return
				}()
			if ee != nil {
				log.Debugf("Try Set Error %s", ee)
				ss.recover()
				ac++
			}
		}
		log.Debugf("ac:%d", ac)
		if ac == len(all) {
			panic(ee)
		}
	}
	v.pop(true)
}

func (any anyType) Value(v tboValue) {
	l := len(any)
	t := any[0]
	v.try(
		func() {
			t.Value(v)
		})
	v.catch(
		func(e interface{}) {
			if l == 1 {
				panic(e)
			}
			anyType(any[1:]).Value(v)
		})
}

func (spe simpleType) Value(v tboValue) {
	l := len(spe)
	t := spe[0]
	v.try(
		func() {
			t.Value(v)
		})
	v.catch(
		func(e interface{}) {
			if l == 1 {
				panic(e)
			}
			simpleType(spe[1:]).Value(v)
		})
}

func (bt baseType) Value(v tboValue) {
	t := string(bt)
	v.push(chunkBase, t)
	var s string
	if vv, ok := v.next(); ok {
		s = vv.value()
	}
	switch t {
	case "int":
		if _, err := strconv.ParseInt(s, 10, 64); err != nil {
			v.throw("Bad Int %s", s)
		}
	case "uint":
		if _, err := strconv.ParseUint(s, 10, 64); err != nil {
			v.throw("Bad UInt %s", s)
		}
	case "float":
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			v.throw("Bad Float %s", s)
		}
	case "bool":
		switch s {
		case "TRUE", "true", "1":
			s = "true"
		case "FALSE", "false", "0":
			s = "false"
		default:
			v.throw("Bad Bool %s", s)
		}
	case "string", "bytes":
		s = strings.TrimSpace(s)
	}
	v.append(s)
	v.pop(true)
}

func (et enumType) Value(v tboValue) {

	v.push(chunkEnum)

	for vv, ok := v.next(); ok; vv, ok = v.next() {
		vv.push(chunkEnumElement)

		vv1, ok := vv.next()
		if !ok || vv1 == nil {
			vv.pop(false)
			continue
		}
		vv1.push(chunkEnumName)
		tboString_.Value(vv1)
		vv1.pop(true)

		vv2, ok := vv.next()
		if !ok || vv2 == nil {
			vv.pop(false)
			continue
		}
		vv2.push(chunkEnumValue)
		baseType(et).Value(vv2)
		vv2.pop(true)

		vv3, ok := vv.next()
		if !ok || vv3 == nil {
			vv.push(chunkEnumDesc)
			vv.push(chunkBase, "string", "")
			vv.pop(true)
			vv.pop(true)
		} else {
			vv3.push(chunkEnumDesc)
			tboString_.Value(vv3)
			vv3.pop(true)
		}

		vv.pop(true)
	}

	v.pop(true)
}

func (at *arrayType) Value(v tboValue) {
	v.push(chunkArray)
	for vv, ok := v.next(); ok; vv, ok = v.next() {
		if vv == nil {
			continue
		}
		at.element.Value(vv)
	}
	v.pop(true)
}

func (mt *mapType) Value(v tboValue) {
	v.push(chunkMap)
	for vv, ok := v.next(); ok; vv, ok = v.next() {
		if vv == nil {
			continue
		}
		vv.push(chunkMapElement)

		vv1, ok := vv.next()
		if !ok || vv1 == nil {
			vv.pop(false)
			continue
		}

		vv1.push(chunkKey)
		mt.key.Value(vv1)
		vv1.pop(true)

		vv2, ok := vv.next()
		if !ok || vv2 == nil {
			vv.pop(false)
			continue
		}

		vv2.push(chunkValue)
		mt.value.Value(vv2)
		vv2.pop(true)

		vv.pop(true)
	}
	v.pop(true)
}

func (st *structType) Value(v tboValue) {
	v.push(chunkStruct, st.name)
	for _, f := range st.fileds {
		v.push(chunkFiled, f.name, f.ext)
		log.Debugf("start si %s => filed %s ", st.String(), f.name)
		vv, ok := v.next()
		if !ok {
			v.throw("Bad Struct %s Field %s", st.String(), f.name)
		}
		if vv == nil {
			v.append("d")
			if any, ok := f.t.(anyType); ok {
				var (
					sk bool
					kk = &sk
					ee *interface{}
				)
				for _, t := range any {
					if t.Level() == indexLevel {
						continue
					}
					v.try(func() {
						log.Debugf("Struct %s.%s try default:%s", st.String(), f.name, t.Default())
						vv = v.join(func() tboValue {
							return tboBuffer(t.Default())
						})
						t.Value(vv)
						*kk = true
					})
					v.catch(func(e interface{}) {
						ee = &e
					})
					if sk {
						break
					}
				}
				if !sk {
					panic(*ee)
				}
			} else {
				log.Debugf("Struct %s.%s try default:%s", st.String(), f.name, f.t.Default())
				vv = v.join(func() tboValue {
					return tboBuffer(f.t.Default())
				})
				f.t.Value(vv)
			}
		} else {
			f.t.Value(vv)
		}
		v.pop(true)
	}
	v.pop(true)
}

func (si *structIndex) Value(v tboValue) {
	v.push(chunkStructIndex, si.t.name, si.index.name)
	si.t.Value(v)
	c := v.current()
	for _, cc := range c.cs[0].cs {
		if toString(cc.args[0]) == si.index.name {
			cc.args = append(cc.args, "i")
			v.append(cc)
			v.pop(true)
			return
		}
	}
	v.throw("StructIndex %s Not Index", si.String())
}
