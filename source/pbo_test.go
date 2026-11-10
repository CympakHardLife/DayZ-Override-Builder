package pbo

import (
	"bytes"
	"testing"
)

// literals only: flag byte 0xFF -> 8 literal bytes
func TestLZSSLiterals(t *testing.T) {
	src := []byte{0xFF, 'D', 'a', 'y', 'Z', ' ', 'm', 'o', 'd', 0x03, 'x', 'y'}
	out, err := lzssDecompress(src, 10)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "DayZ modxy" {
		t.Fatalf("got %q", out)
	}
}

// literal 'A', then overlapping back-reference (dist=1, len=18), then literal 'B'
func TestLZSSMatch(t *testing.T) {
	src := []byte{0x05, 'A', 0x01, 0x0F, 'B'}
	out, err := lzssDecompress(src, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Repeat([]byte{'A'}, 19), 'B')
	if !bytes.Equal(out, want) {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// ссылка до начала выхода даёт пробелы (BIS "Expand")
func TestLZSSSpaceFill(t *testing.T) {
	// backref dist=5 len=3 при пустом выходе -> "   ", затем литерал 'X'
	src := []byte{0x02, 0x05, 0x00, 'X'}
	out, err := lzssDecompress(src, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte("   X")) {
		t.Fatalf("got %q", out)
	}
}

// round-trip: Build -> FromBytes
func TestBuildRoundTrip(t *testing.T) {
	data := Build("TestPrefix", []File{{Name: "config.cpp", Data: []byte("class CfgPatches {};")}})
	p, err := FromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Prefix() != "TestPrefix" {
		t.Fatalf("prefix: %q", p.Prefix())
	}
	if len(p.Entries) != 1 || p.Entries[0].Name != "config.cpp" {
		t.Fatalf("entries: %+v", p.Entries)
	}
	got, err := p.ReadEntry(&p.Entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "class CfgPatches {};" {
		t.Fatalf("content: %q", got)
	}
}
