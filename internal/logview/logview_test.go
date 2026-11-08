package logview

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func line(i int, level, msg, cid string) string {
	return fmt.Sprintf(`{"level":%q,"timestamp":"2026-11-08T20:58:%02d.640+0200","msg":%q,"correlation_id":%q,"status_code":200,"url":"/x/%d"}`+"\n", level, i%60, msg, cid, i)
}

func writeLogs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	for i := range 10 {
		level := "info"
		if i%3 == 0 {
			level = "error"
		}
		b.WriteString(line(i, level, fmt.Sprintf("msg %d", i), fmt.Sprintf("cid%d", i%2)))
	}
	b.WriteString("not json\n")
	if err := os.WriteFile(filepath.Join(dir, Live), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "tyche-2026-11-01T09-14-12.145.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	gz.Write([]byte(line(0, "warn", "old one", "")))
	gz.Close()
	f.Close()
	os.WriteFile(filepath.Join(dir, "tyche-2026-11-03T09-14-12.145.log.gz"), nil, 0o600)
	os.WriteFile(filepath.Join(dir, "other.txt"), nil, 0o600)
	return dir
}

func msgs(p Page) []string {
	var out []string
	for _, e := range p.Entries {
		out = append(out, e.Msg)
	}
	return out
}

func TestFiles(t *testing.T) {
	files, err := Files(writeLogs(t))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	want := []string{Live, "tyche-2026-11-03T09-14-12.145.log.gz", "tyche-2026-11-01T09-14-12.145.log.gz"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("files = %v, want %v", names, want)
	}
}

func TestReadPagesNewestFirst(t *testing.T) {
	dir := writeLogs(t)
	p, err := Read(dir, Live, Filter{Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if got := msgs(p); fmt.Sprint(got) != "[not json msg 9 msg 8 msg 7]" {
		t.Errorf("page 1 = %v", got)
	}
	e := p.Entries[1]
	if e.Level != "error" || e.CorrelationID != "cid1" || e.Time.IsZero() {
		t.Errorf("entry = %+v", e)
	}
	if fmt.Sprint(e.Fields) != "[{status_code 200} {url /x/9}]" {
		t.Errorf("fields = %v", e.Fields)
	}

	var all []string
	all = append(all, msgs(p)...)
	for p.More > 0 {
		if p, err = Read(dir, Live, Filter{Limit: 4, Before: p.More}); err != nil {
			t.Fatal(err)
		}
		all = append(all, msgs(p)...)
	}
	if len(all) != 11 || all[10] != "msg 0" {
		t.Errorf("all pages = %v", all)
	}
}

func TestReadFilters(t *testing.T) {
	dir := writeLogs(t)
	for name, tc := range map[string]struct {
		f    Filter
		want string
	}{
		"level": {Filter{Level: "error"}, "[msg 9 msg 6 msg 3 msg 0]"},
		"query": {Filter{Query: "/X/4"}, "[msg 4]"},
		"cid":   {Filter{CorrelationID: "cid1", Level: "error"}, "[msg 9 msg 3]"},
	} {
		p, err := Read(dir, Live, tc.f)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(msgs(p)); got != tc.want {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

func TestReadGzip(t *testing.T) {
	p, err := Read(writeLogs(t), "tyche-2026-11-01T09-14-12.145.log.gz", Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(msgs(p)) != "[old one]" || p.Entries[0].Level != "warn" {
		t.Errorf("gz = %+v", p)
	}
}

func TestReadOnlyListedFiles(t *testing.T) {
	dir := writeLogs(t)
	for _, name := range []string{"../" + Live, "other.txt", "/etc/passwd"} {
		if _, err := Read(dir, name, Filter{}); err != ErrNoFile {
			t.Errorf("%s: err = %v, want ErrNoFile", name, err)
		}
	}
}
