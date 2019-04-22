package tbo

import (
	"fmt"
	"strings"

	"github.com/tealeg/xlsx"
	//
	log "github.com/sirupsen/logrus"
)

type sheet struct {
	set   string // 绑定类型名字
	sheet string // 绑定数据片类型

	rows *groupValue
	cols *groupValue

	data *xlsx.Sheet
}

func (s sheet) cellToString(row, col int) string {
	if row > s.rows.size() || col > s.cols.size() {
		throw("Cell(%d,%d) over Sheet %s Range Max(%d,%d)", row, col, s.String(), s.rows.size()-1, s.cols.size()-1)
	}
	ci, _ := s.cols.index(col)
	if ci == -1 {
		throw("Cell(%d,%d) col %d is Group", row, col, ci)
	}
	ri, _ := s.rows.index(row)
	log.Debugf("%v max(%d:%d) cell(%d:%d) index(%d:%d)", s.rows, s.rows.size(), s.cols.size(), row, col, ri, ci)
	return s.data.Cell(ri, ci).String()
}

func (s *sheet) reset() {
	if s.data == nil {
		return
	}

	if s.rows.size() == -1 {
		for row := s.data.MaxRow - 1; row > -1; row-- {
			for i := s.cols.size() - 1; i > -1; i-- {
				col, _ := s.cols.index(i)
				if col == -1 {
					continue
				}
				if s.data.Cell(row, col).String() != "" {
					s.rows.reset(row)
					return
				}
			}
		}
	}
}

func (s sheet) String() string {
	return strings.Join([]string{s.set, ".", s.sheet, s.cols.String(), s.rows.String()}, "")
}

type sheetValue interface {
	index(i int) (int, sheetValue)
	size() int
	String() string
}

type rangeValue struct {
	min int
	max int
	i   bool
}

func (rv rangeValue) index(i int) (int, sheetValue) {
	return rv.min + i, nil
}

func (rv rangeValue) size() int {
	return rv.max - rv.min + 1
}

func (rv rangeValue) String() (s string) {
	if rv.i {
		if rv.min != rv.max {
			s = fmt.Sprintf("%d.%d", rv.min+1, rv.max+1)
		} else if rv.max == -1 {
			s = fmt.Sprintf("%d.", rv.min+1)
		} else {
			s = fmt.Sprintf("%d", rv.min+1)
		}
	} else {
		if rv.min != rv.max {
			s = strings.Join([]string{index2Alphabet(rv.min + 1), index2Alphabet(rv.max + 1)}, ".")
		} else if rv.max == -1 {
			s = strings.Join([]string{index2Alphabet(rv.min + 1), "."}, "")
		} else {
			s = index2Alphabet(rv.min + 1)
		}
	}
	return
}

type groupValue struct {
	rs    []sheetValue
	size_ int
}

func (gv groupValue) index(i int) (m int, c sheetValue) {
	for _, v := range gv.rs {
		if rv, ok := v.(*rangeValue); ok {
			m, _ = rv.index(i)
			i = i - rv.size()
			c = nil
		} else {
			m = -1
			i--
			c = v
		}
		if i < 0 {
			break
		}
	}
	return
}

func (gv groupValue) size() int {
	return gv.size_
}

func (gv groupValue) String() string {
	var rs []string
	for _, v := range gv.rs {
		rs = append(rs, v.String())
	}
	return strings.Join([]string{"[", strings.Join(rs, " "), "]"}, "")
}

func (gv *groupValue) reset(max int) {
	lv := gv.rs[len(gv.rs)-1]
	switch vv := lv.(type) {
	case *rangeValue:
		if vv.max == -1 {
			if max == -1 {
				gv.size_ = -1
				return
			}
			vv.max = max
		}
	case *groupValue:
		vv.reset(max)
	}
	gv.size_ = 0
	for _, v := range gv.rs {
		switch vv := v.(type) {
		case *rangeValue:
			gv.size_ += vv.size()
		case *groupValue:
			gv.size_++
		}
	}
}

func (gv *groupValue) generalization() (g int) {
	for _, v := range gv.rs {
		switch vv := v.(type) {
		case *rangeValue:
			if vv.max == -1 {
				g++
			}
		case *groupValue:
			g += vv.generalization()
		}
	}
	return
}

func (p *parser) tboSheet(set, name string) *sheet {
	return &sheet{
		set:   set,
		sheet: name,
	}
}

func (p *parser) tboRangeValue(min, max int) *rangeValue {
	return &rangeValue{
		min: min,
		max: max,
	}
}

func (p *parser) tboGroupValue(rs []sheetValue) *groupValue {
	gv := &groupValue{
		rs: rs,
	}
	gv.reset(-1)
	return gv
}

func (p *parser) tboCols(rs []sheetValue) (gv *groupValue) {
	gv = p.tboGroupValue(rs)
	if gv.generalization() > 0 {
		p.panicf("Sheet Col no generalization")
	}
	return
}

func (p *parser) tboRows(rs []sheetValue) (gv *groupValue) {
	gv = p.tboGroupValue([]sheetValue(rs))
	if gv.generalization() > 1 {
		p.panicf("Sheet Row no multi generalization")
	}
	return
}
