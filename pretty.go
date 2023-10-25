package pp

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/zeebo/sudo"
)

var (
	stringerType = reflect.TypeOf(new(fmt.Stringer)).Elem()
	uint64type   = reflect.TypeOf(uint64(0))
	timeType     = reflect.TypeOf(time.Time{})
)

const (
	indention = "    "
	padding   = " "
)

func New(w io.Writer) *Printer {
	return &Printer{
		Writer:      w,
		ByteEncoder: base64.URLEncoding.EncodeToString,
	}
}

func Fprint(w io.Writer, x interface{}) (err error) {
	return New(w).Print(x)
}

func Fprintln(w io.Writer, x interface{}) (err error) {
	return New(w).Println(x)
}

func Println(x interface{}) (err error) {
	return Fprintln(os.Stdout, x)
}

func Print(x interface{}) error {
	return Fprint(os.Stdout, x)
}

func Sprint(x interface{}) string {
	var buf strings.Builder
	Fprint(&buf, x)
	return buf.String()
}

func Sprintln(x interface{}) string {
	var buf strings.Builder
	Fprintln(&buf, x)
	return buf.String()
}

type Printer struct {
	Writer         io.Writer
	ByteEncoder    func([]byte) string
	HideUnexported bool
	HexIntegers    bool
	OmitAddress    bool
}

type printerState struct {
	pp        Printer
	indention string
	padding   string
	err       error
	visited   map[uintptr]bool
}

func (pp Printer) Print(x interface{}) error {
	return pp.print(x, false)
}

func (pp Printer) Println(x interface{}) error {
	return pp.print(x, true)
}

func (pp Printer) Sprint(x interface{}) string {
	var buf strings.Builder
	pp.Writer = &buf
	pp.Print(x)
	return buf.String()
}

func (pp Printer) Sprintln(x interface{}) string {
	var buf strings.Builder
	pp.Writer = &buf
	pp.Println(x)
	return buf.String()
}

func (pp Printer) print(x interface{}, nl bool) error {
	pps := &printerState{
		pp:      pp,
		visited: map[uintptr]bool{},
	}
	if x == nil {
		pps.printf("<nil>\n")
		return pps.err
	}
	xtype := reflect.TypeOf(x)
	if xtype.Kind() == reflect.Struct {
		pps.printf("%s ", xtype.Name())
	}
	xv := reflect.ValueOf(x)
	if nl {
		pps.printValueLine(xv, 0)
	} else {
		pps.printValue(xv, 0)
	}
	return pps.err
}

func (pps *printerState) failed() bool {
	return pps.err != nil
}

func (pps *printerState) printValueLine(value reflect.Value, n int) {
	pps.printValue(value, n)
	pps.printf("\n")
}

func (pps *printerState) printValue(value reflect.Value, n int) {
	if pps.failed() {
		// short-circuit if an error has been encountered
		return
	}

	value = sudo.Sudo(value)
	vtype := value.Type()

	switch vtype.Kind() {
	case reflect.Ptr:
		if value.IsNil() {
			pps.printf("<nil>")
		} else {
			indirect := reflect.Indirect(value)
			key := value.Pointer()
			if pps.visited[key] {
				if pps.pp.OmitAddress {
					pps.printf("<CYCLIC REFERENCE: %s>", vtype)
				} else {
					pps.printf("<CYCLIC REFERENCE: [%08x] %s>", value.Pointer(), vtype)
				}
				return
			}
			if !pps.pp.OmitAddress {
				pps.printf("[%08x] ", value.Pointer())
			}
			pps.visited[key] = true
			pps.printValue(indirect, n)
			delete(pps.visited, key)
		}
	case reflect.Struct:
		if value.CanInterface() {
			if t, ok := value.Interface().(time.Time); ok {
				pps.printf("%s", t)
				break
			}
		}

		var nfields int
		var longest_name int
		for i := 0; i < vtype.NumField(); i++ {
			name := vtype.Field(i).Name
			if pps.pp.HideUnexported && !isExported(name) {
				continue
			}

			nfields++

			if nlen := len(vtype.Field(i).Name); nlen > longest_name {
				longest_name = nlen
			}
		}
		if nfields == 0 {
			pps.printf("%s {}", vtype)
			break
		}
		pps.printf("%s {\n", vtype)
		for i := 0; i < vtype.NumField(); i++ {
			name := vtype.Field(i).Name
			if pps.pp.HideUnexported && !isExported(name) {
				continue
			}
			pps.iprintf(n+1, "%s %s= ", name, pps.pad(longest_name-len(name)))
			pps.printValueLine(value.Field(i), n+1)
		}
		pps.iprintf(n, "}")
	case reflect.Slice, reflect.Array:
		if vtype.Elem().Kind() == reflect.Uint8 {
			tmp := make([]byte, value.Len())
			reflect.Copy(reflect.ValueOf(tmp), value)
			pps.printf("%s", pps.encode(tmp))
			break
		}
		if value.Len() == 0 {
			if vtype.Kind() == reflect.Slice && value.IsNil() {
				pps.printf("<nil>")
			} else {
				pps.printf("%s []", vtype)
			}
			break
		}
		pps.printf("%s [\n", vtype)
		for i := 0; i < value.Len(); i++ {
			pps.iprintf(n+1, "")
			pps.printValue(value.Index(i), n+1)
			pps.printf(",\n")
		}
		pps.iprintf(n, "]")
	case reflect.String:
		pps.printf("%q", value)
	case reflect.Map:
		keys := value.MapKeys()
		if len(keys) == 0 {
			if value.IsNil() {
				pps.printf("<nil>")
			} else {
				pps.printf("%s {}", vtype)
			}
			break
		}
		pps.printf("%s {\n", vtype)
		for _, key := range keys {
			pps.iprintf(n+1, "")
			pps.printValue(key, n+1)
			pps.printf(": ")
			pps.printValue(value.MapIndex(key), n+1)
			pps.printf(",\n")
		}
		pps.iprintf(n, "}")
	case reflect.Interface:
		elem := value.Elem()
		if elem.IsValid() {
			pps.printValue(elem, n)
		} else {
			pps.printf("<nil>")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:

		stringer := value.Type().Implements(stringerType)

		if pps.pp.HexIntegers && !stringer {
			pps.printf("0x%016x", value.Convert(uint64type))
		} else if value.CanInterface() {
			pps.printf("%v", value.Interface())
		} else {
			pps.printf("%v", value)
		}
	default:
		if value.CanInterface() {
			pps.printf("%v", value.Interface())
		} else {
			pps.printf("%v", value)
		}
	}
}

func isAscii(data []byte) bool {
	for _, v := range data {
		if v > 0x7f {
			return false
		}
	}
	return true
}

func (pps *printerState) encode(data []byte) string {
	if isAscii(data) {
		return fmt.Sprintf("%q", data)
	}
	if pps.pp.ByteEncoder == nil {
		return base64.URLEncoding.EncodeToString(data)
	}
	return pps.pp.ByteEncoder(data)
}

func (pps *printerState) indent(n int) string {
	if len(pps.indention) < n*len(indention) {
		pps.indention = strings.Repeat(indention, max(n, 10))
	}
	return pps.indention[:len(indention)*n]
}

func (pps *printerState) pad(n int) string {
	if len(pps.padding) < n*len(padding) {
		pps.padding = strings.Repeat(padding, max(n, 10))
	}
	return pps.padding[:len(padding)*n]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (pps *printerState) iprintf(indention int, format string,
	args ...interface{}) {

	if pps.failed() {
		return
	}
	_, pps.err = fmt.Fprintf(pps.pp.Writer, "%s"+format,
		append([]interface{}{pps.indent(indention)}, args...)...)
}

func (pps *printerState) printf(format string, args ...interface{}) {
	if pps.failed() {
		return
	}
	_, pps.err = fmt.Fprintf(pps.pp.Writer, format, args...)
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper([]rune(name)[0])
}
