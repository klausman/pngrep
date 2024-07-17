// Simple PNG parser. Can be used to discover and extract text chunks.
// Minimal error handling, does not play well with malformed chunks and doesn't
// check chunk CRC32 checksums.
//
// Copyright 2023 Tobias Klausmann
// Licensed under the GPLv3, see COPYING for details
//

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"slices"
)

// From https://www.w3.org/TR/png/#5PNG-file-signature:
// ```
// The first eight bytes of a PNG datastream always contain the following
// (decimal) values:
//
// 137 80 78 71 13 10 26 10
//
// which are (in hexadecimal):
//
// 89 50 4E 47 0D 0A 1A 0A
//
// This signature indicates that the remainder of the datastream contains a
// single PNG image, consisting of a series of chunks beginning with an IHDR
// chunk and ending with an IEND chunk.
// ```
const (
	PNGMagic   = "\x89\x50\x4E\x47\x0D\x0A\x1A\x0A"
	iHDRlength = 13
)

var ct2bd map[int][]int

func init() {
	ct2bd = make(map[int][]int)
	ct2bd[0] = []int{1, 2, 4, 8, 16}
	ct2bd[2] = []int{8, 16}
	ct2bd[3] = []int{1, 2, 4, 8}
	ct2bd[4] = ct2bd[2]
	ct2bd[6] = ct2bd[2]
}

// PNG represent a PNG file, including metadata and (compressed) image data
type PNG struct {
	Width       int
	Height      int
	Depth       int
	ColorType   int
	Compression int
	Filter      int
	Interlace   int
	Chunks      []*Chunk
	NumCHunks   int
}

// Chunk is a PNG file chunk, including its CRC32 checksum
type Chunk struct {
	Len      int
	Type     string
	Data     []byte
	Checksum []byte
}

// Load reads from an io.Reader and returns a PNG struct
func Load(r io.Reader) (PNG, error) {
	var png PNG
	var err error
	// Read first 8 bytes == PNG header.
	header := make([]byte, 8)
	// Read CRC32 hash
	if _, err = io.ReadFull(r, header); err != nil {
		return png, err
	}
	if string(header) != PNGMagic {
		return png, fmt.Errorf("wrong PNG header. Got %x - Expected %x",
			header, PNGMagic)
	}

	for err == nil {
		var c Chunk
		err = (&c).Fill(r)
		// Drop the last empty chunk.
		if c.Type != "" {
			png.Chunks = append(png.Chunks, &c)
		}
	}

	if err := (&png).Fill(); err != nil {
		return png, err
	}
	return png, nil
}

// Fill will read bytes from the reader and fill in the chunk
func (c *Chunk) Fill(r io.Reader) error {
	var err error

	// Length of the chunk, 4 bytes
	buf := make([]byte, 4)
	err = fillRead(&buf, r)
	if err != nil {
		return err
	}
	c.Len = int(binary.BigEndian.Uint32(buf))

	// Type, 4 ASCII bytes
	buf = make([]byte, 4)
	err = fillRead(&buf, r)
	if err != nil {
		return err
	}
	c.Type = string(buf)

	// Data
	// We use a separate buffer for this data since it's used wholesale in our
	// own data structure, instead of being copy-converted.
	tmp := make([]byte, c.Len)
	err = fillRead(&tmp, r)
	if err != nil {
		return err
	}
	c.Data = tmp

	// CRC32
	buf = make([]byte, 4)
	err = fillRead(&buf, r)
	if err != nil {
		return err
	}
	c.Checksum = buf

	// TODO: report CRC32 checksum errors
	return nil
}

// IHDR Parsing
// Inspired by/lifted from https://golang.org/src/image/png/reader.go
func (png *PNG) parseIHDR(iHDR *Chunk) error {
	if iHDR.Len != iHDRlength {
		return fmt.Errorf("invalid IHDR length: got %d - expected %d",
			iHDR.Len, iHDRlength)
	}

	// https://www.w3.org/TR/png/#11IHDR
	// Width:              4 bytes (big endian)
	// Height:             4 bytes (big endian)
	// Bit depth:          1 byte
	// Color type:         1 byte
	// Compression method: 1 byte
	// Filter method:      1 byte
	// Interlace method:   1 byte

	tmp := iHDR.Data

	// From https://www.w3.org/TR/png/#11IHDR
	// ```
	// Width and height give the image dimensions in pixels. They are PNG
	// four-byte unsigned integers. Zero is an invalid value.
	// ```
	// and from https://www.w3.org/TR/png/#dfn-png-four-byte-unsigned-integer:
	// ```
	// a four-byte unsigned integer limited to the range 0 to 231-1.
	// NOTE
	// The restriction is imposed in order to accommodate languages that have
	// difficulty with unsigned four-byte values.
	// ``
	png.Width = int(binary.BigEndian.Uint32(tmp[0:4]))
	if png.Width == 0 || png.Width > 2<<30 {
		return fmt.Errorf("invalid width in iHDR expected 0 < w < 2^31, got: %d", png.Width)
	}

	png.Height = int(binary.BigEndian.Uint32(tmp[4:8]))
	if png.Height == 0 || png.Height > 2<<30 {
		return fmt.Errorf("invalid height in iHDR expected 0 < h < 2^31, got: %d", png.Height)
	}

	// From https://www.w3.org/TR/png/#11IHDR:
	// ```
	// Bit depth is a single-byte integer giving the number of bits per sample
	// or per palette index (not per pixel). Valid values are 1, 2, 4, 8, and
	// 16, although not all values are allowed for all colour types. See 6.1
	// Colour types and values.
	//
	// Colour type is a single-byte integer.
	//
	// Bit depth restrictions for each colour type are imposed to simplify
	// implementations and to prohibit combinations that do not compress well.
	// The allowed combinations are defined in Table 13.
	//
	// Table 13 Allowed combinations of colour type and bit depth (abridged)
	//
	// PNG image type     Colour Allowed
	// type               type   bit depths
	// Greyscale	      0	     1,2,4,8,16
	// Truecolour	      2	     8,16
	// Indexed            3	     1,2,4,8
	// Greyscale w/alpha  4      8,16
	// Truecolour w/alpha 6      8,16
	// ```
	png.Depth = int(tmp[8])
	png.ColorType = int(tmp[9])
	allowedct, ok := ct2bd[png.ColorType]
	if !ok {
		return fmt.Errorf("image with invalid color type - expected one of [0,2,3,4,6], got %d", png.ColorType)
	}
	if !slices.Contains(allowedct, png.Depth) {
		return fmt.Errorf("image with color type %d and wrong depth - expected one of %v, got %d", png.ColorType, allowedct, png.Depth)
	}

	// From https://www.w3.org/TR/png/#11IHDR
	// ```
	// Compression method is a single-byte integer that indicates the method
	// used to compress the image data. Only compression method 0 (deflate
	// compression with a sliding window of at most 32768 bytes) is defined in
	// this specification. All conforming PNG images shall be compressed with
	// this scheme.
	// ```
	if int(tmp[10]) != 0 {
		return fmt.Errorf("invalid compression method - expected 0 - got %x", tmp[10])
	}
	png.Compression = int(tmp[10])

	// From https://www.w3.org/TR/png/#11IHDR
	// ```
	// Filter method is a single-byte integer that indicates the preprocessing
	// method applied to the image data before compression. Only filter method
	// 0 (adaptive filtering with five basic filter types) is defined in this
	// specification.
	// ```
	if int(tmp[11]) != 0 {
		return fmt.Errorf("invalid filter method - expected 0 - got %x", tmp[11])
	}
	png.Filter = int(tmp[11])

	// From https://www.w3.org/TR/png/#11IHDR
	// ```
	// Interlace method is a single-byte integer that indicates the
	// transmission order of the image data. Two values are defined in this
	// specification: 0 (no interlace) or 1 (Adam7 interlace).
	// ```
	if int(tmp[12]) != 0 && int(tmp[12]) != 1 {
		return fmt.Errorf("invalid interlace method - expected 0 or 1 - got %x", tmp[12])
	}
	png.Interlace = int(tmp[12])

	return nil
}

// Fill populates the PNG header fields and the number of chunks
func (png *PNG) Fill() error {
	if err := png.parseIHDR(png.Chunks[0]); err != nil {
		return err
	}
	png.NumCHunks = len(png.Chunks)
	return nil
}

// GetTextChunks examines the chunks of a PNG image and returns the ones of type tEXt
func (png PNG) GetTextChunks() []string {
	var chunks []string
	for _, c := range png.Chunks {
		if c.Type == "tEXt" {
			chunks = append(chunks, string(c.Data))
		}
	}
	return chunks
}

func fillRead(buf *[]byte, r io.Reader) error {
	expected := len(*buf)
	n, err := io.ReadFull(r, *buf)
	if n != expected {
		return fmt.Errorf("short read - expected %d, got %d", expected, n)
	}
	return err
}
