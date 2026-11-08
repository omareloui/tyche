// Package logview reads the app's own JSON log files (logger.Dir: the live
// one + lumberjack's gzipped backups) for the admin logs page.
package logview

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Live is the file being written; the rotated ones are LivePrefix-*.log.gz.
const (
	Live       = "tyche.log"
	livePrefix = "tyche-"
)

// DefaultLimit is how many entries a page holds.
const DefaultLimit = 200

// maxRead caps what's read of one file (from its end); the logger rotates at
// 5 MB, so a whole file in practice.
const maxRead = 8 << 20

var ErrNoFile = errors.New("no such log file")

type File struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// Files lists the live file then the rotated ones, newest first.
func Files(dir string) ([]File, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var live *File
	var rotated []File
	for _, e := range ents {
		name := e.Name()
		isLive := name == Live
		if e.IsDir() || !(isLive || strings.HasPrefix(name, livePrefix) && strings.HasSuffix(name, ".log.gz")) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		f := File{Name: name, Size: info.Size(), ModTime: info.ModTime()}
		if isLive {
			live = &f
		} else {
			rotated = append(rotated, f)
		}
	}
	// Rotated names carry their time, so newest first is by name.
	sort.Slice(rotated, func(i, j int) bool { return rotated[i].Name > rotated[j].Name })
	if live != nil {
		rotated = slices.Insert(rotated, 0, *live)
	}
	return rotated, nil
}

// Levels are zap's, lowest first.
var Levels = []string{"debug", "info", "warn", "error", "dpanic", "panic", "fatal"}

func levelRank(l string) int {
	if i := slices.Index(Levels, l); i >= 0 {
		return i
	}
	return -1
}

type Filter struct {
	// Level is the lowest level shown, "" for all.
	Level string
	// Query matches the line, case-insensitive.
	Query string
	// CorrelationID matches the entries of one request.
	CorrelationID string
	// Before is the offset to page back from (an Entry's), 0 = the end.
	Before int64
	Limit  int
}

type Entry struct {
	// Offset is the line's in the (decompressed) file: the paging cursor.
	Offset        int64
	Time          time.Time
	Level         string
	Msg           string
	CorrelationID string
	// Fields are the other keys, sorted; Stacktrace apart.
	Fields     []Field
	Stacktrace string
}

type Field struct {
	Key   string
	Value string
}

type Page struct {
	Entries []Entry
	// More is the Before of the next page, 0 when there's none.
	More int64
}

// Read pages back through file (one of Files) with f, newest first.
func Read(dir, file string, f Filter) (Page, error) {
	files, err := Files(dir)
	if err != nil {
		return Page{}, err
	}
	if !slices.ContainsFunc(files, func(x File) bool { return x.Name == file }) {
		return Page{}, ErrNoFile
	}
	data, base, err := readFile(filepath.Join(dir, file))
	if err != nil {
		return Page{}, err
	}
	if f.Limit <= 0 {
		f.Limit = DefaultLimit
	}

	end := int64(len(data))
	if f.Before > 0 && f.Before-base < end {
		end = max(f.Before-base, 0)
	}
	minRank := levelRank(f.Level)
	query := strings.ToLower(f.Query)

	var p Page
	for end > 0 {
		start := int64(bytes.LastIndexByte(data[:end-1], '\n') + 1)
		line := data[start:end]
		end = start
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if len(p.Entries) == f.Limit {
			p.More = base + start + int64(len(line))
			break
		}
		if query != "" && !bytes.Contains(bytes.ToLower(line), []byte(query)) {
			continue
		}
		e := parse(line)
		if minRank > 0 && levelRank(e.Level) < minRank {
			continue
		}
		if f.CorrelationID != "" && e.CorrelationID != f.CorrelationID {
			continue
		}
		e.Offset = base + start
		p.Entries = append(p.Entries, e)
	}
	return p, nil
}

// readFile reads the last maxRead bytes of path (decompressed), starting on
// a line, and the offset they start at.
func readFile(path string) ([]byte, int64, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer fh.Close()

	var r io.Reader = fh
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(fh)
		if err != nil {
			return nil, 0, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
		}
		defer gz.Close()
		r = gz
	} else if info, err := fh.Stat(); err == nil && info.Size() > maxRead {
		if _, err := fh.Seek(info.Size()-maxRead, io.SeekStart); err != nil {
			return nil, 0, err
		}
		data, err := io.ReadAll(fh)
		if err != nil {
			return nil, 0, err
		}
		cut := bytes.IndexByte(data, '\n') + 1
		return data[cut:], info.Size() - maxRead + int64(cut), nil
	}

	data, err := io.ReadAll(io.LimitReader(r, 4*maxRead))
	if err != nil {
		return nil, 0, err
	}
	var base int64
	if len(data) > maxRead {
		cut := len(data) - maxRead
		cut += bytes.IndexByte(data[cut:], '\n') + 1
		data, base = data[cut:], int64(cut)
	}
	return data, base, nil
}

func parse(line []byte) Entry {
	line = bytes.TrimRight(line, "\r\n")
	var m map[string]json.RawMessage
	if err := json.Unmarshal(line, &m); err != nil {
		return Entry{Msg: string(line)}
	}
	str := func(k string) string {
		var s string
		if raw, ok := m[k]; ok {
			_ = json.Unmarshal(raw, &s)
			delete(m, k)
		}
		return s
	}
	e := Entry{
		Level:         str("level"),
		Msg:           str("msg"),
		CorrelationID: str("correlation_id"),
		Stacktrace:    str("stacktrace"),
	}
	if ts := str("timestamp"); ts != "" {
		for _, layout := range []string{"2006-01-02T15:04:05.000Z0700", time.RFC3339Nano} {
			if t, err := time.Parse(layout, ts); err == nil {
				e.Time = t
				break
			}
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := string(m[k])
		var s string
		if json.Unmarshal(m[k], &s) == nil {
			v = s
		}
		e.Fields = append(e.Fields, Field{Key: k, Value: v})
	}
	return e
}
