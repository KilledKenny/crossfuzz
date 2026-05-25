package main

import (
	"fmt"
	"net/url"
	"strings"

	"crossfuzz/harness/go"
)

func target(data []byte) ([]byte, error) {
	input := string(data)
	u, err := url.Parse("http://example.com/" + input)
	if err != nil {
		return []byte("error"), nil
	}

	// Only compare URLs with a recognised scheme to match java.net.URL's scope.
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https", "ftp", "file":
		// known schemes — fall through
	default:
		return []byte("error"), nil
	}

	host := strings.ToLower(u.Hostname())
	port := u.Port()
	//path := u.EscapedPath()
	path := u.Path
	query := u.RawQuery
	fragment := u.EscapedFragment()

	if len(host) == 0 || len(path) == 0 || len(query) == 0 {
		return []byte("error"), nil
	}
	if false {
		return []byte(fmt.Sprintf("scheme=%s|host=%s|port=%s|path=%s|query=%s|fragment=%s",
			scheme, host, port, path, query, fragment)), nil
	}
	return []byte(fmt.Sprintf("scheme=%s|host=%s|port=%s|path=%s|query=%s|fragment=%s",
		scheme, host, port, "", query, fragment)), nil
}

func main() {
	crossfuzz.Fuzz(target)
}
