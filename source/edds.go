// Package edds decodes DayZ .edds textures (DDS header + COPY/LZ4 chunked
// mipmaps, DXT1/DXT5/BGRA8) to Go images, and encodes PNG back to an
// uncompressed BGRA8 .edds with a full mip chain.
package edds

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
)

const (
	ddsMagic  = 0x20534444 // "DDS "
	fccDXT1   = 0x31545844
	fccDXT5   = 0x35545844
	fccDX10   = 0x30315844
	chunkCopy = 0x59504F43 // "COPY"
	chunkLZ4  = 0x20345A4C // "LZ4 "
)

type format int

const (
	fmtBGRA8 format = iota
	fmtRGBA8
	fmtDXT1
	fmtDXT5
)

type header struct {
	width, height int
	mipCount      int
	fmt           format
	dataStart     int
}

func u32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

func parseHeader(data []byte) (*header, error) {
	if len(data) < 128 || u32(data, 0) != ddsMagic {
		return nil, errors.New("не DDS/EDDS файл")
	}
	h := &header{
		height:    int(u32(data, 12)),
		width:     int(u32(data, 16)),
		mipCount:  int(u32(data, 28)),
		dataStart: 128,
	}
	if h.mipCount == 0 {
		h.mipCount = 1
	}
	pfFlags := u32(data, 80)
	fourcc := u32(data, 84)
	bitCount := u32(data, 88)
	rMask := u32(data, 92)
	switch {
	case pfFlags&0x4 != 0 && fourcc == fccDXT1:
		h.fmt = fmtDXT1
	case pfFlags&0x4 != 0 && fourcc == fccDXT5:
		h.fmt = fmtDXT5
	case pfFlags&0x4 != 0 && fourcc == fccDX10:
		if len(data) < 148 {
			return nil, errors.New("обрезанный DX10 заголовок")
		}
		dxgi := u32(data, 128)
		h.dataStart = 148
		switch dxgi {
		case 70, 71, 72: // BC1
			h.fmt = fmtDXT1
		case 76, 77, 78: // BC3
			h.fmt = fmtDXT5
		case 87, 88, 91: // B8G8R8A8
			h.fmt = fmtBGRA8
		case 28, 29, 30: // R8G8B8A8
			h.fmt = fmtRGBA8
		default:
			return nil, fmt.Errorf("неподдерживаемый DXGI формат %d", dxgi)
		}
	case bitCount == 32 && rMask == 0x00FF0000:
		h.fmt = fmtBGRA8
	case bitCount == 32 && rMask == 0x000000FF:
		h.fmt = fmtRGBA8
	default:
		return nil, fmt.Errorf("неподдерживаемый формат пикселей (fourCC=%08x, bits=%d)", fourcc, bitCount)
	}
	if h.width <= 0 || h.height <= 0 || h.width > 16384 || h.height > 16384 {
		return nil, errors.New("некорректные размеры текстуры")
	}
	return h, nil
}

func mipSize(f format, w, h int) int {
	switch f {
	case fmtDXT1:
		return ((w + 3) / 4) * ((h + 3) / 4) * 8
	case fmtDXT5:
		return ((w + 3) / 4) * ((h + 3) / 4) * 16
	default:
		return w * h * 4
	}
}

// lz4Append decodes a raw LZ4 block APPENDING to dst: смещения могут
// указывать в ранее распакованные данные (зависимые блоки, сквозное окно).
func lz4Append(dst []byte, src []byte, maxTotal int) ([]byte, error) {
	i := 0
	for i < len(src) {
		token := src[i]
		i++
		litLen := int(token >> 4)
		if litLen == 15 {
			for i < len(src) {
				b := src[i]
				i++
				litLen += int(b)
				if b != 255 {
					break
				}
			}
		}
		if i+litLen > len(src) {
			return nil, errors.New("LZ4: обрыв литералов")
		}
		dst = append(dst, src[i:i+litLen]...)
		i += litLen
		if i >= len(src) {
			break // конец блока
		}
		if i+2 > len(src) {
			return nil, errors.New("LZ4: обрыв смещения")
		}
		offset := int(binary.LittleEndian.Uint16(src[i:]))
		i += 2
		if offset == 0 || offset > len(dst) {
			return nil, errors.New("LZ4: некорректное смещение")
		}
		matchLen := int(token&0xF) + 4
		if token&0xF == 15 {
			for i < len(src) {
				b := src[i]
				i++
				matchLen += int(b)
				if b != 255 {
					break
				}
			}
		}
		pos := len(dst) - offset
		for k := 0; k < matchLen; k++ {
			dst = append(dst, dst[pos+k])
		}
		if len(dst) > maxTotal {
			return nil, errors.New("LZ4: переполнение")
		}
	}
	return dst, nil
}

// В EDDS после DDS-заголовка идёт ТАБЛИЦА заголовков чанков
// ([kind u32][size u32] на каждый блок, мипы от меньшего к большему),
// а затем подряд все данные блоков в том же порядке. Крупные мипы могут
// быть разбиты на несколько блоков, поэтому число записей ищем по тому,
// при каком n конец таблицы + сумма размеров совпадает с концом файла.
func chunkTable(data []byte, start int) (kinds []uint32, sizes []int, payloadStart int, err error) {
	sum := 0
	for n := 1; n <= 4096; n++ {
		pos := start + (n-1)*8
		if pos+8 > len(data) {
			return nil, nil, 0, errors.New("не удалось разобрать таблицу чанков текстуры")
		}
		kind := u32(data, pos)
		size := int(u32(data, pos+4))
		if (kind != chunkCopy && kind != chunkLZ4) || size < 0 || size > len(data) {
			return nil, nil, 0, fmt.Errorf("некорректный чанк текстуры (запись %d)", n)
		}
		kinds = append(kinds, kind)
		sizes = append(sizes, size)
		sum += size
		end := start + n*8 + sum
		if end == len(data) {
			return kinds, sizes, start + n*8, nil
		}
		if end > len(data) {
			return nil, nil, 0, errors.New("несогласованные размеры чанков текстуры")
		}
	}
	return nil, nil, 0, errors.New("слишком много чанков текстуры")
}

// largestMip decodes the whole chunk stream (сквозное LZ4-окно между
// блоками) and returns the largest mip's data.
func largestMip(data []byte, h *header) ([]byte, error) {
	kinds, sizes, pos, err := chunkTable(data, h.dataStart)
	if err != nil {
		return nil, err
	}
	total := 0
	largest := 0
	for level := h.mipCount - 1; level >= 0; level-- {
		w := h.width >> level
		hh := h.height >> level
		if w < 1 {
			w = 1
		}
		if hh < 1 {
			hh = 1
		}
		largest = mipSize(h.fmt, w, hh)
		total += largest
	}
	all := make([]byte, 0, total)
	for i := range kinds {
		chunk := data[pos : pos+sizes[i]]
		pos += sizes[i]
		switch kinds[i] {
		case chunkCopy:
			all = append(all, chunk...)
		case chunkLZ4:
			all, err = lz4Append(all, chunk, total)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(all) < largest {
		return nil, errors.New("данные текстуры неполные")
	}
	return all[len(all)-largest:], nil
}

// Decode returns the largest mip as an image.
func Decode(data []byte) (image.Image, error) {
	h, err := parseHeader(data)
	if err != nil {
		return nil, err
	}
	mip, err := largestMip(data, h)
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, h.width, h.height))
	switch h.fmt {
	case fmtBGRA8:
		for i := 0; i+3 < len(mip) && i/4 < h.width*h.height; i += 4 {
			img.Pix[i] = mip[i+2]
			img.Pix[i+1] = mip[i+1]
			img.Pix[i+2] = mip[i]
			img.Pix[i+3] = mip[i+3]
		}
	case fmtRGBA8:
		copy(img.Pix, mip)
	case fmtDXT1:
		decodeDXT(img, mip, h.width, h.height, false)
	case fmtDXT5:
		decodeDXT(img, mip, h.width, h.height, true)
	}
	return img, nil
}

func c565(v uint16) (r, g, b uint8) {
	r = uint8((v >> 11) & 0x1F)
	g = uint8((v >> 5) & 0x3F)
	b = uint8(v & 0x1F)
	return r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2
}

func decodeDXT(img *image.RGBA, data []byte, w, h int, dxt5 bool) {
	blockBytes := 8
	if dxt5 {
		blockBytes = 16
	}
	bw := (w + 3) / 4
	bh := (h + 3) / 4
	for by := 0; by < bh; by++ {
		for bx := 0; bx < bw; bx++ {
			off := (by*bw + bx) * blockBytes
			if off+blockBytes > len(data) {
				return
			}
			block := data[off : off+blockBytes]
			var alphas [16]uint8
			colorBlock := block
			if dxt5 {
				a0, a1 := block[0], block[1]
				var pal [8]uint8
				pal[0], pal[1] = a0, a1
				if a0 > a1 {
					for i := 1; i <= 6; i++ {
						pal[1+i] = uint8(((7-i)*int(a0) + i*int(a1)) / 7)
					}
				} else {
					for i := 1; i <= 4; i++ {
						pal[1+i] = uint8(((5-i)*int(a0) + i*int(a1)) / 5)
					}
					pal[6], pal[7] = 0, 255
				}
				bits := uint64(0)
				for i := 5; i >= 0; i-- {
					bits = bits<<8 | uint64(block[2+i])
				}
				for i := 0; i < 16; i++ {
					alphas[i] = pal[(bits>>(3*i))&0x7]
				}
				colorBlock = block[8:]
			} else {
				for i := range alphas {
					alphas[i] = 255
				}
			}
			cv0 := binary.LittleEndian.Uint16(colorBlock[0:])
			cv1 := binary.LittleEndian.Uint16(colorBlock[2:])
			r0, g0, b0 := c565(cv0)
			r1, g1, b1 := c565(cv1)
			var pal [4]color.RGBA
			pal[0] = color.RGBA{r0, g0, b0, 255}
			pal[1] = color.RGBA{r1, g1, b1, 255}
			if cv0 > cv1 || dxt5 {
				pal[2] = color.RGBA{uint8((2*int(r0) + int(r1)) / 3), uint8((2*int(g0) + int(g1)) / 3), uint8((2*int(b0) + int(b1)) / 3), 255}
				pal[3] = color.RGBA{uint8((int(r0) + 2*int(r1)) / 3), uint8((int(g0) + 2*int(g1)) / 3), uint8((int(b0) + 2*int(b1)) / 3), 255}
			} else {
				pal[2] = color.RGBA{uint8((int(r0) + int(r1)) / 2), uint8((int(g0) + int(g1)) / 2), uint8((int(b0) + int(b1)) / 2), 255}
				pal[3] = color.RGBA{0, 0, 0, 0}
			}
			idx := binary.LittleEndian.Uint32(colorBlock[4:])
			for py := 0; py < 4; py++ {
				for px := 0; px < 4; px++ {
					x := bx*4 + px
					y := by*4 + py
					if x >= w || y >= h {
						continue
					}
					c := pal[(idx>>(2*(py*4+px)))&0x3]
					c.A = alphas[py*4+px]
					o := img.PixOffset(x, y)
					img.Pix[o] = c.R
					img.Pix[o+1] = c.G
					img.Pix[o+2] = c.B
					img.Pix[o+3] = c.A
				}
			}
		}
	}
}

// ---------- encoding PNG -> EDDS (uncompressed BGRA8 + mip chain) ----------

func downscale(src *image.RGBA, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw := src.Rect.Dx()
	sh := src.Rect.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// box-фильтр по соответствующей области
			x0, x1 := x*sw/w, (x+1)*sw/w
			y0, y1 := y*sh/h, (y+1)*sh/h
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			var r, g, b, a, n int
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					o := src.PixOffset(sx, sy)
					r += int(src.Pix[o])
					g += int(src.Pix[o+1])
					b += int(src.Pix[o+2])
					a += int(src.Pix[o+3])
					n++
				}
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o] = uint8(r / n)
			dst.Pix[o+1] = uint8(g / n)
			dst.Pix[o+2] = uint8(b / n)
			dst.Pix[o+3] = uint8(a / n)
		}
	}
	return dst
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x-b.Min.X, y-b.Min.Y, img.At(x, y))
		}
	}
	return out
}

// Encode converts an image to uncompressed BGRA8 .edds with full mip chain.
func Encode(img image.Image) []byte {
	src := toRGBA(img)
	w := src.Rect.Dx()
	h := src.Rect.Dy()
	mips := 1
	for m := max(w, h); m > 1; m >>= 1 {
		mips++
	}
	head := make([]byte, 128)
	binary.LittleEndian.PutUint32(head[0:], ddsMagic)
	binary.LittleEndian.PutUint32(head[4:], 124)
	binary.LittleEndian.PutUint32(head[8:], 0x0002100F) // CAPS|HEIGHT|WIDTH|PITCH|PIXELFORMAT|MIPMAPCOUNT
	binary.LittleEndian.PutUint32(head[12:], uint32(h))
	binary.LittleEndian.PutUint32(head[16:], uint32(w))
	binary.LittleEndian.PutUint32(head[20:], uint32(w*4))
	binary.LittleEndian.PutUint32(head[28:], uint32(mips))
	binary.LittleEndian.PutUint32(head[76:], 32)
	binary.LittleEndian.PutUint32(head[80:], 0x41) // RGB|ALPHAPIXELS
	binary.LittleEndian.PutUint32(head[88:], 32)
	binary.LittleEndian.PutUint32(head[92:], 0x00FF0000)
	binary.LittleEndian.PutUint32(head[96:], 0x0000FF00)
	binary.LittleEndian.PutUint32(head[100:], 0x000000FF)
	binary.LittleEndian.PutUint32(head[104:], 0xFF000000)
	binary.LittleEndian.PutUint32(head[108:], 0x00401008) // COMPLEX|TEXTURE|MIPMAP
	out := head

	// уровни от МЕНЬШЕГО к БОЛЬШЕМУ (как в оригинальных edds)
	levels := make([]*image.RGBA, mips)
	levels[0] = src
	for i := 1; i < mips; i++ {
		lw := w >> i
		lh := h >> i
		if lw < 1 {
			lw = 1
		}
		if lh < 1 {
			lh = 1
		}
		levels[i] = downscale(src, lw, lh)
	}
	// формат EDDS: сначала ТАБЛИЦА заголовков чанков, затем данные подряд
	var payloads [][]byte
	for i := mips - 1; i >= 0; i-- {
		l := levels[i]
		lw := l.Rect.Dx()
		lh := l.Rect.Dy()
		payload := make([]byte, lw*lh*4)
		for p := 0; p < lw*lh; p++ {
			payload[p*4] = l.Pix[p*4+2]   // B
			payload[p*4+1] = l.Pix[p*4+1] // G
			payload[p*4+2] = l.Pix[p*4]   // R
			payload[p*4+3] = l.Pix[p*4+3] // A
		}
		payloads = append(payloads, payload)
	}
	for _, p := range payloads {
		ch := make([]byte, 8)
		binary.LittleEndian.PutUint32(ch, chunkCopy)
		binary.LittleEndian.PutUint32(ch[4:], uint32(len(p)))
		out = append(out, ch...)
	}
	for _, p := range payloads {
		out = append(out, p...)
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
