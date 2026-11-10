// Package pbo reads and writes Bohemia Interactive PBO archives (DayZ / Arma).
package pbo

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	MethodUncompressed = 0x0
	MethodPacked       = 0x43707273 // "Cprs" — LZSS compressed
	MethodVers         = 0x56657273 // "Vers" — properties (header) entry
)

// Entry is a single file record inside a PBO.
type Entry struct {
	Name         string
	Method       uint32
	OriginalSize uint32
	Reserved     uint32
	Timestamp    uint32
	DataSize     uint32
	Offset       int64 // absolute offset of the data block within the PBO file
}

// PBO is a parsed archive header plus a handle to read entry data lazily.
type PBO struct {
	Path       string
	Properties [][2]string
	Entries    []Entry
	ra         io.ReaderAt
	size       int64
	closer     io.Closer
}

// Prefix returns the "prefix" header property (mod path prefix), or "".
func (p *PBO) Prefix() string {
	for _, kv := range p.Properties {
		if strings.EqualFold(kv[0], "prefix") {
			return kv[1]
		}
	}
	return ""
}

func (p *PBO) Close() error {
	if p.closer != nil {
		return p.closer.Close()
	}
	return nil
}

// Open parses the PBO header from a file on disk (data is read lazily).
func Open(path string) (*PBO, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	p, err := parse(f, st.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	p.Path = path
	p.closer = f
	return p, nil
}

// FromBytes parses a PBO fully contained in memory.
func FromBytes(data []byte) (*PBO, error) {
	return parse(bytes.NewReader(data), int64(len(data)))
}

const maxNameLen = 1024

func readCString(r *bufio, limit int) (string, error) {
	var sb []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			return string(sb), nil
		}
		if b < 0x20 {
			return "", errors.New("в заголовке PBO повреждённые имена файлов — вероятно, PBO зашифрован или обфусцирован")
		}
		sb = append(sb, b)
		if len(sb) > limit {
			return "", errors.New("слишком длинная строка в заголовке (файл повреждён или обфусцирован)")
		}
	}
}

// bufio is a tiny counting buffered reader over an io.ReaderAt.
type bufio struct {
	ra   io.ReaderAt
	off  int64
	size int64
	buf  []byte
	bpos int
	blen int
}

func newBufio(ra io.ReaderAt, size int64) *bufio {
	return &bufio{ra: ra, size: size, buf: make([]byte, 64*1024)}
}

func (b *bufio) ReadByte() (byte, error) {
	if b.bpos >= b.blen {
		if b.off >= b.size {
			return 0, io.ErrUnexpectedEOF
		}
		n, err := b.ra.ReadAt(b.buf, b.off)
		if n == 0 {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		b.blen = n
		b.bpos = 0
	}
	c := b.buf[b.bpos]
	b.bpos++
	b.off++
	return c, nil
}

func (b *bufio) ReadUint32() (uint32, error) {
	var v [4]byte
	for i := 0; i < 4; i++ {
		c, err := b.ReadByte()
		if err != nil {
			return 0, err
		}
		v[i] = c
	}
	return binary.LittleEndian.Uint32(v[:]), nil
}

func parse(ra io.ReaderAt, size int64) (*PBO, error) {
	r := newBufio(ra, size)
	p := &PBO{ra: ra, size: size}
	sawVers := false
	for {
		name, err := readCString(r, maxNameLen)
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения заголовка PBO: %w", err)
		}
		method, err := r.ReadUint32()
		if err != nil {
			return nil, err
		}
		osize, _ := r.ReadUint32()
		res, _ := r.ReadUint32()
		ts, _ := r.ReadUint32()
		dsize, err := r.ReadUint32()
		if err != nil {
			return nil, err
		}
		if name == "" && method == MethodVers {
			if sawVers {
				// second properties block — tolerate
			}
			sawVers = true
			for {
				k, err := readCString(r, maxNameLen)
				if err != nil {
					return nil, err
				}
				if k == "" {
					break
				}
				v, err := readCString(r, 4096)
				if err != nil {
					return nil, err
				}
				p.Properties = append(p.Properties, [2]string{k, v})
			}
			continue
		}
		if name == "" {
			break // header terminator
		}
		p.Entries = append(p.Entries, Entry{
			Name: name, Method: method, OriginalSize: osize,
			Reserved: res, Timestamp: ts, DataSize: dsize,
		})
		if len(p.Entries) > 200000 {
			return nil, errors.New("слишком много записей в PBO (файл повреждён или обфусцирован)")
		}
	}
	// data blocks follow sequentially
	off := r.off
	for i := range p.Entries {
		p.Entries[i].Offset = off
		off += int64(p.Entries[i].DataSize)
	}
	if off > size {
		return nil, errors.New("данные PBO выходят за пределы файла (файл повреждён или обфусцирован)")
	}
	return p, nil
}

// ReadEntry returns the decompressed content of an entry.
func (p *PBO) ReadEntry(e *Entry) ([]byte, error) {
	raw := make([]byte, e.DataSize)
	if e.DataSize > 0 {
		if _, err := p.ra.ReadAt(raw, e.Offset); err != nil {
			return nil, err
		}
	}
	if e.Method == MethodPacked && e.OriginalSize > 0 && e.OriginalSize != e.DataSize {
		out, err := lzssDecompress(raw, int(e.OriginalSize))
		if err != nil {
			return nil, fmt.Errorf("ошибка LZSS-распаковки %s: %w", e.Name, err)
		}
		return out, nil
	}
	return raw, nil
}

// lzssDecompress implements BI LZSS ("Expand"): back-references are
// RELATIVE to the current output position (dist = 12 bit, len = low nibble + 3),
// references before the start of output produce spaces (0x20).
func lzssDecompress(src []byte, expected int) ([]byte, error) {
	out, err := LzssExpand(src, expected)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LzssExpand распаковывает BIS LZSS до expected байт (expected <= 0 — до
// исчерпания входа, но не более 64 МБ).
func LzssExpand(src []byte, expected int) ([]byte, error) {
	limit := expected
	if limit <= 0 {
		limit = 64 << 20
	}
	out := make([]byte, 0, min(limit, 1<<20))
	pos := 0
	var flags uint32
	for len(out) < limit && pos < len(src) {
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
			for k := 0; k < ln && len(out) < limit; k++ {
				p := rpos + k
				if p < 0 {
					out = append(out, 0x20)
				} else if p >= len(out) {
					out = append(out, 0x20)
				} else {
					out = append(out, out[p])
				}
			}
		}
	}
	if expected > 0 && len(out) < expected {
		return nil, io.ErrUnexpectedEOF
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// File is an input for Build.
type File struct {
	Name string
	Data []byte
}

// Build creates a complete (uncompressed) PBO archive in memory with the
// given prefix property and files, including the trailing zero + SHA1.
func Build(prefix string, files []File) []byte {
	var buf bytes.Buffer
	writeC := func(s string) {
		buf.WriteString(s)
		buf.WriteByte(0)
	}
	writeU32 := func(v uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		buf.Write(b[:])
	}
	ts := uint32(time.Now().Unix())
	// properties entry
	writeC("")
	writeU32(MethodVers)
	writeU32(0)
	writeU32(0)
	writeU32(0)
	writeU32(0)
	writeC("prefix")
	writeC(prefix)
	writeC("")
	// file entries
	for _, f := range files {
		writeC(f.Name)
		writeU32(MethodUncompressed)
		writeU32(0)
		writeU32(0)
		writeU32(ts)
		writeU32(uint32(len(f.Data)))
	}
	// terminator
	writeC("")
	writeU32(0)
	writeU32(0)
	writeU32(0)
	writeU32(0)
	writeU32(0)
	// data
	for _, f := range files {
		buf.Write(f.Data)
	}
	// trailing zero + sha1 of everything before it
	sum := sha1.Sum(buf.Bytes())
	buf.WriteByte(0)
	buf.Write(sum[:])
	return buf.Bytes()
}
