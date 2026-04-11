package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	// Upstream server you want to proxy to
	target, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Optional: customize the director (request rewrite step)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		/** FUZZING INSTRUMENTATION COLLECT */
		originalDirector(req)

		// You can tweak headers here if needed
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Origin-Host", target.Host)
		/** FUZZING INSTRUMENTATION CLEAR */
	}

	log.Println("Reverse proxy running on :8000 ->", target)

	// Start proxy server
	if err := http.ListenAndServe(":8000", proxy); err != nil {
		log.Fatal(err)
	}
}
