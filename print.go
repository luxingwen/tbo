package tbo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"sync"
	//
	log "github.com/sirupsen/logrus"
)

func print(ps []*tboPrint) (err error) {

	var w sync.WaitGroup
	msgc := make(chan string, len(ps))

	for _, p := range ps {
		for _, pr := range p.ps {
			w.Add(1)
			go pr.print(&w, msgc, p.chunk, p.structs)
		}
	}

	go func() {
		w.Wait()
		close(msgc)
	}()

	var emsgs []string

	for msg := range msgc {
		emsgs = append(emsgs, msg)
	}

	if len(emsgs) != 0 {
		err = errors.New(strings.Join(emsgs, "\n\n\t"))
	}

	return
}

type tboPrint struct {
	ts []tboType
	ps []*printer

	chunk   *chunk
	structs []*structType
}

var pm *printManager

func init() {
	pm = &printManager{
		printCreates: make(map[string]func() Printer),
	}
	RegisterPrinter("erl", func() Printer {
		p := new(erlPrinter)
		p.i = 0
		p.s = "erl"
		p.basePrinter.c = p
		return p
	})
	RegisterPrinter("hrl", func() Printer {
		p := new(hrlPrinter)
		p.i = 0
		p.s = "hrl"
		p.basePrinter.c = p
		return p
	})
	RegisterPrinter("esys", func() Printer {
		p := new(esysPrinter)
		p.i = 0
		p.s = "esys"
		p.kvs = make(map[string]string)
		p.basePrinter.c = p
		return p
	})
	RegisterPrinter("lua", func() Printer {
		p := new(luaPrinter)
		p.i = 0
		p.s = "lua"
		p.basePrinter.c = p
		return p
	})
	RegisterPrinter("xml", func() Printer {
		p := new(xmlPrinter)
		p.i = 0
		p.s = "xml"
		p.basePrinter.c = p
		return p
	})
	RegisterPrinter("json", func() Printer {
		p := new(jsonPrinter)
		p.i = 0
		p.s = "json"
		p.basePrinter.c = p
		return p
	})
	RegisterPrinter("go", func() Printer {
		p := new(goStructPrinter)
		p.i = 0
		p.s = "go"
		p.basePrinter.c = p
		return p
	})
}

func RegisterPrinter(name string, f func() Printer) {
	pm.printCreates[name] = f
}

type printer struct {
	typ  string
	op   string
	path string
	ext  *ext

	f *os.File
	t [][]byte

	p Printer
}

func (p *printer) print(w *sync.WaitGroup, msgc chan<- string, c *chunk, s []*structType) {
	defer func() {
		if p.f != nil {
			p.f.Close()
		}
		if r := recover(); r != nil {
			fmt.Printf("%s\n", panicTrace(1024))
			if e, ok := r.(error); ok {
				msgc <- fmt.Sprintf("%s Fail! \n%s", p.String(), e.Error())
			} else {
				msgc <- fmt.Sprintf("%s Fail! \n\t\tUnKonw Error : %v", p.String(), r)
			}
		}
		w.Done()
	}()

	p.p = pm.createPrinter(p.typ)

	p.reday()

	p.printChunk(c)

	p.printStructs(s)

	p.p.Close()
}

func (p *printer) reday() {
	var err error
	switch p.op {
	case ">":
		p.f, err = os.OpenFile(p.path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
		if err != nil {
			p.panicf("Cover Open File %s Error %s", p.path, err.Error())
		}
		p.f.Write([]byte(""))
	case ">>":
		p.f, err = os.OpenFile(p.path, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			p.panicf("Append Open File %s Error %s", p.path, err.Error())
		}
	case "~>":
		if p.ext == nil {
			p.panicf("Printer Not Ext")
		}
		var (
			template string = p.ext.template(p.typ)
			buf      []byte
		)
		buf, err = ioutil.ReadFile(template)
		if err != nil {
			p.panicf("Cover Read Template %s Error %s", template, err.Error())
		}
		p.t = bytes.Split(buf, []byte("@"))
		p.f, err = os.OpenFile(p.path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
		if err != nil {
			p.panicf("Cover Open File %s Error %s", p.path, err.Error())
		}
		p.f.Write([]byte(""))
	default:
		p.panicf("Bad Print OP %s", p.op)
	}

	p.p.Reset(p.f, p.ext)

}

func (p *printer) printChunk(c *chunk) {
	if !p.p.CheckChunk(c) {
		return
	}
	if len(p.t) > 0 {
		for _, bs := range p.t {
			log.Debugf("bs:%s", bs)
			if bs[0] == '~' {
				p.p.Set("format", "all")
			} else if bs[0] == '!' {
				p.p.Set("format", "not")
			} else {
				n := 0
				for i := len(bs) - 1; i > -1; i-- {
					if bs[i] == '\n' {
						break
					}
					n++
				}
				p.p.Set("indentation", strings.Repeat(" ", n))
				p.p.Write(bs)
				continue
			}
			b := string(bs[1:len(bs)])
			for _, cc := range c.cs {
				n := toString(cc.args[0])
				log.Debugf("n:%s b:%s", n, b)
				if n == b {
					p.p.PrintChunk(cc)
					break
				}
			}
		}
	} else {
		bc := tboChunk(chunkBegin)
		p.p.PrintChunk(bc)
		recede(bc)

		p.p.PrintChunk(c)

		ec := tboChunk(chunkEnd)
		p.p.PrintChunk(ec)
		recede(ec)
	}
}

func (p *printer) printStructs(ss []*structType) {
	p.p.PrintHead()
	for _, s := range ss {
		if p.p.CheckStruct(s) {
			p.p.PrintStruct(s)
		}
	}
}

func (p *printer) panicf(format string, v ...interface{}) {
	panic(parseError(fmt.Sprintf(format, v...)))
}

func (p *printer) String() string {
	var ext string
	if p.ext != nil {
		ext = p.ext.String()
	}
	return fmt.Sprintf("%s %s \"%s\" %s", p.typ, p.op, p.path, ext)
}

//

type Printer interface {
	Reset(w io.Writer, e *ext)
	Set(key string, value interface{})
	Get(key string) (value interface{})
	CheckChunk(c *chunk) bool
	CheckStruct(s *structType) bool
	PrintChunk(c *chunk)
	PrintIndex(c *chunk)
	PrintStruct(s *structType)
	PrintBase(t string, v interface{})
	PrintHead()
	Write([]byte)
	Close()
	String() string
}

type printManager struct {
	printCreates map[string]func() Printer
}

func (p *printManager) createPrinter(name string) Printer {
	f, ok := p.printCreates[name]
	if !ok {
		return nil
	}
	return f()
}

var (
	indentation = []byte("  ")
	sepB        = []byte("")
	indlen      = len(indentation)
)

type basePrinter struct {
	w io.Writer
	e *ext

	i int
	s string

	c Printer
	b *bytes.Buffer
}

func (p *basePrinter) Reset(w io.Writer, e *ext) {
	p.w = w
	p.e = e
}

func (p *basePrinter) Set(key string, value interface{}) {

}

func (p *basePrinter) Get(key string) (value interface{}) {
	return
}

func (p *basePrinter) printChunkFiled(c *chunk, e *ext) {
	oe := p.e
	if p.e == nil {
		p.e = e
	} else {
		p.e = p.e.merge(e)
	}
	vc := c.cs[0]
	if len(c.args) < 3 {
		p.PrintChunk(vc)
		p.e = oe
		return
	}
	if e != nil && toString(c.args[2]) == "d" {
		dv := e.value(p.s, "default")
		if dv == "" {
			p.PrintChunk(vc)
		} else {
			p.Write([]byte(dv))
		}
	} else {
		p.PrintChunk(vc)
	}
	p.e = oe
}

func (p *basePrinter) filterStruct(c *chunk) bool {
	switch c.flag {
	case chunkStructIndex:
		return p.filterStruct(c.cs[0])
	case chunkStruct:
		for _, cc := range c.cs {
			if cc.cs[0].flag == chunkBase {
				ext := cc.args[1].(*ext)
				if ext != nil && ext.filter(p.s, toString(cc.cs[0].args[1])) {
					return true
				}
			}
		}
	}
	return false
}

func (p *basePrinter) PrintBase(t string, v interface{}) {
	p.c.PrintBase(t, v)
}

func (p basePrinter) CheckChunk(c *chunk) bool {
	return false
}

func (p basePrinter) CheckStruct(s *structType) bool {
	return false
}

func (p *basePrinter) PrintChunk(c *chunk) {
	p.c.PrintChunk(c)
}

func (p *basePrinter) PrintIndex(c *chunk) {
	if p.e != nil {
		sn := toString(c.args[2])
		if p.e.skipIndex(p.s, sn) {
			p.PrintBase(toString(c.args[0]), c.args[1])
			return
		}
	}
	p.c.PrintChunk(c.cs[0])
}

func (p *basePrinter) PrintStruct(s *structType) {
	p.c.PrintStruct(s)
}

func (p basePrinter) String() string {
	return p.s
}

func (p *basePrinter) Close() {
	if p.b == nil {
		return
	}
	if p.b.Len() > 0 {
		if _, err := p.b.WriteTo(p.w); err != nil {
			throw("%s print chunk error %s", p.s, err.Error())
		}
	}
	p.b.Reset()
}

func (p *basePrinter) Write(b []byte) {
	if p.b != nil {
		if _, err := p.b.Write(b); err != nil {
			throw("%s print chunk error %s", p.s, err.Error())
		}
	} else {
		p.b = bytes.NewBuffer(b)
	}
	if p.b.Len() > 4096 {
		if _, err := p.b.WriteTo(p.w); err != nil {
			throw("%s print chunk error %s", p.s, err.Error())
		}
		p.b.Reset()
	}
}

func (p *basePrinter) PrintHead() {

}

type luaPrinter struct {
	basePrinter
}

func (p luaPrinter) CheckChunk(c *chunk) bool {
	return true
}

func (p *luaPrinter) PrintChunk(c *chunk) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(chunkError); ok {
				panic(e)
			} else {
				panic(chunkError{c: c, err: fmt.Sprintf("%v", r)})
			}
		}
	}()
	switch c.flag {
	case chunkBegin:
		p.Write([]byte("return\n"))
		p.i++
	case chunkEnd:
		p.i--
	case chunkSets:
		p.Write([]byte("{\n"))
		bi := bytes.Repeat(indentation, p.i)
		b := false
		log.Debugf("all set child len:%d", len(c.cs))
		for _, cc := range c.cs {
			log.Debugf("set %s", cc.args[0])
			for _, ccc := range cc.cs {
				for _, cccc := range ccc.cs {
					if p.filterStruct(cccc) {
						continue
					}
					if b {
						if ccc.flag == chunkEnum {
							p.Write([]byte("\n"))
						} else {
							p.Write([]byte(",\n"))
						}
						p.Write(bi)
					} else {
						b = true
						p.Write(bi)
					}
					p.PrintChunk(cccc)
				}
			}
		}
		p.Write([]byte("\n}"))
	case chunkSet:
		p.PrintChunk(c.cs[0])
	case chunkIndex:
		p.PrintIndex(c)
	case chunkMap, chunkArray, chunkEnum:
		p.Write([]byte("{\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		b := false
		for _, cc := range c.cs {
			if b {
				p.Write([]byte(",\n"))
			} else {
				b = true
			}
			p.Write(bi)
			p.PrintChunk(cc)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("}")}, sepB))
	case chunkStructIndex:
		ic, ok := c.args[2].(*chunk)
		if !ok {
			throw("structIndex %s.%s Not Index", toString(c.args[0]), toString(c.args[1]))
		}
		p.Write([]byte("["))
		p.PrintChunk(ic.cs[0])
		p.Write([]byte("] = "))
		p.i++
		p.PrintChunk(c.cs[0])
		p.i--
	case chunkStruct:
		p.Write([]byte("{\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		b := false
		for _, cc := range c.cs {
			ext := cc.args[1].(*ext)
			if ext != nil && ext.ignore(p.s) {
				continue
			}
			if b {
				p.Write([]byte(",\n"))
			} else {
				b = true
			}
			p.Write(bi)
			p.Write(toBytes(cc.args[0]))
			p.Write([]byte(" = "))
			p.printChunkFiled(cc, ext)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("}")}, sepB))
	case chunkMapElement:
		p.Write([]byte("["))
		p.PrintChunk(c.cs[0].cs[0])
		p.Write([]byte("] = "))
		p.i++
		p.PrintChunk(c.cs[1].cs[0])
		p.i--
	case chunkEnumElement:
		b := toBytes(c.cs[0].cs[0].args[1])
		p.Write(b)
		p.Write(bytes.Repeat([]byte(" "), 32-len(b)))
		p.Write([]byte(" = "))
		p.PrintChunk(c.cs[1].cs[0])
		p.Write([]byte(", --"))
		p.Write(toBytes(c.cs[2].cs[0].args[1]))
	case chunkBase:
		p.PrintBase(toString(c.args[0]), c.args[1])
	}
}

func (p *luaPrinter) PrintIndex(c *chunk) {
	if p.e != nil {
		sn := toString(c.args[2])
		if p.e.skipIndex(p.s, sn) {
			p.PrintBase(toString(c.args[0]), c.args[1])
			return
		}
	}
	if c.p != nil && c.p.flag == chunkArray {
		p.Write([]byte("["))
		p.PrintBase(toString(c.args[0]), c.args[1])
		p.Write([]byte("] = "))
		p.i++
		p.PrintChunk(c.cs[0])
		p.i--
		return
	}
	p.PrintChunk(c.cs[0])
}

func (p *luaPrinter) PrintBase(t string, v interface{}) {
	switch t {
	case "string", "bytes":
		p.Write([]byte(fmt.Sprintf("%q", v)))
	default:
		p.Write(toBytes(v))
	}
}

type erlPrinter struct {
	basePrinter
}

func (p erlPrinter) CheckChunk(c *chunk) bool {
	if c.flag != chunkSets {
		return false
	}
	for _, cc := range c.cs {
		for _, ccc := range cc.cs {
			switch ccc.flag {
			case chunkArray:
			default:
				return false
			}
		}
	}
	return true
}

func (p *erlPrinter) PrintChunk(c *chunk) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(chunkError); ok {
				panic(e)
			} else {
				panic(chunkError{c: c, err: fmt.Sprintf("%v", r)})
			}
		}
	}()
	switch c.flag {
	case chunkSets:
		for _, cc := range c.cs {
			for _, ccc := range cc.cs {
				for _, cccc := range ccc.cs {
					if p.filterStruct(cccc) {
						continue
					}
					p.PrintChunk(cccc)
					p.Write([]byte(".\n"))
				}
			}
		}
	case chunkSet, chunkStructIndex:
		p.PrintChunk(c.cs[0])
	case chunkIndex:
		p.PrintIndex(c)
	case chunkMap:
		p.Write([]byte("#{\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		b := false
		for _, cc := range c.cs {
			if b {
				p.Write([]byte(",\n"))
			} else {
				b = true
			}
			p.Write(bi)
			p.PrintChunk(cc)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("}")}, sepB))
	case chunkArray:
		p.Write([]byte("[\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		b := false
		for _, cc := range c.cs {
			if b {
				p.Write([]byte(",\n"))
			} else {
				b = true
			}
			p.Write(bi)
			p.PrintChunk(cc)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("]")}, sepB))
	case chunkStruct:
		p.Write([]byte("{\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		p.Write(bi)
		p.printRecordName(toBytes(c.args[0]))
		for _, cc := range c.cs {
			ext := cc.args[1].(*ext)
			if ext != nil && ext.ignore(p.s) {
				continue
			}
			p.Write([]byte(",\n"))
			p.Write(bi)
			p.printChunkFiled(cc, ext)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("}")}, sepB))
	case chunkMapElement:
		p.PrintChunk(c.cs[0].cs[0])
		p.Write([]byte(" => "))
		p.i++
		p.PrintChunk(c.cs[1].cs[0])
		p.i--
	case chunkBase:
		p.PrintBase(toString(c.args[0]), c.args[1])
	}
}

func (p *erlPrinter) PrintBase(t string, v interface{}) {
	switch t {
	case "string":
		p.Write([]byte(fmt.Sprintf("%q", v)))
	case "bytes":
		p.Write(bytes.Join([][]byte{[]byte("<<\""), toBytes(v), []byte("\">>")}, sepB))
	default:
		p.Write(toBytes(v))
	}
}

func (p *erlPrinter) printRecordName(n []byte) {
	if p.e != nil {
		log.Debugf("p.v :%v", p.e)
		n = bytes.Join([][]byte{[]byte(p.e.prefix(p.s)), n, []byte(p.e.suffix(p.s))}, sepB)
	}
	if isUpper(rune(n[0])) {
		p.Write(bytes.Join([][]byte{[]byte("'"), n, []byte("'")}, sepB))
	} else {
		p.Write(n)
	}
}

type hrlPrinter struct {
	erlPrinter
}

func (p hrlPrinter) CheckChunk(c *chunk) bool {
	if c == nil || c.flag != chunkSets {
		return false
	}
	for _, cc := range c.cs {
		for _, ccc := range cc.cs {
			switch ccc.flag {
			case chunkEnum:
			default:
				return false
			}
		}
	}
	return true
}

func (p hrlPrinter) CheckStruct(s *structType) bool {
	return true
}

func (p hrlPrinter) PrintIndex(c *chunk) {

}

func (p *hrlPrinter) PrintChunk(c *chunk) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(chunkError); ok {
				panic(e)
			} else {
				panic(chunkError{c: c, err: fmt.Sprintf("%v", r)})
			}
		}
	}()
	switch c.flag {
	case chunkSets, chunkEnum:
		for _, cc := range c.cs {
			p.PrintChunk(cc)
		}
	case chunkSet:
		p.PrintChunk(c.cs[0])
	case chunkEnumElement:
		p.Write([]byte("-define("))
		p.PrintChunk(c.cs[0])
		p.PrintChunk(c.cs[1])
		p.Write([]byte("). %% "))
		p.PrintChunk(c.cs[2])
		p.Write([]byte("\n"))
	case chunkEnumName:
		p.printDefineName(toBytes(c.cs[0].args[1]))
	case chunkEnumValue:
		p.erlPrinter.PrintChunk(c.cs[0])
	case chunkEnumDesc:
		p.Write(toBytes(c.cs[0].args[1]))
	}
}

func (p *hrlPrinter) printDefineName(n []byte) {
	n[0] = bytes.ToUpper([]byte{n[0]})[0]
	b := bytes.Join([][]byte{n, []byte(",")}, sepB)
	p.Write(b)
	p.Write(bytes.Repeat([]byte(" "), 24-len(b)))
}

func (p *hrlPrinter) PrintStruct(s *structType) {
	p.Write([]byte("\n-record("))
	p.erlPrinter.printRecordName([]byte(s.name))
	p.Write([]byte(",\n"))
	p.Write(bytes.Join([][]byte{indentation, []byte("{\n")}, sepB))
	bi := bytes.Repeat(indentation, 2)
	b := false
	desc := ""
	for _, f := range s.fileds {
		if f.ext != nil && f.ext.ignore(p.s) {
			continue
		}
		if b {
			p.Write([]byte(","))
			if len(desc) > 0 {
				p.Write([]byte(" %% "))
				p.Write([]byte(desc))
			}
			p.Write([]byte("\n"))
		} else {
			b = true
		}
		p.Write(bi)
		bn := []byte(f.name)
		p.Write(bn)
		p.Write(bytes.Repeat([]byte(" "), 16-len(bn)))
		p.Write([]byte(":: "))
		p.printStructFiledType(f.t)
		desc = f.desc
	}
	if len(desc) > 0 {
		p.Write([]byte(" %% "))
		p.Write([]byte(desc))
	}
	p.Write(bytes.Join([][]byte{[]byte("\n"), indentation, []byte("}).\n")}, sepB))

	p.Write([]byte("\n-type "))
	p.erlPrinter.printRecordName([]byte(s.name))
	p.Write([]byte("() :: #"))
	p.erlPrinter.printRecordName([]byte(s.name))
	p.Write([]byte("{}.\n"))
}

func (p *hrlPrinter) printStructFiledType(t tboType) {
	switch tt := t.(type) {
	case baseType:
		switch tt.String() {
		case "int":
			p.Write([]byte("integer()"))
		case "uint":
			p.Write([]byte("non_neg_integer()"))
		case "float":
			p.Write([]byte("float()"))
		case "bool":
			p.Write([]byte("boolean()"))
		case "bytes":
			p.Write([]byte("bitstring()"))
		case "string":
			p.Write([]byte("string()"))
		}
	case *arrayType:
		p.Write([]byte("["))
		p.printStructFiledType(tt.element)
		p.Write([]byte("]"))
	case *mapType:
		p.Write([]byte("#{"))
		p.printStructFiledType(tt.key)
		p.Write([]byte(" => "))
		p.printStructFiledType(tt.value)
		p.Write([]byte("}"))
	case *structType:
		p.erlPrinter.printRecordName([]byte(tt.name))
		p.Write([]byte("()"))
	case *structIndex:
		p.erlPrinter.printRecordName([]byte(tt.t.name))
		p.Write([]byte("()"))
	case *setType:
		p.printStructFiledType(tt.t)
	case *indexType:
		b := false
		for _, bi := range tt.binds {
			if b {
				p.Write([]byte("|"))
			} else {
				b = true
			}
			p.printStructFiledType(bi.t.vt)
		}
	case simpleType:
		b := false
		if b {
			p.Write([]byte("|"))
		} else {
			b = true
		}
		for _, ct := range tt {
			p.printStructFiledType(ct)
		}
	case anyType:
		b := false
		for _, ct := range tt {
			if b {
				p.Write([]byte("|"))
			} else {
				b = true
			}
			p.printStructFiledType(ct)
		}
	case allset:
		p.Write([]byte("["))
		b := false
		for _, ct := range tt {
			if b {
				p.Write([]byte("|"))
			} else {
				b = true
			}
			p.printStructFiledType(ct)
		}
		p.Write([]byte("]"))
	default:
		p.Write([]byte("any()"))
	}
}

type esysPrinter struct {
	erlPrinter
	kvs map[string]string
}

func (p *esysPrinter) Set(key string, value interface{}) {
	if v, ok := value.(string); ok {
		p.kvs[key] = v
	}
}

func (p esysPrinter) Get(key string) (value interface{}) {
	return p.kvs[key]
}

func (p esysPrinter) indentation() string {
	return p.kvs["indentation"]
}

func (p esysPrinter) format() string {
	if v, ok := p.kvs["format"]; ok {
		return v
	}
	return "not"
}

func (p esysPrinter) CheckChunk(c *chunk) bool {
	if c.flag != chunkSets {
		return false
	}
	for _, cc := range c.cs {
		for _, ccc := range cc.cs {
			switch ccc.flag {
			case chunkMap:
			case chunkArray:
			default:
				return false
			}
		}
	}
	return true
}

func (p *esysPrinter) PrintChunk(c *chunk) {
	switch c.flag {
	case chunkSet:
		p.i = int(math.Ceil(float64(len(p.indentation()) / indlen)))
		p.PrintChunk(c.cs[0])
	case chunkMap, chunkArray:
		if p.format() == "not" {
			p.erlPrinter.PrintChunk(c)
			return
		}
		p.i++
		b := false
		for _, cc := range c.cs {
			if b {
				p.Write([]byte(",\n"))
				p.Write([]byte(p.indentation()))
			} else {
				b = true
			}
			p.PrintChunk(cc)
		}
		p.i--
	case chunkStructIndex:
		ic, ok := c.args[2].(*chunk)
		if !ok {
			throw("structIndex %s.%s Not Index", toString(c.args[0]), toString(c.args[1]))
		}
		p.Write([]byte("{"))
		p.printEnvKey(ic.cs[0])
		p.Write([]byte(","))
		p.i++
		p.erlPrinter.PrintChunk(c.cs[0])
		p.i--
		p.Write([]byte("}"))
	case chunkMapElement:
		p.Write([]byte("{"))
		p.printEnvKey(c.cs[0].cs[0])
		p.Write([]byte(","))
		p.erlPrinter.PrintChunk(c.cs[1].cs[0])
		p.Write([]byte("}"))
	default:
		p.erlPrinter.PrintChunk(c)
	}
}

func (p *esysPrinter) printEnvKey(c *chunk) {
	switch toString(c.args[0]) {
	case "string", "bytes":
		s := []byte("'")
		b := bytes.Join([][]byte{s, toBytes(c.args[1]), s}, sepB)
		p.Write(b)
	default:
		p.Write(toBytes(c.args[1]))
	}
}

func toBytes(i interface{}) []byte {
	b, ok := i.(string)
	if !ok {
		throw("%v to Bytes Fail", i)
	}
	return []byte(b)
}

func toString(i interface{}) string {
	s, ok := i.(string)
	if !ok {
		throw("%v to String Fail", i)
	}
	return s
}

type xmlPrinter struct {
	basePrinter
	pn []byte
}

func (p xmlPrinter) CheckChunk(c *chunk) bool {
	return true
}

func (p *xmlPrinter) PrintChunk(c *chunk) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(chunkError); ok {
				panic(e)
			} else {
				panic(chunkError{c: c, err: fmt.Sprintf("%v", r)})
			}
		}
	}()
	switch c.flag {
	case chunkBegin:
		p.Write([]byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"))
	case chunkSets:
		for _, cc := range c.cs {
			p.PrintChunk(cc)
		}
	case chunkSet:
		pi := bytes.Repeat(indentation, p.i)
		p.Write(pi)
		p.Write([]byte("<"))
		p.Write(toBytes(c.args[0]))
		p.Write([]byte(">\n"))
		p.PrintChunk(c.cs[0])
		p.Write(pi)
		p.Write([]byte("</"))
		p.Write(toBytes(c.args[0]))
		p.Write([]byte(">\n"))
	case chunkMap, chunkEnum:
		p.i++
		for _, cc := range c.cs {
			p.PrintChunk(cc)
		}
		p.i--
	case chunkArray:
		p.i++
		i := 0
		for _, cc := range c.cs {
			if cc.flag == chunkBase {
				bi := bytes.Repeat(indentation, p.i)
				p.Write(bi)
				p.Write([]byte("<Element Index=\""))
				p.Write([]byte(fmt.Sprintf("%d", i)))
				p.Write([]byte("\">"))
				p.PrintChunk(cc)
				p.Write([]byte("</Element>\n"))
				i++
				continue
			}
			p.PrintChunk(cc)
		}
		p.i--
	case chunkStruct:
		var sn []byte
		if p.pn != nil {
			sn = p.pn
		} else {
			sn = toBytes(c.args[0])
		}
		pi := bytes.Repeat(indentation, p.i)
		p.Write(pi)
		p.Write([]byte("<"))
		p.Write(sn)
		var childs []*chunk
		for _, cc := range c.cs {
			ext := cc.args[1].(*ext)
			if ext != nil && ext.ignore(p.s) {
				continue
			}

			vc := cc.cs[0]

			if vc.flag == chunkBase || (vc.flag == chunkIndex && ext != nil && ext.skipIndex(p.s, toString(vc.args[2]))) {
				p.Write([]byte(" "))
				p.Write(toBytes(cc.args[0]))
				p.Write([]byte("=\""))
				p.PrintBase(toString(vc.args[0]), vc.args[1])
				p.Write([]byte("\""))
			} else {
				childs = append(childs, cc)
			}
		}
		if len(childs) > 0 {
			p.Write([]byte(">\n"))
			p.i++
			bi := bytes.Repeat(indentation, p.i)
			pn := p.pn
			for _, cc := range childs {
				p.pn = nil
				cn := toBytes(cc.args[0])
				flag := cc.cs[0].flag
				checked := true
				switch flag {
				case chunkSets, chunkSet, chunkArray, chunkMap, chunkEnum, chunkIndex:
					checked = false
				}
				if checked && p.e.replaceChildType("xml") {
					p.pn = cn
					p.printChunkFiled(cc, cc.args[1].(*ext))
					continue
				}
				p.Write(bi)
				p.Write([]byte("<"))
				p.Write(cn)
				if flag == chunkIndex {
					p.Write([]byte(" Index=\""))
					p.Write(toBytes(cc.cs[0].args[1]))
					p.Write([]byte("\">\n"))
				} else if flag == chunkSet || checked {
					p.Write([]byte(">\n"))
				} else {
					p.Write([]byte(" Size=\""))
					p.Write([]byte(fmt.Sprintf("%d", len(cc.cs[0].cs))))
					p.Write([]byte("\">\n"))
				}
				p.i++
				p.printChunkFiled(cc, cc.args[1].(*ext))
				p.i--
				p.Write(bi)
				p.Write([]byte("</"))
				p.Write(cn)
				p.Write([]byte(">\n"))
			}
			p.pn = pn
			p.i--
			p.Write(pi)
			p.Write([]byte("</"))
			p.Write(sn)
			p.Write([]byte(">\n"))
		} else {
			p.Write([]byte("/>\n"))
		}
	case chunkMapElement:
		pi := bytes.Repeat(indentation, p.i)
		p.Write(pi)
		p.Write([]byte("<Key>"))
		p.PrintChunk(c.cs[0].cs[0])
		p.Write([]byte("</Key>\n"))
		p.Write(pi)
		p.Write([]byte("<Value>"))
		p.i++
		p.PrintChunk(c.cs[1].cs[0])
		p.i--
		p.Write([]byte("</Value>\n"))
	case chunkEnumElement:
		pi := bytes.Repeat(indentation, p.i)
		p.Write(pi)
		bn := toBytes(c.cs[0].cs[0].args[1])
		p.Write([]byte("<"))
		p.Write(bn)
		p.Write([]byte(" Desc=\""))
		p.Write(toBytes(c.cs[2].cs[0].args[1]))
		p.Write([]byte("\">"))
		p.i++
		p.PrintChunk(c.cs[1].cs[0])
		p.i--
		p.Write(pi)
		p.Write([]byte("</"))
		p.Write(bn)
		p.Write([]byte(">\n"))
	case chunkStructIndex:
		p.PrintChunk(c.cs[0])
	case chunkIndex:
		p.PrintIndex(c)
	case chunkBase:
		p.PrintBase(toString(c.args[0]), c.args[1])
	}
}

func (p *xmlPrinter) PrintBase(t string, v interface{}) {
	switch t {
	case "string", "bytes":
		b := []byte(fmt.Sprintf("%q", v))
		p.Write(b[1 : len(b)-1])
	case "bool":
		b := toBytes(v)
		b[0] = bytes.ToUpper([]byte{b[0]})[0]
		p.Write(b)
	default:
		p.Write(toBytes(v))
	}
}

type jsonPrinter struct {
	basePrinter
}

func (p *jsonPrinter) printRecordName(n []byte) {
	if p.e != nil {
		log.Debugf("p.v :%v", p.e)
		n = bytes.Join([][]byte{[]byte(p.e.prefix(p.s)), n, []byte(p.e.suffix(p.s))}, sepB)
	}
	if isUpper(rune(n[0])) {
		p.Write(append(n, sepB...))
	} else {
		p.Write(n)
	}
}

func (p jsonPrinter) CheckChunk(c *chunk) bool {
	if c == nil || c.flag != chunkSets {
		return false
	}
	for _, cc := range c.cs {
		for _, ccc := range cc.cs {
			switch ccc.flag {
			case chunkArray:
			default:
				return false
			}
		}
	}
	return true
}

func (p *jsonPrinter) PrintChunk(c *chunk) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(chunkError); ok {
				panic(e)
			} else {
				panic(chunkError{c: c, err: fmt.Sprintf("%v", r)})
			}
		}
	}()
	switch c.flag {

	case chunkBegin:
		p.Write([]byte("{\n"))
		p.i++
	case chunkEnd:
		p.i--
		p.Write([]byte("\n}"))
	case chunkSets:
		for _, cc := range c.cs {
			p.PrintChunk(cc)
		}
	case chunkSet:
		pi := bytes.Repeat(indentation, p.i)
		p.Write(pi)
		p.Write([]byte("\""))
		p.Write(toBytes(c.args[0]))
		p.Write([]byte("\""))
		p.Write([]byte(":"))
		p.PrintChunk(c.cs[0])
		p.Write(pi)
	case chunkStructIndex:
		p.PrintChunk(c.cs[0])

	case chunkIndex:
		p.PrintIndex(c)
	case chunkMap:
		p.Write([]byte("\""))
		p.Write(toBytes(c.args[0]))
		p.Write([]byte("\""))
		p.Write([]byte(":"))
		p.Write([]byte("{\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		b := false
		for _, cc := range c.cs {
			if b {
				p.Write([]byte(",\n"))
			} else {
				b = true
			}
			p.Write(bi)
			p.PrintChunk(cc)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("}")}, sepB))
	case chunkArray:
		p.Write([]byte("[\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		b := false
		for _, cc := range c.cs {
			if b {
				p.Write([]byte(",\n"))
			} else {
				b = true
			}
			p.Write(bi)
			p.PrintChunk(cc)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("]")}, sepB))
	case chunkStruct:
		p.Write([]byte("{\n"))
		p.i++
		bi := bytes.Repeat(indentation, p.i)
		b := false
		for _, cc := range c.cs {
			ext := cc.args[1].(*ext)
			if ext != nil && ext.ignore(p.s) {
				continue
			}
			if b {
				p.Write([]byte(",\n"))
			} else {
				b = true
			}
			p.Write(bi)
			p.Write([]byte("\""))
			p.Write(toBytes(cc.args[0]))
			p.Write([]byte("\""))
			p.Write([]byte(" : "))
			p.printChunkFiled(cc, ext)
		}
		p.i--
		p.Write(bytes.Join([][]byte{[]byte("\n"), bytes.Repeat(indentation, p.i), []byte("}")}, sepB))
	case chunkMapElement:
		p.Write([]byte("\""))
		p.PrintChunk(c.cs[0].cs[0])
		p.Write([]byte("\""))
		p.Write([]byte(" : "))
		p.i++
		p.PrintChunk(c.cs[1].cs[0])
		p.i--
	case chunkBase:
		p.PrintBase(toString(c.args[0]), c.args[1])
	}
}

func (p *jsonPrinter) PrintBase(t string, v interface{}) {
	switch t {
	case "string":
		p.Write([]byte(fmt.Sprintf("%q", v)))
	case "bytes":
		p.Write(bytes.Join([][]byte{[]byte("["), toBytes(v), []byte("]")}, sepB))
	default:
		p.Write(toBytes(v))
	}
}

type goStructPrinter struct {
	jsonPrinter
}

func (p *goStructPrinter) CheckChunk(c *chunk) bool {
	if c == nil || c.flag != chunkSets {
		return false
	}
	for _, cc := range c.cs {
		for _, ccc := range cc.cs {
			switch ccc.flag {
			case chunkEnum:
			default:
				return false
			}
		}
	}
	return true
}

func (p *goStructPrinter) CheckStruct(s *structType) bool {
	return true
}

func (p *goStructPrinter) PrintHead() {
	p.Write([]byte("package data\n"))
}

func (p *goStructPrinter) PrintStruct(s *structType) {
	p.Write([]byte("\ntype "))
	p.jsonPrinter.printRecordName([]byte(s.name))
	p.Write([]byte(" struct{\n"))
	p.Write(sepB)
	bi := bytes.Repeat(indentation, 2)
	b := false
	desc := ""
	for _, f := range s.fileds {
		if f.ext != nil && f.ext.ignore(p.s) {
			continue
		}
		if b {
			if len(desc) > 0 {
				p.Write([]byte(" // "))
				p.Write([]byte(desc))
			}
			p.Write([]byte("\n"))
		} else {
			b = true
		}
		p.Write(bi)
		bn := []byte(strings.Title(f.name))
		p.Write(bn)
		p.Write(bytes.Repeat([]byte(" "), 16-len(bn)))
		p.printStructFiledType(f.t)
		desc = f.desc
	}
	if len(desc) > 0 {
		p.Write([]byte(" // "))
		p.Write([]byte(desc))
	}
	p.Write(bytes.Join([][]byte{[]byte("\n"), []byte("}\n")}, sepB))

}

func (p *goStructPrinter) printStructFiledType(t tboType) {
	switch tt := t.(type) {
	case baseType:
		switch tt.String() {
		case "int":
			p.Write([]byte("int64"))
		case "uint":
			p.Write([]byte("uint64"))
		case "float":
			p.Write([]byte("float64"))
		case "bool":
			p.Write([]byte("bool"))
		case "bytes":
			p.Write([]byte("[]byte"))
		case "string":
			p.Write([]byte("string"))
		}
	case *arrayType:
		p.Write([]byte("[]"))
		p.printStructFiledType(tt.element)
	case *mapType:
		p.Write([]byte("map["))
		p.printStructFiledType(tt.key)
		p.Write([]byte("]"))
		p.printStructFiledType(tt.value)
	case *structType:
		p.Write([]byte("*"))
		p.jsonPrinter.printRecordName([]byte(tt.name))
	case *structIndex:
		p.Write([]byte("*"))
		p.jsonPrinter.printRecordName([]byte(tt.t.name))
	case *setType:
		p.printStructFiledType(tt.t)
	default:
		p.Write([]byte("interface{}"))
	}
}
