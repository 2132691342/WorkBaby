// Package repo 是壳自己的本地存储：单个 JSON 文件，原子整写 + 互斥锁。
// 装配阶段还会清掉数据根下残留的 SQLite 文件（workbaby.db/-shm/-wal）：
// 它们不再被任何代码读写，留着只会占磁盘并误导用户以为数据还在里面。
package repo

import (
	"os"
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// sqliteMagic 是 SQLite 文件头的前 16 字节。判据用它而不是看后缀：
// 后缀相同的文件未必是我们当初写的那个库，误删等于删用户数据。
const sqliteMagic = "SQLite format 3\x00"

// legacyDBFiles 是历史存储留下的文件名（含 WAL 模式的两个伙伴文件）。
var legacyDBFiles = []string{"workbaby.db", "workbaby.db-shm", "workbaby.db-wal"}

// CleanupLegacyDB 删掉数据根下历史遗留的 SQLite 文件。
//
// 三个前提，少一个都不动：
//  1. JSON 存储文件已存在——它才是现在的真相，没它就删旧库等于两头落空
//  2. 主库文件确实是 SQLite（不是用户自己放的同名文件）
//  3. 只在数据根下一层，不递归——别的目录里的 .db 与我们无关
//
// 返回删掉的路径，装配日志里记一笔，出问题能对上账。
func CleanupLegacyDB(dataDir, storePath string) []string {
	if !pkg.Exists(storePath) {
		return nil
	}
	main := filepath.Join(dataDir, legacyDBFiles[0])
	if !pkg.Exists(main) || !isSQLiteFile(main) {
		return nil
	}
	removed := make([]string, 0, len(legacyDBFiles))
	for _, name := range legacyDBFiles {
		path := filepath.Join(dataDir, name)
		if !pkg.Exists(path) {
			continue
		}
		if err := os.Remove(path); err != nil {
			// 删不掉不算故障：文件被别的进程占着就留着，下次启动再试。
			pkg.Warn("遗留存储文件删除失败 %s：%v", path, err)
			continue
		}
		removed = append(removed, path)
	}
	if len(removed) > 0 {
		pkg.Info("已清理历史的 SQLite 存储文件 %d 个", len(removed))
	}
	return removed
}

// isSQLiteFile 读文件头判断是不是 SQLite 库；读不到给 false。
func isSQLiteFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, len(sqliteMagic))
	if _, err := file.Read(head); err != nil {
		return false
	}
	return string(head) == sqliteMagic
}
