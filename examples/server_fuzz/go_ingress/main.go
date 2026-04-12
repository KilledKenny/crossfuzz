package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"crossfuzz/harness/go"
)

func main() {
	crossfuzz.InitServer()

	// Upstream server you want to proxy to
	target, err := url.Parse("http://127.0.0.1:8080")
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Optional: customize the director (request rewrite step)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		crossfuzz.Collect()
		originalDirector(req)

		// You can tweak headers here if needed
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Origin-Host", target.Host)
		crossfuzz.Clear()
	}

	log.Println("Reverse proxy running on :8000 ->", target)

	// Start proxy server
	if err := http.ListenAndServe(":8000", proxy); err != nil {
		log.Fatal(err)
	}
}
