package tbo

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

func (p *parser) check() (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s\n", panicTrace(1024))
			if er, ok := r.(error); ok {
				err = er
			} else {
				err = errors.New(fmt.Sprintf("%v", r))
			}
		}
	}()

	p.checkExecls()
	p.checkTypes()
	p.checkPrinters()

	return
}

func (p *parser) checkExecls() {
	var (
		errc chan error
		wait sync.WaitGroup
	)

	errc = make(chan error)

	for _, e := range p.execls {
		e.loadFiles(&wait, errc)
	}

	go func() {
		wait.Wait()
		close(errc)
	}()

	var msgs []string
	for err := range errc {
		msgs = append(msgs, err.Error())
	}

	if len(msgs) != 0 {
		p.panicf("%s", strings.Join(msgs, "\n\t"))
	}
}

func (p *parser) checkTypes() {
	for k, t := range p.types {
		p.types[k] = p.preCheck(t)
	}
}

func (p parser) preCheck(t tboType) tboType {
	switch tt := t.(type) {
	case defineTypeName:
		if rt, ok := p.types[string(tt)]; ok {
			log.Debugf("Define Type <%s> Be Replace", tt)
			return rt
		}
		p.panicf("Not Found Define Type %s", tt.String())
	case structIndexName:
		log.Debugf("==================PreCheck SI %s=================", tt.String())
		name := strings.Join([]string{string(tt.t), tt.index}, ".")
		if si, ok := p.types[name]; ok {
			log.Debugf("Define Type <%s> Be Replace", tt.String())
			return si
		}
		if rt, ok := p.types[string(tt.t)]; ok {
			if st, ok := rt.(*structType); ok {
				si := p.tboStructIndex(tt.index, st)
				p.types[name] = si
				log.Debugf("Define Type <%s> Be Replace", tt.String())
				return si
			}
		}
		p.panicf("Not Found Define Type %s", tt.String())
	case *arrayType:
		tt.element = p.preCheck(tt.element)
		return tt
	case *mapType:
		tt.value = p.preCheck(tt.value)
		return tt
	case *structType:
		for i, f := range tt.fileds {
			log.Debugf("reply struct %s filed %s type:%s", tt.name, f.name, f.t.String())
			f.t = p.preCheck(f.t)
			log.Debugf("field %s new type %s ", f.name, f.t.String())
			tt.fileds[i] = f
		}
		return tt
	case anyType:
		for i, ct := range tt {
			tt[i] = p.preCheck(ct)
		}
		return tt
	case allNameSet:
		var as allset
		for _, vt := range tt {
			st, ok := p.preCheck(vt).(*setType)
			if !ok {
				p.panicf("NoFound %s Set", vt.String())
			}
			as = append(as, st)
		}
		return as
	case *setType:
		if tt.d.data != nil {
			return tt
		}
		tt.t = p.preCheck(tt.t)
		var ok bool
		tt.d.data, ok = p.execls[tt.d.set].sheet(tt.d.sheet)
		if !ok {
			p.panicf("Not Found Data %s.%s", tt.d.set, tt.d.sheet)
		}
		log.Debugf("set %s sheet %s.%s reset", tt.name, tt.d.set, tt.d.sheet)
		tt.d.reset()
		log.Debugf("cols:%d rows:%d", tt.d.cols.size(), tt.d.rows.size())
		isIndex := true
		switch ct := tt.t.(type) {
		case enumType:
			if tt.d.cols.size() != 3 {
				p.panicf("Set %s Child Type Enum Need 3 Col Data", tt.name)
			}
			tt.k = 0
			tt.kt = tboString_
			tt.vt = baseType(ct)
		case *mapType:
			if tt.d.cols.size() != 2 {
				p.panicf("Set %s Child Type Map Need 2 Col Data", tt.name)
			}
			tt.k = 0
			tt.kt = ct.key
			tt.vt = ct.value
		case *arrayType:
			cols := 0
			switch cct := ct.element.(type) {
			case *structIndex:
				cols = len(cct.t.fileds)
				tt.k = cct.i
				tt.kt = cct.index.t
				tt.vt = cct.t
			case *structType:
				cols = len(cct.fileds)
				isIndex = false
				tt.vt = cct
			default:
				p.panicf("Set %s Bad Child Type %s", tt.name, ct.element.String())
			}
			if tt.d.cols.size() != cols {
				p.panicf("Set %s Need %d Col Has %d Col Data", tt.name, cols, tt.d.cols.size())
			}
		}
		if isIndex {
			tt.i = make(map[string]int)
			for i := tt.d.rows.size() - 1; i > -1; i-- {
				k := tt.d.cellToString(i, tt.k)
				if k == "" {
					continue
				}
				if _, ok = tt.i[k]; ok {
					p.panicf("Set %s Index %s Same", tt.name, k)
				}
				tt.i[k] = i
			}
		}
		return tt
	case *indexType:
		ktacc := (tt.kt == nil)
		for i, b := range tt.binds {
			vt, ok := p.types[b.b]
			if !ok {
				p.panicf("Index %s Not Found Value Type", tt.String())
			}
			bt, ok := p.preCheck(vt).(*setType)
			if !ok {
				p.panicf("Index %s Value Type Not setType", tt.String())
			}
			if bt.i == nil {
				p.panicf("Index %s Value Type Not index setType", tt.String())
			}
			if ktacc {
				tt.kt = tt.kt.merge(bt.kt)
			}
			b.t = bt
			switch bt.t.(type) {
			case enumType:
				b.f =
					func(r int) tboValue {
						return tboBuffer(bt.d.cellToString(r, 1))
					}
			case *mapType:
				b.f =
					func(r int) tboValue {
						return tboBuffer(bt.d.cellToString(r, 1))
					}
			case *arrayType:
				b.f =
					func(r int) tboValue {
						rr, _ := bt.d.rows.index(r)
						return tboRecord(bt.d, rr)
					}
			}
			tt.binds[i] = b
		}
		return tt
	}
	return t
}

func (p *parser) checkPrinters() {

	for _, pr := range p.prints {

		for i, t := range pr.ts {
			dn, ok := t.(defineTypeName)
			if !ok {
				p.panicf("\t\tBad Print Type %s", t.String())
			}
			tt, ok := p.types[string(dn)]
			if !ok {
				p.panicf("\t\tNo Found Print Type %s", t.String())
			}
			pr.ts[i] = tt
		}

		sets := make([]*setType, 0)
		structs := make(map[string]*structType)

		for _, t := range pr.ts {
			switch tt := t.(type) {
			case *setType:
				sets = append(sets, tt)
			default:
				accStruct(tt, structs)
			}
		}

		if len(sets) != 0 {
			as := allset(sets)
			v := tboBuffer(as.Default())
			as.Value(v)
			pr.chunk = v.base()
			recede(v)
		}

		for _, s := range structs {
			pr.structs = append(pr.structs, s)
		}

		sort.Sort(structTypes(pr.structs))

	}
}

type structTypes []*structType

func (ss structTypes) Len() int      { return len(ss) }
func (ss structTypes) Swap(i, j int) { ss[i], ss[j] = ss[j], ss[i] }
func (ss structTypes) Less(i, j int) bool {
	return ss[i].name < ss[j].name
}
