package mirror

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

type Config struct {
	StartURL string
	MaxDepth int
	Workers  int
	OutDir   string
	Timeout  time.Duration
}

type crawler struct {
	cfg       Config
	client    *http.Client
	baseURL   *url.URL
	baseHost  string
	jobs      chan job
	scheduleW sync.WaitGroup
	workerW   sync.WaitGroup

	mu      sync.Mutex
	visited map[string]struct{}
}

type job struct {
	rawURL string
	depth  int
}

type htmlRef struct {
	node   *html.Node
	attr   string
	rawVal string
	isPage bool
}

func Run(cfg Config) error {
	if cfg.StartURL == "" {
		return errors.New("start URL is required")
	}
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.OutDir == "" {
		cfg.OutDir = "mirror_output"
	}

	baseURL, err := url.Parse(cfg.StartURL)
	if err != nil {
		return fmt.Errorf("parse start url: %w", err)
	}
	if baseURL.Host == "" {
		return errors.New("start URL must include host")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return errors.New("only http and https schemes are supported")
	}

	c := &crawler{
		cfg:      cfg,
		client:   &http.Client{Timeout: cfg.Timeout},
		baseURL:  baseURL,
		baseHost: strings.ToLower(baseURL.Host),
		jobs:     make(chan job, cfg.Workers*4),
		visited:  make(map[string]struct{}),
	}

	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	for i := 0; i < cfg.Workers; i++ {
		c.workerW.Add(1)
		go c.worker()
	}

	if err := c.enqueue(cfg.StartURL, 0); err != nil {
		return err
	}

	c.scheduleW.Wait()
	close(c.jobs)
	c.workerW.Wait()
	return nil
}

func (c *crawler) enqueue(rawURL string, depth int) error {
	normalized, err := c.normalize(rawURL)
	if err != nil {
		return err
	}
	if !c.withinDomain(normalized) {
		return nil
	}

	c.mu.Lock()
	if _, exists := c.visited[normalized]; exists {
		c.mu.Unlock()
		return nil
	}
	c.visited[normalized] = struct{}{}
	c.scheduleW.Add(1)
	c.mu.Unlock()

	// Do not block worker goroutines on a full queue: otherwise all workers can
	// end up waiting on send and no one is left to consume jobs.
	go func() {
		c.jobs <- job{rawURL: normalized, depth: depth}
	}()
	return nil
}

func (c *crawler) worker() {
	defer c.workerW.Done()
	for j := range c.jobs {
		if err := c.handleJob(j); err != nil {
			fmt.Printf("warning: %s: %v\n", j.rawURL, err)
		}
		c.scheduleW.Done()
	}
}

func (c *crawler) handleJob(j job) error {
	req, err := http.NewRequest(http.MethodGet, j.rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mini-wget/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	parsedURL, err := url.Parse(j.rawURL)
	if err != nil {
		return err
	}

	localAbsPath, err := c.localPath(parsedURL)
	if err != nil {
		return err
	}

	if strings.Contains(contentType, "text/html") {
		body, err = c.processHTML(body, parsedURL, localAbsPath, j.depth)
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(localAbsPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(localAbsPath, body, 0o644)
}

func (c *crawler) processHTML(body []byte, pageURL *url.URL, pageLocalPath string, depth int) ([]byte, error) {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	refs := collectRefs(root)
	pageDir := filepath.Dir(pageLocalPath)

	for _, ref := range refs {
		resolved, err := resolveURL(pageURL, ref.rawVal)
		if err != nil || !c.withinDomain(resolved.String()) {
			continue
		}

		linkDepth := depth
		if ref.isPage {
			linkDepth = depth + 1
			if linkDepth > c.cfg.MaxDepth {
				continue
			}
		}

		_ = c.enqueue(resolved.String(), linkDepth)

		targetLocalAbs, err := c.localPath(resolved)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(pageDir, targetLocalAbs)
		if err != nil {
			continue
		}
		relative = filepath.ToSlash(relative)
		setAttr(ref.node, ref.attr, relative)
	}

	var out bytes.Buffer
	if err := html.Render(&out, root); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (c *crawler) normalize(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if !u.IsAbs() {
		u = c.baseURL.ResolveReference(u)
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

func (c *crawler) withinDomain(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, c.baseHost)
}

func (c *crawler) localPath(u *url.URL) (string, error) {
	safePath := u.EscapedPath()
	if safePath == "" || safePath == "/" {
		safePath = "/index.html"
	} else if strings.HasSuffix(safePath, "/") {
		safePath += "index.html"
	} else if path.Ext(safePath) == "" {
		safePath += "/index.html"
	}
	safePath = sanitizeEscapedPathForFS(safePath)

	full := filepath.Join(c.cfg.OutDir, filepath.FromSlash(u.Host), filepath.FromSlash(path.Clean(safePath)))
	if u.RawQuery != "" {
		base := filepath.Base(full)
		ext := filepath.Ext(base)
		nameOnly := strings.TrimSuffix(base, ext)
		sfx := shortHash(u.RawQuery)
		base = fmt.Sprintf("%s_q_%s%s", nameOnly, sfx, ext)
		full = filepath.Join(filepath.Dir(full), base)
	}

	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:10]
}

func resolveURL(base *url.URL, href string) (*url.URL, error) {
	if href == "" {
		return nil, errors.New("empty href")
	}
	trimmed := strings.TrimSpace(href)
	if strings.HasPrefix(trimmed, "javascript:") || strings.HasPrefix(trimmed, "mailto:") || strings.HasPrefix(trimmed, "tel:") || strings.HasPrefix(trimmed, "#") {
		return nil, errors.New("unsupported href scheme")
	}

	ref, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	resolved := base.ResolveReference(ref)
	resolved.Fragment = ""
	return resolved, nil
}

func collectRefs(root *html.Node) []htmlRef {
	var out []htmlRef
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "a":
				if val, ok := getAttr(n, "href"); ok {
					out = append(out, htmlRef{node: n, attr: "href", rawVal: val, isPage: true})
				}
			case "link":
				if val, ok := getAttr(n, "href"); ok {
					out = append(out, htmlRef{node: n, attr: "href", rawVal: val})
				}
			case "script", "img", "iframe", "audio", "video", "source":
				if val, ok := getAttr(n, "src"); ok {
					out = append(out, htmlRef{node: n, attr: "src", rawVal: val})
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func getAttr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

func setAttr(n *html.Node, key, value string) {
	for i := range n.Attr {
		if strings.EqualFold(n.Attr[i].Key, key) {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}

var invalidFSChars = regexp.MustCompile(`[<>:"\\|?*\x00-\x1F]`)

func sanitizeEscapedPathForFS(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		s := invalidFSChars.ReplaceAllString(parts[i], "_")
		s = strings.TrimSpace(strings.TrimRight(s, ". "))
		if s == "" {
			s = "_"
		}
		parts[i] = s
	}
	out := strings.Join(parts, "/")
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return out
}
