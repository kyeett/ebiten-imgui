package renderer

// #include "float.h"
import "C"
import (
	"fmt"
	"image"
	"unsafe"

	"github.com/hajimehoshi/ebiten"
	"github.com/inkyblackness/imgui-go/v2"
)

var Filter ebiten.Filter

// struct ImDrawVert
// {
//     ImVec2  pos; // 2 floats
//     ImVec2  uv; // 2 floats
//     ImU32   col; // uint32
// };

type cVec2x32 struct {
	X float32
	Y float32
}

type cImDrawVertx32 struct {
	Pos cVec2x32
	UV  cVec2x32
	Col uint32
}

type cVec2x64 struct {
	X float64
	Y float64
}

type cImDrawVertx64 struct {
	Pos cVec2x64
	UV  cVec2x64
	Col uint32
}

var szfloat int

func getVertices(vbuf unsafe.Pointer, vblen, vsize, offpos, offuv, offcol int) []ebiten.Vertex {
	if szfloat == 4 {
		return getVerticesx32(vbuf, vblen, vsize, offpos, offuv, offcol)
	}
	if szfloat == 8 {
		return getVerticesx64(vbuf, vblen, vsize, offpos, offuv, offcol)
	}
	panic("invalid char size")
}

func getVerticesx32(vbuf unsafe.Pointer, vblen, vsize, offpos, offuv, offcol int) []ebiten.Vertex {
	n := vblen / vsize
	vertices := make([]ebiten.Vertex, 0, vblen/vsize)
	rawverts := (*[1 << 28]cImDrawVertx32)(vbuf)[:n:n]
	for i := 0; i < n; i++ {
		c0 := rawverts[i].Col
		c00 := uint8(c0 & 0xFF)
		c01 := (c0 >> 8) & 0xFF
		c02 := (c0 >> 16) & 0xFF
		c03 := (c0 >> 24) & 0xFF
		_, _, _, _ = c00, c01, c02, c03
		vertices = append(vertices, ebiten.Vertex{
			SrcX:   rawverts[i].UV.X,
			SrcY:   rawverts[i].UV.Y,
			DstX:   rawverts[i].Pos.X,
			DstY:   rawverts[i].Pos.Y,
			ColorR: float32(rawverts[i].Col&0xFF) / 255,
			ColorG: float32(rawverts[i].Col>>8&0xFF) / 255,
			ColorB: float32(rawverts[i].Col>>16&0xFF) / 255,
			ColorA: float32(rawverts[i].Col>>24&0xFF) / 255,
		})
	}
	return vertices
}

func getVerticesx64(vbuf unsafe.Pointer, vblen, vsize, offpos, offuv, offcol int) []ebiten.Vertex {
	n := vblen / vsize
	vertices := make([]ebiten.Vertex, 0, vblen/vsize)
	rawverts := (*[1 << 28]cImDrawVertx64)(vbuf)[:n:n]
	for i := 0; i < n; i++ {
		vertices = append(vertices, ebiten.Vertex{
			SrcX:   float32(rawverts[i].UV.X),
			SrcY:   float32(rawverts[i].UV.Y),
			DstX:   float32(rawverts[i].Pos.X),
			DstY:   float32(rawverts[i].Pos.Y),
			ColorR: float32(uint8(rawverts[i].Col)) / 255,
			ColorG: float32(uint8(rawverts[i].Col<<8)) / 255,
			ColorB: float32(uint8(rawverts[i].Col<<16)) / 255,
			ColorA: float32(uint8(rawverts[i].Col<<24)) / 255,
		})
	}
	return vertices
}

func lerp(a, b int, t float32) float32 {
	return float32(a)*(1-t) + float32(b)*t
}

func vcopy(v []ebiten.Vertex) []ebiten.Vertex {
	cl := make([]ebiten.Vertex, len(v))
	copy(cl, v)
	return cl
}

func vmultiply(v, vbuf []ebiten.Vertex, bmin, bmax image.Point) {
	for i := range vbuf {
		vbuf[i].SrcX = lerp(bmin.X, bmax.X, v[i].SrcX)
		vbuf[i].SrcY = lerp(bmin.Y, bmax.Y, v[i].SrcY)
	}
}

func getTexture(tex *imgui.RGBA32Image, filter ebiten.Filter) *ebiten.Image {
	n := tex.Width * tex.Height
	pix := (*[1 << 28]uint8)(tex.Pixels)[: n*4 : n*4]
	img, _ := ebiten.NewImage(tex.Width, tex.Height, filter)
	img.ReplacePixels(pix)
	return img
}

func getIndices(ibuf unsafe.Pointer, iblen, isize int) []uint16 {
	n := iblen / isize
	switch isize {
	case 2:
		// direct conversion (without a data copy)
		//TODO: document the size limit (?) this fits 268435456 bytes
		// https://github.com/golang/go/wiki/cgo#turning-c-arrays-into-go-slices
		return (*[1 << 28]uint16)(ibuf)[:n:n]
	case 4:
		slc := make([]uint16, n)
		for i := 0; i < n; i++ {
			slc[i] = uint16(*(*uint32)(unsafe.Pointer(uintptr(ibuf) + uintptr(i*isize))))
		}
		return slc
	case 8:
		slc := make([]uint16, n)
		for i := 0; i < n; i++ {
			slc[i] = uint16(*(*uint64)(unsafe.Pointer(uintptr(ibuf) + uintptr(i*isize))))
		}
		return slc
	default:
		panic(fmt.Sprint("byte size", isize, "not supported"))
	}
	return nil
}

func Render(target *ebiten.Image, displaySize [2]float32, framebufferSize [2]float32, drawData imgui.DrawData, txcache map[imgui.TextureID]*ebiten.Image) {
	if !drawData.Valid() {
		return
	}

	vertexSize, vertexOffsetPos, vertexOffsetUv, vertexOffsetCol := imgui.VertexBufferLayout()
	indexSize := imgui.IndexBufferLayout()

	for _, clist := range drawData.CommandLists() {
		var indexBufferOffset int
		vertexBuffer, vertexLen := clist.VertexBuffer()
		indexBuffer, indexLen := clist.IndexBuffer()
		vertices := getVertices(vertexBuffer, vertexLen, vertexSize, vertexOffsetPos, vertexOffsetUv, vertexOffsetCol)
		vbuf := vcopy(vertices)
		indices := getIndices(indexBuffer, indexLen, indexSize)
		for _, cmd := range clist.Commands() {
			ecount := cmd.ElementCount()
			if cmd.HasUserCallback() {
				cmd.CallUserCallback(clist)
			} else {
				texid := cmd.TextureID()
				if _, ok := txcache[texid]; !ok {
					txcache[texid] = getTexture(imgui.CurrentIO().Fonts().TextureDataRGBA32(), Filter)
				}
				tx := txcache[texid]
				vmultiply(vertices, vbuf, tx.Bounds().Min, tx.Bounds().Max)
				target.DrawTriangles(vbuf, indices[indexBufferOffset:indexBufferOffset+ecount], txcache[texid], &ebiten.DrawTrianglesOptions{})
			}
			indexBufferOffset += ecount
		}
	}
}

func init() {
	szfloat = int(C.SzFloat())
}