package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Entry struct {
	Text string    `json:"text"`
	TS   time.Time `json:"ts"`
}

type Store struct {
	Path string
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".echo", "history.jsonl"), nil
}

func NewDefault() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return &Store{Path: path}, nil
}

func (s *Store) ensureDir() error {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return errors.New("history store path is empty")
	}
	return os.MkdirAll(filepath.Dir(s.Path), 0o755)
}

func (s *Store) Append(text string) error {
	if s == nil {
		return errors.New("history store is nil")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if err := s.ensureDir(); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := Entry{Text: text, TS: time.Now()}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) LoadTexts() ([]string, error) {
	if s == nil {
		return nil, errors.New("history store is nil")
	}
	if strings.TrimSpace(s.Path) == "" {
		return nil, errors.New("history store path is empty")
	}
	f, err := os.Open(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var out []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if strings.TrimSpace(e.Text) == "" {
			continue
		}
		out = append(out, e.Text)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
