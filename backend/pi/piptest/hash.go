// piptest 内部的夹具指纹工具：识别假 PI 源码变化，决定要不要重编。
package piptest

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

// digest 是夹具指纹的累加器：任意源文件名字或内容变化，摘要就变。
type digest struct{ h hash.Hash }

func newDigest() *digest { return &digest{h: sha256.New()} }

func (d *digest) write(b []byte) { d.h.Write(b) }

func (d *digest) sum() string { return hex.EncodeToString(d.h.Sum(nil))[:16] }
