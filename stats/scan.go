package main

import (
	"cmp"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// maxFiles bounds a scan, so a huge folder can't keep the extension busy.
const maxFiles = 200000

// settings are the extension's settings, as Flint sends them (every key
// present: Flint fills in the manifest's defaults).
type settings struct {
	Exclude     []string `json:"exclude"`
	TopFiles    int      `json:"topFiles"`
	CountHidden bool     `json:"countHidden"`
	SortBy      string   `json:"sortBy"`
}

// fileType is what the panel shows for one extension (".go").
type fileType struct {
	Name    string
	Files   int
	Bytes   int64
	Largest []file // the biggest, at most settings.TopFiles
}

type file struct {
	Path string
	Size int64
}

type stats struct {
	Files     int
	Bytes     int64
	Types     []fileType
	Truncated bool
}

// scan counts the files under roots.
func scan(roots []string, s settings) stats {
	byType := map[string]*fileType{}
	var out stats
	skip := map[string]bool{}
	for _, name := range s.Exclude {
		skip[name] = true
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable: skip
			}
			name := d.Name()
			hidden := strings.HasPrefix(name, ".") && path != root
			if d.IsDir() {
				if path != root && (skip[name] || (hidden && !s.CountHidden) || name == ".git") {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || (hidden && !s.CountHidden) {
				return nil
			}
			if out.Files >= maxFiles {
				out.Truncated = true
				return filepath.SkipAll
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(name))
			if ext == "" {
				ext = "(no extension)"
			}
			t := byType[ext]
			if t == nil {
				t = &fileType{Name: ext}
				byType[ext] = t
			}
			t.Files++
			t.Bytes += info.Size()
			t.Largest = keepLargest(t.Largest, file{Path: path, Size: info.Size()}, s.TopFiles)
			out.Files++
			out.Bytes += info.Size()
			return nil
		})
	}
	for _, t := range byType {
		out.Types = append(out.Types, *t)
	}
	slices.SortFunc(out.Types, func(a, b fileType) int {
		if s.SortBy == "size" {
			return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.Name, b.Name))
		}
		return cmp.Or(cmp.Compare(b.Files, a.Files), cmp.Compare(a.Name, b.Name))
	})
	return out
}

// keepLargest adds f to list, keeping the n largest, biggest first.
func keepLargest(list []file, f file, n int) []file {
	i, _ := slices.BinarySearchFunc(list, f, func(a, b file) int { return cmp.Compare(b.Size, a.Size) })
	list = slices.Insert(list, i, f)
	if len(list) > n {
		list = list[:n]
	}
	return list
}

// size is a byte count for people: 1.2 MB.
func size(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f, i := float64(n)/1000, 0
	for f >= 1000 && i < len(units)-1 {
		f /= 1000
		i++
	}
	return strconv.FormatFloat(f, 'f', 1, 64) + " " + units[i]
}
