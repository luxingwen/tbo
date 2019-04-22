package tbo

import (
	"sort"
	"strings"
)

const (
	indexLevel uint8 = 1

	intLevel   uint8 = 2
	uintLevel  uint8 = 2
	floatLevel uint8 = 2

	boolLevel uint8 = 4

	stringLevel uint8 = 64
	bytesLevel  uint8 = 64

	arrayLevel       uint8 = 16
	mapLevel         uint8 = 16
	structLevel      uint8 = 16
	structIndexLevel uint8 = 16

	setLevel    uint8 = 32
	allSetLevel uint8 = 32
)

type tboType interface {
	String() string
	Default() string
	Level() uint8
	Value(tboValue)
}

type anyType []tboType

func (at anyType) Len() int      { return len(at) }
func (at anyType) Swap(i, j int) { at[i], at[j] = at[j], at[i] }
func (at anyType) Less(i, j int) bool {
	return at[i].Level() < at[j].Level()
}

func (at anyType) String() string {
	var anys []string
	for _, t := range at {
		anys = append(anys, t.String())
	}
	return strings.Join(anys, "|")
}

func (at anyType) Default() string {
	return at[0].Default()
}

func (at anyType) Level() uint8 {
	var l uint8
	for _, t := range at {
		l = l | t.Level()
	}
	return l
}

type baseType string

func (bt baseType) String() string {
	return string(bt)
}

func (bt baseType) Default() string {
	switch string(bt) {
	case "int":
		return "0"
	case "uint":
		return "0"
	case "float":
		return "0"
	case "bool":
		return "false"
	case "string", "bytes":
		return "''"
	}
	return ""
}

func (bt baseType) Level() uint8 {
	switch string(bt) {
	case "int":
		return intLevel
	case "uint":
		return uintLevel
	case "float":
		return floatLevel
	case "bool":
		return boolLevel
	case "string":
		return stringLevel
	case "bytes":
		return bytesLevel
	}
	return 0
}

type enumType baseType

func (et enumType) String() string {
	return strings.Join([]string{"enum(", baseType(et).String(), ")"}, "")
}

func (et enumType) Default() string {
	return baseType(et).Default()
}

func (et enumType) Level() uint8 {
	return baseType(et).Level()
}

type simpleType []baseType

func (st simpleType) Len() int      { return len(st) }
func (st simpleType) Swap(i, j int) { st[i], st[j] = st[j], st[i] }
func (st simpleType) Less(i, j int) bool {
	return st[i].Level() < st[j].Level()
}

func (st simpleType) String() string {
	var simples []string
	for _, t := range st {
		simples = append(simples, t.String())
	}
	return strings.Join(simples, "|")
}

func (st simpleType) Default() string {
	return st[0].Default()
}

func (st simpleType) Level() uint8 {
	var l uint8
	for _, t := range st {
		l = l | t.Level()
	}
	return l
}

func (st simpleType) merge(t tboType) (nst simpleType) {
	nst = append(nst, st...)
	switch tt := t.(type) {
	case simpleType:
		nst = append(nst, tt...)
	case baseType:
		nst = append(nst, tt)
	default:
		throw("Merge Simple %s Not Simple", tt.String())
	}
	for i := 0; i < len(nst); i++ {
		for j := i + 1; j < len(nst); {
			if nst[i].String() == nst[j].String() {
				nst = append(nst[:j], nst[j+1:]...)
			} else {
				j++
			}
		}
	}
	sort.Sort(nst)
	return
}

type arrayType struct {
	element tboType
}

func (at arrayType) String() string {
	return strings.Join([]string{"array(", at.element.String(), ")"}, "")
}

func (at arrayType) Default() string {
	return ""
}

func (at arrayType) Level() uint8 {
	return arrayLevel
}

type mapType struct {
	key   tboType
	value tboType
}

func (mt mapType) String() string {
	return strings.Join([]string{"map(", mt.key.String(), ":", mt.value.String(), ")"}, "")
}

func (mt mapType) Default() string {
	return ""
}

func (mt mapType) Level() uint8 {
	return mapLevel
}

type structType struct {
	name   string
	fileds []structFiled
}

type structFiled struct {
	name string
	t    tboType
	ext  *ext
	desc string
}

func (st structType) String() string {
	return st.name
}

func (st structType) Default() string {
	var fvs []string
	for _, f := range st.fileds {
		fvs = append(fvs, f.t.Default())
	}
	return strings.Join([]string{"(", strings.Join(fvs, ";"), ")"}, "")
}

func (st structType) Level() uint8 {
	return structLevel
}

type structIndex struct {
	index *structFiled
	i     int
	t     *structType
}

func (si structIndex) String() string {
	return strings.Join([]string{si.t.name, si.index.name}, ".")
}

func (si structIndex) Default() string {
	return si.t.Default()
}

func (st structIndex) Level() uint8 {
	return structIndexLevel
}

type allset []*setType

func (as allset) String() string {
	var ss []string
	for _, t := range as {
		ss = append(ss, t.String())
	}
	return strings.Join([]string{"(", strings.Join(ss, ","), ")"}, "")
}

func (as allset) Default() string {
	var ss []string
	for _, t := range as {
		ss = append(ss, strings.Join([]string{"!", t.name}, ""))
	}
	return strings.Join([]string{"(", strings.Join(ss, ","), ")"}, "")
}

func (as allset) Level() uint8 {
	return allSetLevel
}

type setType struct {
	name string

	t tboType // 绑定类型

	d *sheet
	k int // 索引的列

	kt tboType // 索引类型
	vt tboType // 值类型

	i map[string]int
}

func (b setType) String() string {
	return b.name
}

func (b setType) Default() string {
	return ""
}

func (b setType) Level() uint8 {
	return setLevel
}

type indexType struct {
	kt    simpleType // 索引的类型
	binds []indexBind
}

type indexBind struct {
	b string
	t *setType
	f func(int) tboValue
}

func (it indexType) String() string {
	var bs []string
	for _, b := range it.binds {
		bs = append(bs, b.b)
	}
	return strings.Join([]string{"<", strings.Join(bs, "|"), " = ", it.kt.String(), ">"}, "")
}

func (it indexType) Default() string {
	return ""
}

func (it indexType) Level() uint8 {
	return indexLevel
}

type defineTypeName string

func (dtn defineTypeName) String() string {
	return strings.Join([]string{"!", string(dtn)}, "")
}

func (dtn defineTypeName) Default() string {
	return ""
}

func (dtn defineTypeName) Level() uint8 {
	return 0
}

func (dtn defineTypeName) Value(v tboValue) {
	throw("DefineTypeName %s is Bad Print", dtn.String())
}

type structIndexName struct {
	index string
	t     defineTypeName
}

func (sin structIndexName) String() string {
	return strings.Join([]string{"!", string(sin.t), ".", sin.index}, "")
}

func (sin structIndexName) Default() string {
	return ""
}

func (sin structIndexName) Level() uint8 {
	return 0
}

func (sin structIndexName) Value(v tboValue) {
	throw("StructIndexName %s is Bad Print", sin.String())
}

type allNameSet []defineTypeName

func (ans allNameSet) String() string {
	var ss []string
	for _, n := range ans {
		ss = append(ss, n.String())
	}
	return strings.Join([]string{"(", strings.Join(ss, ","), ")"}, "")
}

func (ans allNameSet) Default() string {
	return ""
}

func (ans allNameSet) Level() uint8 {
	return allSetLevel
}

func (ans allNameSet) Value(v tboValue) {
	throw("AllNameSet %s is Bad Print", ans.String())
}

var (
	tboInt_    baseType   = "int"
	tboUInt_   baseType   = "uint"
	tboFloat_  baseType   = "float"
	tboString_ baseType   = "string"
	tboBytes_  baseType   = "bytes"
	tboBool_   baseType   = "bool"
	tboSimple_ []baseType = []baseType{tboInt_, tboUInt_, tboString_, tboBytes_}
)

func (p parser) tboAny(any []tboType) (at anyType) {
	if len(any) < 1 {
		p.bug("Any Is Empty")
	}
	at = anyType(any)
	sort.Sort(at)
	return
}

func (p parser) tboBaseType(b string) (bt baseType) {
	switch b {
	case "int":
		bt = tboInt_
	case "uint":
		bt = tboUInt_
	case "float":
		bt = tboFloat_
	case "string":
		bt = tboString_
	case "bytes":
		bt = tboBytes_
	case "bool":
		bt = tboBool_
	default:
		p.bug("BaseType only is int uint float string bytes bool")
	}
	return
}

func (p parser) tboEnum(element baseType) enumType {
	if !isSimpleByBase(element) {
		p.bug("enum only is int uint string bytes")
	}
	return enumType(element)
}

func (p parser) tboDefaultSimple() simpleType {
	return simpleType(tboSimple_)
}

func (p parser) tboSimple(anyBase []baseType) (st simpleType) {
	if len(anyBase) < 1 {
		p.bug("Simple Is Empty")
	}
	for _, b := range anyBase {
		if !isSimpleByBase(b) {
			p.bug("simpleType only is int uint string bytes")
		}
	}
	st = simpleType(anyBase)
	sort.Sort(st)
	return
}

func (p parser) tboArray(element tboType) *arrayType {
	return &arrayType{
		element: element,
	}
}

func (p parser) tboMap(key tboType, value tboType) *mapType {
	return &mapType{
		key:   key,
		value: value,
	}
}

func (p parser) tboStruct(name string, fileds []structFiled) *structType {
	return &structType{
		name:   name,
		fileds: fileds,
	}
}

func (p parser) tboStructIndex(index string, st *structType) *structIndex {
	for i, f := range st.fileds {
		if f.name == index {
			if !isSimple(f.t) {
				p.bug("Struct %s Child Type %s Index Not Simple Base", st.name, f.name)
			}
			return &structIndex{
				index: &f,
				i:     i,
				t:     st,
			}
		}
	}
	p.bug("NoFound %s index filed %s", st.name, index)
	return nil
}

func (p parser) tboSet(name string, t tboType, d *sheet) *setType {
	return &setType{
		name: name,
		t:    t,
		d:    d,
	}
}

func (p parser) tboAllSet(all []*setType) allset {
	if len(all) < 1 {
		p.bug("AllSet Is Empty")
	}
	return allset(all)
}

func (p parser) tboIndex(kt simpleType, binds []indexBind) (i *indexType) {
	i = &indexType{
		kt:    kt,
		binds: binds,
	}
	return
}

func (p parser) tboDefineName(s string) tboType {
	dn := strings.Split(s, ".")
	if len(dn) == 2 {
		return structIndexName{
			index: dn[1],
			t:     defineTypeName(dn[0]),
		}
	}
	return defineTypeName(s)
}

func (p parser) tboAllNameSet(all []defineTypeName) allNameSet {
	if len(all) < 1 {
		p.bug("AllNameSet Is Empty")
	}
	return allNameSet(all)
}

func isSimple(t tboType) bool {
	switch tt := t.(type) {
	case baseType:
		return isSimpleByBase(tt)
	case simpleType:
		return true
	case anyType:
		for _, ct := range tt {
			if !isSimple(ct) {
				return false
			}
		}
		return true
	}
	return false
}

func isSimpleByBase(b baseType) bool {
	switch string(b) {
	case "int", "uint", "string", "bytes":
		return true
	default:
		return false
	}
}

func accStruct(t tboType, acc map[string]*structType) {
	switch tt := t.(type) {
	case *structType:
		if _, ok := acc[tt.name]; ok {
			return
		}
		acc[tt.name] = tt
		for _, f := range tt.fileds {
			accStruct(f.t, acc)
		}
	case *structIndex:
		accStruct(tt.t, acc)
	case *setType:
		accStruct(tt.vt, acc)
	case *indexType:
		for _, b := range tt.binds {
			accStruct(b.t.vt, acc)
		}
	case *arrayType:
		accStruct(tt.element, acc)
	case *mapType:
		accStruct(tt.key, acc)
		accStruct(tt.value, acc)
	case anyType:
		for _, ct := range tt {
			accStruct(ct, acc)
		}
	case allset:
		for _, ct := range tt {
			accStruct(ct, acc)
		}
	}
}
