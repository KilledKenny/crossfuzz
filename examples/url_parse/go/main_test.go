package main

import "testing"

func FuzzTarget(f *testing.F) {
	f.Add([]byte("http://example.com/"))
	f.Add([]byte("https://user:pass@example.com:8080/path?q=1#frag"))
	f.Add([]byte("ftp://ftp.example.com/pub/file.txt"))
	f.Add([]byte("http://[::1]/ipv6"))
	f.Add([]byte("http://example.com/%2F..%2F"))

	f.Fuzz(func(t *testing.T, data []byte) {
		target(data)
	})
}
