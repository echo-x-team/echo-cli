package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"echo-cli/internal/agent"

	"github.com/google/uuid"
)

type Record struct {
	ID       string          `json:"id"`
	Workdir  string          `json:"workdir,omitempty"`
	Messages []agent.Message `json:"messages"`
	Updated  time.Time       `json:"updated"`
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".echo", "sessions"), nil
}

func ensureDir() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func Save(id string, workdir string, messages []agent.Message) (string, error) {
	if id == "" {
		id = uuid.NewString()
	}
	d, err := ensureDir()
	if err != nil {
		return "", err
	}
	rec := Record{ID: id, Workdir: workdir, Messages: messages, Updated: time.Now()}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(d, id+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func Load(id string) (Record, error) {
	var rec Record
	d, err := dir()
	if err != nil {
		return rec, err
	}
	path := filepath.Join(d, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, err
	}
	return rec, nil
}

func Last() (Record, error) {
	d, err := dir()
	if err != nil {
		return Record{}, err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		return Record{}, err
	}
	if len(entries) == 0 {
		return Record{}, fmt.Errorf("no sessions found")
	}
	sort.Slice(entries, func(i, j int) bool {
		iInfo, _ := entries[i].Info()
		jInfo, _ := entries[j].Info()
		if iInfo == nil || jInfo == nil {
			return entries[i].Name() > entries[j].Name()
		}
		return iInfo.ModTime().After(jInfo.ModTime())
	})
	return Load(trimExt(entries[0].Name()))
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

func ListIDs() ([]string, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := trimExt(e.Name())
		ids = append(ids, name)
	}
	return ids, nil
}

// List returns session records, optionally filtering by workdir when available.
func List(showAll bool, workdir string) ([]Record, error) {
	ids, err := ListIDs()
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, id := range ids {
		rec, err := Load(id)
		if err != nil {
			continue
		}
		if showAll || rec.Workdir == "" || workdir == "" || samePath(rec.Workdir, workdir) {
			records = append(records, rec)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Updated.After(records[j].Updated)
	})
	return records, nil
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return absA == absB
}
