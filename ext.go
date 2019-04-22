package tbo

import (
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// "erl", "hrl", "esys", "lua", "xml"

type printerType string

func (pt printerType) Base() (s string) {
	s = string(pt)
	switch s {
	case "hrl", "esys":
		s = "erl"
	}
	return
}

//分隔符等级 ' ' > ',' > ';'
//键值符对应 ':'   '='

type split string

func (s split) prev() (n split) {
	switch string(s) {
	case ",":
		n = split(" ")
	case ";":
		n = split(",")
	}
	return
}

func (s split) next() (n split) {
	switch string(s) {
	case " ":
		n = split(",")
	case ",":
		n = split(";")
	case ";":
		n = ""
	}
	return
}

func (s split) ends() []byte {
	if s.prev() == "" {
		return []byte{0}
	} else {
		return append(s.prev().ends(), []byte(string(s.prev()))[0])
	}
}

func (s split) kvSplit() string {
	if string(s) == " " {
		return ":"
	}
	return "="
}

type extBuf struct {
	buf []byte
	pos int
	cur int
}

func (b *extBuf) key(s split) (string, bool) {
	if string(s) != " " {
		b.space()
	}
	return b.fetch(s.ends(), []byte(s.kvSplit())[0], []byte(string(s))[0])
}

func (b *extBuf) value(s split) (string, bool) {
	if string(s) != " " {
		b.space()
	}
	return b.fetch(s.ends(), []byte(string(s))[0])
}

func (b *extBuf) fetch(ends []byte, sps ...byte) (v string, ok bool) {
	log.Debugf("ends:%v sps:%v", ends, sps)
	ok = true
	for b.pos < len(b.buf) {
		for _, end := range ends {
			if b.buf[b.pos] == end {
				ok = false
				v = b.trunc()
				return
			}
		}
		for _, sp := range sps {
			if b.buf[b.pos] == sp {
				v = b.trunc()
				b.pos++
				b.cur = b.pos
				return
			}
		}
		b.pos++
	}
	ok = false
	v = b.trunc()
	return
}

func (b *extBuf) trunc() (v string) {
	v = strings.TrimSpace(string(b.buf[b.cur:b.pos]))
	b.cur = b.pos
	return
}

func (b *extBuf) space() {
	for b.pos < len(b.buf) {
		if b.buf[b.pos] != ' ' {
			b.cur = b.pos
			return
		}
		b.pos++
	}
}

//
type ext struct {
	m map[string]interface{}
	s split
}

func (e *ext) parse(b *extBuf) {
	for {
		key, ok := b.key(e.s)
		log.Debugf("key:%s splict:%s ok:%t", key, e.s, ok)
		switch key {
		case
			"ignore",
			"replaceChildType":
			e.m[key] = true
		case
			"default",
			"prefix",
			"suffix",
			"template":
			val, _ := b.value(e.s)
			log.Debugf("value:%s", val)
			e.m[key] = val
		case
			"filter",
			"skipIndex":
			var vs []string
			for {
				val, vok := b.value(e.s.next())
				log.Debugf("value:%s", val)
				vs = append(vs, val)
				if !vok {
					break
				}
			}
			if len(vs) == 1 {
				e.m[key] = vs[0]
			} else {
				e.m[key] = vs
			}
		case "erl", "hrl", "esys", "lua", "xml": // 输出器 第一级key
			ee := &ext{
				m: make(map[string]interface{}),
				s: e.s.next(),
			}
			ee.parse(b)
			e.m[printerType(key).Base()] = ee
			b.space()
		}
		if !ok {
			break
		}
	}
}

func (e ext) merge(es ...*ext) (ne *ext) {
	ne = &ext{
		m: make(map[string]interface{}),
	}
	for _, oe := range append(es, &e) {
		if oe == nil {
			continue
		}
		for k, ov := range oe.m {
			if nv, ok := ne.m[k]; ok {
				switch vv := ov.(type) {
				case *ext:
					ne.m[k] = nv.(*ext).merge(vv)
				case []string:
					ne.m[k] = append(nv.([]string), vv...)
				}
			} else {
				ne.m[k] = ov
			}
		}
		ne.s = oe.s
	}
	return
}

func (e ext) key(name, key string) bool {
	if v, ok := e.m[printerType(name).Base()]; ok {
		if ee, ok := v.(*ext); ok {
			_, ok = ee.m[key]
			return ok
		}
	}
	return false
}

func (e ext) ignore(name string) bool {
	return e.key(printerType(name).Base(), "ignore")
}

func (e ext) filter(name string, pv string) bool {
	log.Debugf("name:%s %v", name, e.m)
	if v, ok := e.m[printerType(name).Base()]; ok {
		if ee, ok := v.(*ext); ok {
			if v, ok := ee.m["filter"]; ok {
				switch vv := v.(type) {
				case string:
					return e.checkRangeVal(vv, pv)
				case []string:
					for _, n := range vv {
						if e.checkRangeVal(n, pv) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (e ext) replaceChildType(name string) bool {
	return e.key(printerType(name).Base(), "replaceChildType")
}

func (e ext) value(name, key string) string {
	if v, ok := e.m[printerType(name).Base()]; ok {
		if ee, ok := v.(*ext); ok {
			if v, ok := ee.m[key]; ok {
				if s, ok := v.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func (e ext) prefix(name string) string {
	return e.value(printerType(name).Base(), "prefix")
}

func (e ext) suffix(name string) string {
	return e.value(printerType(name).Base(), "suffix")
}

func (e ext) template(name string) string {
	return e.value(printerType(name).Base(), "template")
}

func (e ext) skipIndex(name, sn string) bool {
	if v, ok := e.m[printerType(name).Base()]; ok {
		if ee, ok := v.(*ext); ok {
			if v, ok := ee.m["skipIndex"]; ok {
				switch vv := v.(type) {
				case string:
					if vv == "all" {
						return true
					}
					return vv == sn
				case []string:
					for _, n := range vv {
						if n == sn {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (e ext) checkRangeVal(p, v string) (ok bool) {
	ok = (p == v)
	pp := strings.Split(p, "-")
	if len(pp) != 2 {
		return
	}
	val, er := strconv.ParseInt(v, 10, 64)
	if er != nil {
		return
	}
	min, er := strconv.ParseInt(pp[0], 10, 64)
	if er != nil {
		return
	}
	max, er := strconv.ParseInt(pp[1], 10, 64)
	if er != nil {
		return
	}
	log.Debugf("ext min:%d val:%d max:%d", min, val, max)
	return min <= val && val <= max
}

func (e ext) String() (s string) {
	var ss []string
	for k, v := range e.m {
		switch vv := v.(type) {
		case ext:
			ss = append(ss, strings.Join([]string{k, vv.String()}, e.s.kvSplit()))
		case []string:
			ss = append(ss, strings.Join([]string{k, strings.Join(vv, e.s.next().kvSplit())}, e.s.kvSplit()))
		case bool:
			ss = append(ss, k)
		case string:
			ss = append(ss, strings.Join([]string{k, vv}, e.s.kvSplit()))
		}
	}
	if e.s.kvSplit() == ":" {
		s = strings.Join([]string{"`", strings.Join(ss, string(e.s)), "`"}, "")
	} else {
		s = strings.Join(ss, string(e.s))
	}
	return
}

func tboExt(s string) *ext {
	if s == "" {
		return nil
	}
	e := &ext{
		m: make(map[string]interface{}),
		s: split(" "),
	}
	e.parse(&extBuf{buf: []byte(strings.TrimSpace(s))})
	log.Debugf("ext `%s` %v", s, e)
	return e
}
