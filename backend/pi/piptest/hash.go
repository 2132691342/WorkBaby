package piptest

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

// digest 是夹具指纹的累加器：把每个源文件的名字与内容依次喂进去，
// 顺序与内容任一变化，摘要就变。
type digest struct{ h hash.Hash }

func newDigest() *digest { return &digest{h: sha256.New()} }

func (d *digest) write(b []byte) { d.h.Write(b) }

func (d *digest) sum() string { return hex.EncodeToString(d.h.Sum(nil))[:16] }
