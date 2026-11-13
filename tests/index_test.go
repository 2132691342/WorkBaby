// 工作区预索引：建索引、模糊检索与频次排序。
// 这条能力的价值在大仓库：模型找文件从「几秒的全盘 grep」变成「毫秒级查表」。
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/index"
)

// TestIndexBuildFindsFilesAndSymbols 断言建索引的收录边界：源文件与符号进、二进制与依赖目录不进。
func TestIndexBuildFindsFilesAndSymbols(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), `package main

func StartServer() error {
	return nil
}

type Config struct {
	Port int
}
`)
	writeFile(t, filepath.Join(root, "app.ts"), `export function renderApp() {}
export class Widget {}
`)
	// 二进制与依赖目录不该进索引：node_modules 进了索引就是一堆噪声命中。
	writeFile(t, filepath.Join(root, "logo.png"), "\x89PNG\r\n\x1a\n")
	noise := filepath.Join(root, "node_modules", "pkg")
	if err := os.MkdirAll(noise, 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	writeFile(t, filepath.Join(noise, "index.js"), "module.exports = {}")

	docs, err := index.Build(root)
	if err != nil {
		t.Fatalf("建索引失败：%v", err)
	}
	paths := map[string]bool{}
	symbols := map[string]bool{}
	for _, doc := range docs {
		paths[doc.DocPath] = true
		if doc.DocKind == index.KindSymbol {
			symbols[doc.DocSymbol] = true
		}
		if strings.Contains(doc.DocPath, "node_modules") {
			t.Fatalf("依赖目录不该进索引：%s", doc.DocPath)
		}
	}
	if !paths["main.go"] || !paths["app.ts"] {
		t.Fatalf("源文件该进索引：%v", paths)
	}
	if paths["logo.png"] {
		t.Fatal("二进制不该进索引")
	}
	if !symbols["StartServer"] || !symbols["Config"] {
		t.Fatalf("Go 符号该被抽出：%v", symbols)
	}
	if !symbols["renderApp"] || !symbols["Widget"] {
		t.Fatalf("TS 符号该被抽出：%v", symbols)
	}
}

// TestIndexSearchRanksSymbolFirst 断言检索排序：符号名命中排在路径命中之前，常用近用的再往前。
func TestIndexSearchRanksSymbolFirst(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "helper.go"), `package helper

func ParseConfig() {}
`)
	writeFile(t, filepath.Join(root, "parse-config-notes.md"), "notes")

	docs, err := index.Build(root)
	if err != nil {
		t.Fatalf("建索引失败：%v", err)
	}
	hits := index.Search(docs, "parseconfig", nil, 10)
	if len(hits) == 0 {
		t.Fatal("该搜到 ParseConfig 符号")
	}
	if hits[0].HitDoc.DocKind != index.KindSymbol {
		t.Fatalf("符号命中该排前面，实际是 %s", hits[0].HitDoc.DocKind)
	}
	if hits[0].HitDoc.DocSymbol != "ParseConfig" {
		t.Fatalf("该命中 ParseConfig：%v", hits[0].HitDoc)
	}

	// ---- frecency：常用且近用的文档排得更前 ----
	ranked := []index.Doc{
		{DocID: "F:a.go", DocPath: "a.go", DocKind: index.KindFile},
		{DocID: "F:b.go", DocPath: "b.go", DocKind: index.KindFile},
	}
	frecency := index.NewFrecency()
	// b.go 被反复用到，a.go 从没用过。
	for range 5 {
		frecency.Record("F:b.go", 1)
	}
	frecency.Record("F:a.go", 1)

	hits = index.Search(ranked, "go", frecency, 10)
	if len(hits) < 2 {
		t.Fatalf("该搜到两个文件，实际 %d", len(hits))
	}
	if hits[0].HitDoc.DocPath != "b.go" {
		t.Fatalf("常用文件该排前面，实际 %s", hits[0].HitDoc.DocPath)
	}
}
