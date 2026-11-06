package service

// 技能落盘的两个原语：整目录复制与原子复制。
// 单独成文件是因为它们被落位、导入、重置三处共用，
// 且「中途失败不能留下半个技能」这条纪律值得独立成文。

import (
	"os"
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// copyTree 递归复制目录；目标已存在时合并。
func copyTree(from, to string) error {
	if err := pkg.EnsureDir(to); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(from, entry.Name())
		dst := filepath.Join(to, entry.Name())
		if entry.IsDir() {
			if err := copyTree(src, dst); err != nil {
				return err
			}
			continue
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyTreeAtomic 整目录复制：先落到临时目录，成功后一次换名。
// 逐文件直接写进目标的话，进程在中途被杀会留下半个技能——
// 而目录一旦存在，下次 Provision 就认为它已落位，缺口永远补不上。
func copyTreeAtomic(from, to string) error {
	staging := to + ".copying"
	if err := removeAllQuiet(staging); err != nil {
		return err
	}
	if err := copyTree(from, staging); err != nil {
		_ = removeAllQuiet(staging)
		return err
	}
	if err := removeAllQuiet(to); err != nil {
		_ = removeAllQuiet(staging)
		return err
	}
	if err := os.Rename(staging, to); err != nil {
		_ = removeAllQuiet(staging)
		return err
	}
	return nil
}
