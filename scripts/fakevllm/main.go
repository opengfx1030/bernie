// Command fakevllm serves a fake OpenAI-compatible server for the bench
// smoke test: /health, /v1/models, /tokenize and streaming
// /v1/completions. With --recall correct it repeats the hidden fact of the
// prompt; with --recall wrong it answers another number.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8000", "address to listen on")
	recall := flag.String("recall", "off", "answer the hidden fact of the prompt: off, correct or wrong")
	model := flag.String("model", "fake-model", "served model id")
	flag.Parse()
	if *recall != "off" && *recall != "correct" && *recall != "wrong" {
		fmt.Fprintln(os.Stderr, "fakevllm: --recall must be off, correct or wrong")
		os.Exit(2)
	}
	mode := ""
	if *recall != "off" {
		mode = *recall
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           vllmtest.New(vllmtest.Config{Model: *model, RecallMode: mode}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("fake vLLM server on http://%s (model %s, recall %s)\n", *listen, *model, *recall)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
