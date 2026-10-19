// Generates the project-owned geometric icon and native packaging resources.
// No image libraries or build-time downloads are required.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

var le = binary.LittleEndian

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func write(path string, data []byte) { must(os.WriteFile(path, data, 0644)) }
func rounded(x, y, l, t, r, b, rad float64) bool {
	cx := math.Max(l+rad, math.Min(x, r-rad))
	cy := math.Max(t+rad, math.Min(y, b-rad))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= rad*rad
}
func line(x, y, x1, y1, x2, y2, width float64) bool {
	dx, dy := x2-x1, y2-y1
	u := math.Max(0, math.Min(1, ((x-x1)*dx+(y-y1)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-x1-u*dx, y-y1-u*dy) <= width/2
}
func pixel(x, y float64, mono bool) color.NRGBA {
	c := color.NRGBA{}
	if !mono && rounded(x, y, .045, .045, .955, .955, .22) {
		c = color.NRGBA{uint8(17 + 12*y), uint8(30 + 17*y), uint8(49 + 18*y), 255}
	}
	outline := rounded(x, y, .27, .24, .68, .80, .07) && !rounded(x, y, .32, .29, .63, .75, .025)
	clip := rounded(x, y, .38, .19, .57, .31, .035)
	// A break in the clipboard's right edge makes room for the entering arrow.
	if x > .59 && y > .44 && y < .68 {
		outline = false
	}
	if outline {
		c = color.NRGBA{231, 242, 248, 255}
	}
	if clip {
		c = color.NRGBA{64, 222, 189, 255}
	}
	arrow := line(x, y, .47, .56, .83, .56, .058) || line(x, y, .70, .43, .83, .56, .058) || line(x, y, .70, .69, .83, .56, .058)
	if arrow {
		c = color.NRGBA{64, 222, 189, 255}
	}
	if mono && c.A != 0 {
		c = color.NRGBA{0, 0, 0, 255}
	}
	return c
}
func icon(size int, mono bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const samples = 4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var red, green, blue, alpha float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					c := pixel((float64(x)+(float64(sx)+.5)/samples)/float64(size), (float64(y)+(float64(sy)+.5)/samples)/float64(size), mono)
					a := float64(c.A) / 255
					red += float64(c.R) * a
					green += float64(c.G) * a
					blue += float64(c.B) * a
					alpha += a
				}
			}
			if alpha > 0 {
				img.SetNRGBA(x, y, color.NRGBA{uint8(red / alpha), uint8(green / alpha), uint8(blue / alpha), uint8(alpha*255/(samples*samples) + .5)})
			}
		}
	}
	var out bytes.Buffer
	must(png.Encode(&out, img))
	return out.Bytes()
}

type leaf struct {
	id     uint32
	data   []byte
	offset int
}
type branch struct {
	id       uint32
	children []*branch
	leaves   []*leaf
	offset   int
}

// Build a standard three-level PE resource tree: type / name / language.
func resources(images [][]byte, sizes []int) ([]byte, []int) {
	group := make([]byte, 6+14*len(images))
	le.PutUint16(group[2:], 1)
	le.PutUint16(group[4:], uint16(len(images)))
	icons := &branch{id: 3}
	for i, data := range images {
		e := group[6+i*14:]
		e[0] = byte(sizes[i])
		e[1] = byte(sizes[i])
		le.PutUint16(e[4:], 1)
		le.PutUint16(e[6:], 32)
		le.PutUint32(e[8:], uint32(len(data)))
		le.PutUint16(e[12:], uint16(i+1))
		icons.children = append(icons.children, &branch{id: uint32(i + 1), leaves: []*leaf{{id: 1033, data: data}}})
	}
	root := &branch{children: []*branch{icons, {id: 14, children: []*branch{{id: 1, leaves: []*leaf{{id: 1033, data: group}}}}}}}
	var nodes []*branch
	var leaves []*leaf
	offset := 0
	var visit func(*branch)
	visit = func(n *branch) {
		n.offset = offset
		offset += 16 + 8*(len(n.children)+len(n.leaves))
		nodes = append(nodes, n)
		leaves = append(leaves, n.leaves...)
		for _, child := range n.children {
			visit(child)
		}
	}
	visit(root)
	for _, l := range leaves {
		l.offset = offset
		offset += 16
	}
	buf := make([]byte, offset)
	for _, n := range nodes {
		le.PutUint16(buf[n.offset+14:], uint16(len(n.children)+len(n.leaves)))
		p := n.offset + 16
		for _, child := range n.children {
			le.PutUint32(buf[p:], child.id)
			le.PutUint32(buf[p+4:], uint32(child.offset)|0x80000000)
			p += 8
		}
		for _, l := range n.leaves {
			le.PutUint32(buf[p:], l.id)
			le.PutUint32(buf[p+4:], uint32(l.offset))
			p += 8
		}
	}
	var relocs []int
	for _, l := range leaves {
		for len(buf)%4 != 0 {
			buf = append(buf, 0)
		}
		le.PutUint32(buf[l.offset:], uint32(len(buf)))
		le.PutUint32(buf[l.offset+4:], uint32(len(l.data)))
		relocs = append(relocs, l.offset)
		buf = append(buf, l.data...)
	}
	return buf, relocs
}
func coff(data []byte, relocs []int, machine, relocType uint16) []byte {
	const header = 60
	relocStart := header + len(data)
	symbolStart := relocStart + 10*len(relocs)
	buf := make([]byte, symbolStart+18+4)
	le.PutUint16(buf, machine)
	le.PutUint16(buf[2:], 1)
	le.PutUint32(buf[8:], uint32(symbolStart))
	le.PutUint32(buf[12:], 1)
	copy(buf[20:], ".rsrc")
	le.PutUint32(buf[36:], uint32(len(data)))
	le.PutUint32(buf[40:], header)
	le.PutUint32(buf[44:], uint32(relocStart))
	le.PutUint16(buf[52:], uint16(len(relocs)))
	le.PutUint32(buf[56:], 0x40300040)
	copy(buf[header:], data)
	for i, r := range relocs {
		p := relocStart + i*10
		le.PutUint32(buf[p:], uint32(r))
		le.PutUint16(buf[p+8:], relocType)
	}
	copy(buf[symbolStart:], ".rsrc")
	le.PutUint16(buf[symbolStart+12:], 1)
	buf[symbolStart+16] = 3
	le.PutUint32(buf[symbolStart+18:], 4)
	return buf
}
func main() {
	must(os.MkdirAll("assets", 0755))
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		images[i] = icon(size, false)
		write(fmt.Sprintf("assets/icon-%d.png", size), images[i])
	}
	write("assets/icon-1024.png", icon(1024, false))
	write("assets/tray.png", icon(64, true))
	ico := make([]byte, 6+16*len(images))
	le.PutUint16(ico[2:], 1)
	le.PutUint16(ico[4:], uint16(len(images)))
	for i, data := range images {
		p := 6 + 16*i
		ico[p] = byte(sizes[i])
		ico[p+1] = byte(sizes[i])
		le.PutUint16(ico[p+4:], 1)
		le.PutUint16(ico[p+6:], 32)
		le.PutUint32(ico[p+8:], uint32(len(data)))
		le.PutUint32(ico[p+12:], uint32(len(ico)))
		ico = append(ico, data...)
	}
	write("assets/ProxPaste.ico", ico)
	icns := []byte{'i', 'c', 'n', 's', 0, 0, 0, 0}
	for _, entry := range []struct {
		kind string
		size int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}} {
		data := icon(entry.size, false)
		chunk := make([]byte, 8)
		copy(chunk, entry.kind)
		binary.BigEndian.PutUint32(chunk[4:], uint32(8+len(data)))
		icns = append(icns, chunk...)
		icns = append(icns, data...)
	}
	binary.BigEndian.PutUint32(icns[4:], uint32(len(icns)))
	write("assets/ProxPaste.icns", icns)
	data, relocs := resources(images, sizes)
	write("resource_windows_amd64.syso", coff(data, relocs, 0x8664, 3))
	write("resource_windows_arm64.syso", coff(data, relocs, 0xAA64, 2))
}
