package rap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// IsBinarized reports whether data starts with the raP signature.
func IsBinarized(data []byte) bool {
	return len(data) >= 4 && data[0] == 0 && data[1] == 'r' && data[2] == 'a' && data[3] == 'P'
}

// TryDecompressRaP пытается распаковать LZSS-сжатый config.bin
// (так пакует Mikero pboProject: BIS LZSS с ОТНОСИТЕЛЬНЫМИ смещениями).
// Возвращает распакованные данные, если получился валидный raP-конфиг.
func TryDecompressRaP(data []byte) ([]byte, bool) {
	if len(data) < 8 || len(data) > 8<<20 || IsBinarized(data) {
		return nil, false
	}
	out := lzssRelative(data, 64<<20)
	if !IsBinarized(out) {
		return nil, false
	}
	if _, err := ParseBin(out); err != nil {
		return nil, false
	}
	return out, true
}

// lzssRelative — BIS LZSS "Expand": back-reference задаётся расстоянием от
// ТЕКУЩЕЙ позиции выхода (12 бит), длина = (мл. полубайт)+3; ссылки до начала
// выхода дают пробелы. Распаковывает до исчерпания входа (последние байты
// потока — контрольная сумма, лишний хвост парсеру не мешает).
func lzssRelative(src []byte, maxOut int) []byte {
	out := make([]byte, 0, len(src)*3)
	pos := 0
	var flags uint32
	for pos < len(src) && len(out) < maxOut {
		flags >>= 1
		if flags&0x100 == 0 {
			flags = uint32(src[pos]) | 0xff00
			pos++
			if pos >= len(src) {
				break
			}
		}
		if flags&1 != 0 {
			out = append(out, src[pos])
			pos++
		} else {
			if pos+1 >= len(src) {
				break
			}
			b1 := src[pos]
			b2 := src[pos+1]
			pos += 2
			dist := int(b1) | (int(b2&0xf0) << 4)
			ln := int(b2&0x0f) + 3
			rpos := len(out) - dist
			for k := 0; k < ln && len(out) < maxOut; k++ {
				p := rpos + k
				if p < 0 || p >= len(out) {
					out = append(out, 0x20)
				} else {
					out = append(out, out[p])
				}
			}
		}
	}
	return out
}

type binReader struct {
	data []byte
	pos  int
	work *int // общий счётчик работы для защиты от повреждённых/зацикленных конфигов
}

const maxWork = 3_000_000

func (r *binReader) tick() error {
	if r.work == nil {
		return nil
	}
	*r.work++
	if *r.work > maxWork {
		return errors.New("конфиг слишком большой или повреждён (превышен лимит разбора) — возможно, обфускация")
	}
	return nil
}

func (r *binReader) u8() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, errors.New("неожиданный конец config.bin")
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *binReader) u32() (uint32, error) {
	if r.pos+4 > len(r.data) {
		return 0, errors.New("неожиданный конец config.bin")
	}
	v := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return v, nil
}

func (r *binReader) i32() (int32, error) {
	v, err := r.u32()
	return int32(v), err
}

func (r *binReader) f32() (float32, error) {
	v, err := r.u32()
	return math.Float32frombits(v), err
}

func (r *binReader) i64() (int64, error) {
	if r.pos+8 > len(r.data) {
		return 0, errors.New("неожиданный конец config.bin")
	}
	v := binary.LittleEndian.Uint64(r.data[r.pos:])
	r.pos += 8
	return int64(v), nil
}

func (r *binReader) cstring() (string, error) {
	start := r.pos
	for r.pos < len(r.data) {
		if r.data[r.pos] == 0 {
			s := CleanString(r.data[start:r.pos])
			r.pos++
			return s, nil
		}
		r.pos++
		if r.pos-start > 1<<20 {
			return "", errors.New("слишком длинная строка в config.bin")
		}
	}
	return "", errors.New("незавершённая строка в config.bin")
}

// compressed 7-bit varint
func (r *binReader) compressedInt() (int, error) {
	var v uint64
	shift := 0
	for {
		b, err := r.u8()
		if err != nil {
			return 0, err
		}
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift > 35 {
			return 0, errors.New("некорректное сжатое число в config.bin")
		}
	}
	if v > 1<<31 {
		return 0, errors.New("слишком большое число элементов в config.bin")
	}
	return int(v), nil
}

// ParseBin parses a binarized raP config into an AST root class.
// Основной формат: тело класса со смещения 16 (после сигнатуры и трёх u32).
// Некоторые старые/сторонние инструменты пишут raP без смещения enum —
// тогда тело начинается с 12 байта; пробуем этот вариант как запасной.
func ParseBin(data []byte) (*Class, error) {
	if !IsBinarized(data) {
		return nil, errors.New("файл не является бинаризованным конфигом (нет сигнатуры raP)")
	}
	root, err := parseBinAt(data, 16)
	if err == nil {
		return root, nil
	}
	if root2, err2 := parseBinAt(data, 12); err2 == nil && len(root2.Entries) > 0 {
		return root2, nil
	}
	return nil, err
}

func parseBinAt(data []byte, bodyStart int) (root *Class, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("сбой разбора config.bin: %v", rec)
		}
	}()
	if bodyStart >= len(data) {
		return nil, errors.New("config.bin слишком короткий")
	}
	work := 0
	r := &binReader{data: data, pos: bodyStart, work: &work}
	root = &Class{}
	if err := parseBody(r, root, 0); err != nil {
		return nil, err
	}
	return root, nil
}

func parseBody(r *binReader, cls *Class, depth int) error {
	if depth > 100 {
		return errors.New("слишком глубокая вложенность классов в config.bin")
	}
	parent, err := r.cstring()
	if err != nil {
		return err
	}
	cls.Parent = parent
	n, err := r.compressedInt()
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		if err := r.tick(); err != nil {
			return err
		}
		t, err := r.u8()
		if err != nil {
			return err
		}
		switch t {
		case 0: // nested class: name + absolute offset to body
			name, err := r.cstring()
			if err != nil {
				return err
			}
			off, err := r.u32()
			if err != nil {
				return err
			}
			if int(off) >= len(r.data) {
				return fmt.Errorf("класс %s: смещение тела за пределами файла", name)
			}
			child := &Class{Name: name}
			sub := &binReader{data: r.data, pos: int(off), work: r.work}
			if err := parseBody(sub, child, depth+1); err != nil {
				return fmt.Errorf("класс %s: %w", name, err)
			}
			cls.Entries = append(cls.Entries, &Entry{Kind: KindClass, Name: name, Class: child})
		case 1: // value
			sub, err := r.u8()
			if err != nil {
				return err
			}
			name, err := r.cstring()
			if err != nil {
				return err
			}
			v, err := readScalar(r, sub)
			if err != nil {
				return fmt.Errorf("параметр %s: %w", name, err)
			}
			cls.Entries = append(cls.Entries, &Entry{Kind: KindValue, Name: name, Value: v})
		case 2: // array
			name, err := r.cstring()
			if err != nil {
				return err
			}
			arr, err := readArray(r, 0)
			if err != nil {
				return fmt.Errorf("массив %s: %w", name, err)
			}
			cls.Entries = append(cls.Entries, &Entry{Kind: KindArray, Name: name, Array: arr})
		case 3: // extern class
			name, err := r.cstring()
			if err != nil {
				return err
			}
			cls.Entries = append(cls.Entries, &Entry{Kind: KindExtern, Name: name})
		case 4: // delete class
			name, err := r.cstring()
			if err != nil {
				return err
			}
			cls.Entries = append(cls.Entries, &Entry{Kind: KindDelete, Name: name})
		case 5: // array with flags (+=)
			flags, err := r.u32()
			if err != nil {
				return err
			}
			name, err := r.cstring()
			if err != nil {
				return err
			}
			arr, err := readArray(r, 0)
			if err != nil {
				return fmt.Errorf("массив %s: %w", name, err)
			}
			cls.Entries = append(cls.Entries, &Entry{Kind: KindArray, Name: name, Array: arr, ArrayFlags: flags})
		default:
			return fmt.Errorf("неизвестный тип записи %d по смещению %d", t, r.pos-1)
		}
	}
	return nil
}

func readScalar(r *binReader, sub byte) (*Value, error) {
	switch sub {
	case 0: // string
		s, err := r.cstring()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: VString, Str: s}, nil
	case 1: // float
		f, err := r.f32()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: VFloat, F: float64(f)}, nil
	case 2: // long
		v, err := r.i32()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: VInt, I: int64(v)}, nil
	case 3: // int64 (newer format)
		v, err := r.i64()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: VInt, I: v}, nil
	case 4: // variable (raw token)
		s, err := r.cstring()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: VVariable, Str: s}, nil
	}
	return nil, fmt.Errorf("неизвестный тип значения %d", sub)
}

func readArray(r *binReader, depth int) ([]*Value, error) {
	if depth > 50 {
		return nil, errors.New("слишком глубокая вложенность массивов")
	}
	n, err := r.compressedInt()
	if err != nil {
		return nil, err
	}
	out := make([]*Value, 0, n)
	for i := 0; i < n; i++ {
		t, err := r.u8()
		if err != nil {
			return nil, err
		}
		switch t {
		case 0, 1, 2, 4:
			v, err := readScalar(r, t)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		case 3: // nested array
			sub, err := readArray(r, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, &Value{Kind: VArray, Arr: sub})
		default:
			return nil, fmt.Errorf("неизвестный тип элемента массива %d", t)
		}
	}
	return out, nil
}
