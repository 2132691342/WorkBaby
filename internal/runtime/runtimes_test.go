package runtime

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")
	require.NoError(t, writeZip(archive, map[string]string{
		"../escape.txt": "boom",
	}))
	err := Extract(archive, filepath.Join(dir, "out"), 0, 100, 1<<20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestExtractEnforcesLimits(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "big.zip")
	require.NoError(t, writeZip(archive, map[string]string{
		"a.txt": "hello",
		"b.txt": "world",
	}))
	// maxFiles=1 触发
	err := Extract(archive, filepath.Join(dir, "out1"), 0, 1, 1<<20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxFiles")
	// maxBytes 触发
	err = Extract(archive, filepath.Join(dir, "out2"), 0, 10, 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxBytes")
}


func writeZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}
	return zw.Close()
}
