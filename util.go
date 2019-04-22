package tbo

import (
	"bytes"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func ChangeExtension(filename, newExt string) string {

	file := filepath.Base(filename)

	return strings.TrimSuffix(file, path.Ext(file)) + newExt
}

func StringToPrimitive(str string, value interface{}) (error, bool) {
	switch raw := value.(type) {
	case *int32:
		v, err := strconv.ParseInt(str, 10, 32)
		if err != nil {
			return err, false
		}

		*raw = int32(v)
	case *int64:
		v, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return err, false
		}

		*raw = v
	case *uint32:
		v, err := strconv.ParseUint(str, 10, 32)
		if err != nil {
			return err, false
		}

		*raw = uint32(v)
	case *uint64:
		v, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return err, false
		}

		*raw = v
	case *string:
		*raw = str
	case *bool:

		var v bool
		var err error

		switch str {
		case "是":
			v = true
		case "否", "":
			v = false
		default:
			v, err = strconv.ParseBool(str)
			if err != nil {
				return err, false
			}
		}

		*raw = v
	case *float32:
		v, err := strconv.ParseFloat(str, 32)
		if err != nil {
			return err, false
		}

		*raw = float32(v)
	case *float64:
		v, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return err, false
		}

		*raw = float64(v)

	default:
		return nil, false
	}

	return nil, true
}

func mod(a, b int) int {
	return int(math.Mod(float64(a), float64(b)))
}

func str2int(s string) int {
	return int([]byte(s)[0])
}

var asciiA = str2int("A")

const unit = 26

// 按excel格式 R1C1格式转A1
func index2Alphabet(number int) string {

	if number < 1 {
		return ""
	}

	n := number

	nl := make([]int, 0)

	for {

		quo := n / unit

		var reminder int
		x := mod(n, unit)

		// 余数为0时, 要跳过这个0, 重新计算除数(影响进位)
		if x == 0 {
			reminder = unit
			n--
			quo = n / unit
		} else {
			reminder = x
		}

		nl = append(nl, reminder)

		if quo == 0 {
			break
		}

		n = quo
	}

	var out bytes.Buffer

	for i := len(nl) - 1; i >= 0; i-- {

		v := nl[i]

		out.WriteString(string(v + asciiA - 1))
	}

	return out.String()
}

func alphabet2Index(s string) (i uint64) {
	i, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		byteBegin := byte(asciiA)
		byteEnd := byteBegin + unit
		i = 0
		for _, v := range []byte(s) {
			if v >= byteBegin && v < byteEnd {
				i = i*unit + uint64(v-byteBegin) + 1
			}
		}
	}
	i--
	return
}

// r,c都是base1
func R1C1ToA1(r, c int) string {
	return fmt.Sprintf("%s%d", index2Alphabet(c), r)
}

func StringEscape(s string) string {

	b := make([]byte, 0)

	var index int

	// 表中直接使用换行会干扰最终合并文件格式, 所以转成\n,由pbt文本解析层转回去
	for index < len(s) {
		c := s[index]

		switch c {
		case '"':
			b = append(b, '\\')
			b = append(b, '"')
		case '\n':
			b = append(b, '\\')
			b = append(b, 'n')
		case '\r':
			b = append(b, '\\')
			b = append(b, 'r')
		case '\\':

			var nextChar byte
			if index+1 < len(s) {
				nextChar = s[index+1]
			}

			b = append(b, '\\')

			switch nextChar {
			case 'n', 'r':
			default:
				b = append(b, c)
			}

		default:
			b = append(b, c)
		}

		index++

	}

	return fmt.Sprintf("\"%s\"", string(b))

}

func panicTrace(kb int) []byte {
	s := []byte("/src/runtime/panic.go")
	e := []byte("\ngoroutine ")
	line := []byte("\n")
	stack := make([]byte, kb<<10) //4KB
	length := runtime.Stack(stack, true)
	start := bytes.Index(stack, s)
	stack = stack[start:length]
	start = bytes.Index(stack, line) + 1
	stack = stack[start:]
	end := bytes.LastIndex(stack, line)
	if end != -1 {
		stack = stack[:end]
	}
	end = bytes.Index(stack, e)
	if end != -1 {
		stack = stack[:end]
	}
	stack = bytes.TrimRight(stack, "\n")
	return stack
}
