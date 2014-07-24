package zim

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	xz "github.com/remyoudompheng/go-liblzma"
)

const (
	RedirectEntry   uint16 = 0xffff
	LinkTargetEntry        = 0xfffe
	DeletedEntry           = 0xfffd
)

type Article struct {
	URLPtr     uint64
	Mimetype   uint16
	Namespace  byte
	URL        string
	Title      string
	Blob       uint32
	Cluster    uint32
	RedirectTo *Article
}

// get the article (Directory) pointed by the offset found in URLpos or Titlepos
func (z *ZimReader) getArticleAt(offset uint64) *Article {
	a := new(Article)
	a.URLPtr = offset

	mimeIdx, err := readInt16(z.mmap[offset : offset+2])
	if err != nil {
		panic(err)
	}
	a.Mimetype = mimeIdx

	// Linktarget or Target Entry
	if mimeIdx == LinkTargetEntry || mimeIdx == DeletedEntry {
		//TODO
		return nil
	}

	a.Namespace = z.mmap[offset+3]

	a.Cluster, err = readInt32(z.mmap[offset+8 : offset+8+4])
	if err != nil {
		panic(err)
	}

	a.Blob, err = readInt32(z.mmap[offset+12 : offset+12+4])
	if err != nil {
		panic(err)
	}

	// Redirect
	if mimeIdx == RedirectEntry {
		// check for a possible loop: the redirect could point to the same target
		if z.GetUrlOffsetAtIdx(a.Cluster) != offset {
			// redirect ptr share the same memory offset than Cluster number
			a.RedirectTo = z.getArticleAt(z.GetUrlOffsetAtIdx(a.Cluster))
		}
	}

	b := bytes.NewBuffer(z.mmap[offset+16:])
	a.URL, err = b.ReadString('\x00')
	if err != nil {
		panic(err)
	}
	a.URL = strings.TrimRight(string(a.URL), "\x00")

	a.Title, err = b.ReadString('\x00')
	if err != nil {
		panic(err)
	}
	a.Title = strings.TrimRight(string(a.Title), "\x00")

	return a
}

// return the uncompressed data associated with this article
func (a *Article) Data(z *ZimReader) []byte {
	start, end := z.getClusterOffsetsAtIdx(a.Cluster)
	compression := uint8(z.mmap[start])

	bs, _ := a.getBlobOffsetsAtIdx(z)
	fmt.Println(bs)
	// LZMA
	if compression == 4 {
		b := bytes.NewBuffer(z.mmap[start+1 : end+1])
		dec, err := xz.NewReader(b)
		if err != nil {
			panic(err)
		}

		var bs, be uint64

		//data := make([]byte, be-bs)
		buf := make([]byte, 2048)

		var pos uint64
		for {
			n, err := dec.Read(buf)
			if err != nil && err != io.EOF {
				panic(err)
			}

			if n == 0 {
				break
			}

			// 1st we want to find our blob address
			if bs == 0 {
				if a.Blob*4+1 > pos+uint32(n) {
					continue
				} else {
					bs = readInt64(b)
				}
			}

			// our blob has started
			if pos < bs && pos+uint64(n) >= bs {
				fmt.Println("start here", buf)
			}
			pos += uint64(n)
		}

		dec.Close()
	}

	return nil
}

func (a *Article) getBlobOffsetsAtIdx(z *ZimReader) (start, end uint64) {
	idx := a.Blob
	offset := z.clusterPtrPos + uint64(idx)*8
	start, err := readInt64(z.mmap[offset : offset+8])
	if err != nil {
		panic(err)
	}
	offset = z.clusterPtrPos + uint64(idx+1)*8
	end, err = readInt64(z.mmap[offset : offset+8])
	if err != nil {
		panic(err)
	}
	return
}

func (a *Article) String() string {
	return fmt.Sprintf("Mime: 0x%x URL: [%s], Title: [%s], Cluster: 0x%x Blob: 0x%x",
		a.Mimetype, a.URL, a.Title, a.Cluster, a.Blob)
}
