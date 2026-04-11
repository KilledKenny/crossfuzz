package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"crossfuzz/harness/gofuzz"
)

func main() {
	gofuzz.InitServer()

	// Upstream server you want to proxy to
	target, err := url.Parse("http://localhost:9000")
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Optional: customize the director (request rewrite step)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		gofuzz.Collect()
		originalDirector(req)

		// You can tweak headers here if needed
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Origin-Host", target.Host)
		gofuzz.Clear()
	}

	log.Println("Reverse proxy running on :8000 ->", target)

	// Start proxy server
	if err := http.ListenAndServe(":8000", proxy); err != nil {
		log.Fatal(err)
	}
}
