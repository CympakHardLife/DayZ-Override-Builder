// Package rap parses binarized (raP) and plain-text RV engine configs
// (config.bin / config.cpp) into a shared AST, and can emit config.cpp text.
package rap

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

type EntryKind int

const (
	KindClass EntryKind = iota
	KindValue
	KindArray
	KindExtern // class Name;
	KindDelete // delete Name;
)

type ValueKind int

const (
	VString ValueKind = iota
	VFloat
	VInt
	VArray
	VVariable // raw unquoted token (macro/expression)
)

type Value struct {
	Kind ValueKind
	Str  string
	F    float64
	I    int64
	Arr  []*Value
}

type Entry struct {
	Kind       EntryKind
	Name       string
	Class      *Class   // KindClass
	Value      *Value   // KindValue
	Array      []*Value // KindArray
	ArrayFlags uint32   // 1 => "+=" append
}

type Class struct {
	Name    string
	Parent  string
	Entries []*Entry
}

// ---------- lookup helpers (config names are case-insensitive) ----------

func (c *Class) Get(name string) *Entry {
	if c == nil {
		return nil
	}
	for _, e := range c.Entries {
		if strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return nil
}

// Sub returns a nested class by name (nil if absent or not a class).
func (c *Class) Sub(name string) *Class {
	e := c.Get(name)
	if e == nil || e.Kind != KindClass {
		return nil
	}
	return e.Class
}

// Prop returns a simple value by name (nil if absent).
func (c *Class) Prop(name string) *Value {
	e := c.Get(name)
	if e == nil || e.Kind != KindValue {
		return nil
	}
	return e.Value
}

// ArrayProp returns array elements by name (nil if absent).
func (c *Class) ArrayProp(name string) []*Value {
	e := c.Get(name)
	if e == nil || e.Kind != KindArray {
		return nil
	}
	return e.Array
}

// Classes returns all direct child class definitions.
func (c *Class) Classes() []*Entry {
	var out []*Entry
	for _, e := range c.Entries {
		if e.Kind == KindClass {
			out = append(out, e)
		}
	}
	return out
}

// ---------- value formatting ----------

func (v *Value) Number() (float64, bool) {
	switch v.Kind {
	case VFloat:
		return v.F, true
	case VInt:
		return float64(v.I), true
	}
	return 0, false
}

func FormatFloat(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	// binarized configs store float32 — format at float32 precision
	return strconv.FormatFloat(f, 'g', -1, 32)
}

func (v *Value) String() string {
	switch v.Kind {
	case VString:
		return v.Str
	case VVariable:
		return v.Str
	case VFloat:
		return FormatFloat(v.F)
	case VInt:
		return strconv.FormatInt(v.I, 10)
	case VArray:
		parts := make([]string, len(v.Arr))
		for i, e := range v.Arr {
			parts[i] = e.String()
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return ""
}

func quoteCPP(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func (v *Value) cpp() string {
	switch v.Kind {
	case VString:
		return quoteCPP(v.Str)
	case VVariable:
		return v.Str
	case VFloat:
		return FormatFloat(v.F)
	case VInt:
		return strconv.FormatInt(v.I, 10)
	case VArray:
		parts := make([]string, len(v.Arr))
		for i, e := range v.Arr {
			parts[i] = e.cpp()
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return ""
}

// ---------- config.cpp emitter ----------

// ToCPP renders the class body (root: pass the parsed root with Name=="") as config.cpp text.
func ToCPP(root *Class) string {
	var sb strings.Builder
	emitBody(&sb, root, 0)
	return sb.String()
}

func indent(sb *strings.Builder, depth int) {
	for i := 0; i < depth; i++ {
		sb.WriteByte('\t')
	}
}

func emitBody(sb *strings.Builder, c *Class, depth int) {
	for _, e := range c.Entries {
		indent(sb, depth)
		switch e.Kind {
		case KindExtern:
			sb.WriteString("class " + e.Name + ";\n")
		case KindDelete:
			sb.WriteString("delete " + e.Name + ";\n")
		case KindClass:
			sb.WriteString("class " + e.Name)
			if e.Class != nil && e.Class.Parent != "" {
				sb.WriteString(": " + e.Class.Parent)
			}
			if e.Class == nil || len(e.Class.Entries) == 0 {
				sb.WriteString("{};\n")
				continue
			}
			sb.WriteString("\n")
			indent(sb, depth)
			sb.WriteString("{\n")
			emitBody(sb, e.Class, depth+1)
			indent(sb, depth)
			sb.WriteString("};\n")
		case KindValue:
			sb.WriteString(e.Name + " = " + e.Value.cpp() + ";\n")
		case KindArray:
			op := "="
			if e.ArrayFlags&1 != 0 {
				op = "+="
			}
			av := &Value{Kind: VArray, Arr: e.Array}
			body := av.cpp()
			if len(body) > 100 {
				// long array: one element per line
				sb.WriteString(e.Name + "[] " + op + "\n")
				indent(sb, depth)
				sb.WriteString("{\n")
				for i, el := range e.Array {
					indent(sb, depth+1)
					sb.WriteString(el.cpp())
					if i != len(e.Array)-1 {
						sb.WriteString(",")
					}
					sb.WriteString("\n")
				}
				indent(sb, depth)
				sb.WriteString("};\n")
			} else {
				sb.WriteString(e.Name + "[] " + op + " " + body + ";\n")
			}
		}
	}
}

// ---------- misc ----------

// CleanString ensures valid UTF-8 (binarized configs may contain cp1252 bytes).
func CleanString(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
		} else {
			sb.WriteRune(rune(c)) // latin-1 fallback
		}
	}
	return sb.String()
}

func (v *Value) GoString() string { return fmt.Sprintf("Value(%s)", v.String()) }
