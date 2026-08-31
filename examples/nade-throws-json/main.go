package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	demoPath := flag.String("demo", "", "Path to CS2 demo file (.dem)")
	outPath := flag.String("out", "", "Optional output JSON file path (default: stdout)")
	pretty := flag.Bool("pretty", true, "Pretty-print JSON")
	listen := flag.String("listen", "", "If set, run HTTP backend on this address (e.g. :8080)")
	flag.Parse()

	if *listen != "" {
		runHTTPServer(*listen, *pretty)
		return
	}

	if *demoPath == "" {
		fmt.Fprintln(os.Stderr, "usage:")
		fmt.Fprintln(os.Stderr, "  go run . -demo /path/to/demo.dem [-out throws.json]")
		fmt.Fprintln(os.Stderr, "  go run . -listen :8080")
		os.Exit(2)
	}

	data, err := ExportDemoThrowsFile(*demoPath)
	if err != nil {
		log.Fatalf("export failed: %v", err)
	}

	var w io.Writer = os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			log.Fatalf("create output: %v", err)
		}
		defer f.Close()
		w = f
	}

	if err := writeJSON(w, data, *pretty); err != nil {
		log.Fatalf("write json: %v", err)
	}
}

func runHTTPServer(addr string, pretty bool) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST a demo file as multipart field \"demo\"", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseMultipartForm(512 << 20); err != nil { // 512 MiB
			http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("demo")
		if err != nil {
			http.Error(w, "missing multipart file field \"demo\"", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := ExportDemoThrows(file)
		if err != nil {
			http.Error(w, "export failed: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := writeJSON(w, data, pretty); err != nil {
			log.Printf("write response: %v", err)
		}
	})

	log.Printf("nade-throws-json listening on %s", addr)
	log.Printf("POST /export with multipart field \"demo\"")
	log.Fatal(http.ListenAndServe(addr, mux))
}
