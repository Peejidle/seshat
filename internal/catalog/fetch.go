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
	// WFCD's mirror of DE's published drop tables, for relic contents.
	dropsURL = "https://drops.warframestat.us/data/all.json"
	// The wiki's star chart: the only public list of how much mastery each
	// node gives, keyed by the node names a Warframe profile uses.
	nodesURL = "https://wiki.warframe.com/w/Module:Missions/data?action=raw"
)

// DropsFile is the cached drop tables, read by package relics.
const DropsFile = "drops.json"

// NodesFile is the cached wiki star chart, read by package profile.
const NodesFile = "nodes.lua"

// userAgent says who is asking, as the wiki asks of tools.
const userAgent = "Seshat (https://github.com/Peejidle/seshat)"

type source struct{ file, url string }

// sources lists every file Refresh keeps cached.
func sources() []source {
	var s []source
	for _, f := range SourceFiles {
		s = append(s, source{f + ".json", dataURL + f + ".json"})
	}
	return append(s, source{DropsFile, dropsURL}, source{NodesFile, nodesURL})
}

var client = &http.Client{Timeout: 60 * time.Second}

type meta struct {
	ETags map[string]string `json:"etags"`
	// Modified holds Last-Modified dates, for servers that send no ETag
	// (the wiki).
	Modified  map[string]string `json:"modified,omitempty"`
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
	if m.Modified == nil {
		m.Modified = map[string]string{}
	}
	return m
}

// HasCache reports whether every source file is present locally.
func HasCache(dir string) bool {
	for _, s := range sources() {
		if _, err := os.Stat(filepath.Join(dir, s.file)); err != nil {
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
	for _, s := range sources() {
		path := filepath.Join(dir, s.file)
		req, _ := http.NewRequest("GET", s.url, nil)
		req.Header.Set("User-Agent", userAgent)
		if _, err := os.Stat(path); err == nil {
			if et := m.ETags[s.file]; et != "" {
				req.Header.Set("If-None-Match", et)
			}
			if lm := m.Modified[s.file]; lm != "" {
				req.Header.Set("If-Modified-Since", lm)
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
			return changed, fmt.Errorf("%s: %s", s.file, resp.Status)
		}
		err = writeAtomic(path, resp.Body)
		resp.Body.Close()
		if err != nil {
			return changed, err
		}
		m.ETags[s.file] = resp.Header.Get("ETag")
		m.Modified[s.file] = resp.Header.Get("Last-Modified")
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
