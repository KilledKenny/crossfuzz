package main

import (
	"fmt"
	"io"
	"net/http"
)

func echoHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s %s %s\n", r.Method, r.URL.String(), r.Proto)

	/** FUZZING INSTRUMENTATION CLEAR */
	// Write headers
	for name, values := range r.Header {
		for _, v := range values {
			fmt.Fprintf(w, "%s: %s\n", name, v)
		}
	}
	/** FUZZING INSTRUMENTATION COLLECT */
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
	http.HandleFunc("/", echoHandler)
	http.ListenAndServe(":9000", nil)
}
