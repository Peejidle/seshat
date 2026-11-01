package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	dataURL  = "https://raw.githubusercontent.com/WFCD/warframe-items/master/data/json/"
	imageURL = "https://raw.githubusercontent.com/WFCD/warframe-items/master/data/img/"
)

var client = &http.Client{Timeout: 60 * time.Second}

type meta struct {
	ETags     map[string]string `json:"etags"`
	CheckedAt time.Time         `json:"checkedAt"`
}

func readMeta(dir string) meta {
	m := meta{ETags: map[string]string{}}
	if b, err := os.ReadFile(filepath.Join(dir, "meta.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if m.ETags == nil {
		m.ETags = map[string]string{}
	}
	return m
}

// HasCache reports whether every source file is present locally.
func HasCache(dir string) bool {
	for _, f := range SourceFiles {
		if _, err := os.Stat(filepath.Join(dir, f+".json")); err != nil {
			return false
		}
	}
	return true
}

// LastChecked is when the upstream data was last polled.
func LastChecked(dir string) time.Time { return readMeta(dir).CheckedAt }

// Refresh pulls any upstream files that changed since the last check, using
// ETags so unchanged files cost a 304. It reports how many files changed.
func Refresh(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	m := readMeta(dir)
	changed := 0
	for _, f := range SourceFiles {
		path := filepath.Join(dir, f+".json")
		req, _ := http.NewRequest("GET", dataURL+f+".json", nil)
		if et := m.ETags[f]; et != "" {
			if _, err := os.Stat(path); err == nil {
				req.Header.Set("If-None-Match", et)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			return changed, err
		}
		if resp.StatusCode == http.StatusNotModified {
			resp.Body.Close()
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return changed, fmt.Errorf("%s: %s", f, resp.Status)
		}
		err = writeAtomic(path, resp.Body)
		resp.Body.Close()
		if err != nil {
			return changed, err
		}
		m.ETags[f] = resp.Header.Get("ETag")
		changed++
	}
	m.CheckedAt = time.Now()
	b, _ := json.MarshalIndent(m, "", "  ")
	return changed, os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o644)
}

// Image returns the local path of an item's icon, downloading it if needed.
func Image(dir, imageName string) (string, error) {
	if imageName == "" {
		return "", fmt.Errorf("no image")
	}
	path := filepath.Join(dir, filepath.Base(imageName))
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	resp, err := client.Get(imageURL + imageName)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image %s: %s", imageName, resp.Status)
	}
	return path, writeAtomic(path, resp.Body)
}

func writeAtomic(path string, r io.Reader) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dl-*")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
