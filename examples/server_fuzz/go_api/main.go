package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"crossfuzz/harness/gofuzz"
)

func echoHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s %s %s\n", r.Method, r.URL.String(), r.Proto)

	data := r.URL.Query().Get("data")
	var v any
	json.Unmarshal([]byte(data), &v)
	gofuzz.Clear()
	// Write headers
	for name, values := range r.Header {
		for _, v := range values {
			fmt.Fprintf(w, "%s: %s\n", name, v)
		}
	}
	gofuzz.Collect()
	fmt.Fprintln(w)

	// Write body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	w.Write(body)
}

func main() {
	gofuzz.InitServer()
	http.HandleFunc("/", echoHandler)
	http.ListenAndServe(":9000", nil)
}
