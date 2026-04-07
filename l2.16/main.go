package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"l2.16/mirror"
)

func main() {
	var (
		startURL string
		depth    int
		workers  int
		outDir   string
		timeout  time.Duration
	)

	flag.StringVar(&startURL, "url", "", "start URL to mirror (required)")
	flag.IntVar(&depth, "depth", 1, "maximum link depth for HTML pages")
	flag.IntVar(&workers, "workers", 6, "number of concurrent downloads")
	flag.StringVar(&outDir, "out", "mirror_output", "output directory for mirrored content")
	flag.DurationVar(&timeout, "timeout", 15*time.Second, "HTTP request timeout")
	flag.Parse()

	if startURL == "" {
		flag.Usage()
		os.Exit(2)
	}
	if workers < 1 {
		log.Fatalf("workers must be >= 1")
	}
	if depth < 0 {
		log.Fatalf("depth must be >= 0")
	}

	u, err := url.Parse(startURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		log.Fatalf("invalid start URL: %q", startURL)
	}

	cfg := mirror.Config{
		StartURL: startURL,
		MaxDepth: depth,
		Workers:  workers,
		OutDir:   outDir,
		Timeout:  timeout,
	}

	if err := mirror.Run(cfg); err != nil {
		log.Fatalf("mirror failed: %v", err)
	}

	fmt.Printf("Mirror completed. Files saved to %s\n", outDir)
}
