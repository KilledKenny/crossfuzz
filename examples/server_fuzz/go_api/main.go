package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/KilledKenny/crossfuzz/harness/go"
)

func echoHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s %s %s\n", r.Method, r.URL.String(), r.Proto)

	data := r.URL.Query().Get("data")
	var v any
	json.Unmarshal([]byte(data), &v)
	crossfuzz.Clear()
	// Write headers
	for name, values := range r.Header {
		for _, v := range values {
			fmt.Fprintf(w, "%s: %s\n", name, v)
		}
	}
	crossfuzz.Collect()
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
	crossfuzz.InitServer()
	http.HandleFunc("/", echoHandler)
	http.ListenAndServe(":9000", nil)
}
