// Builds the billing dashboard for the browser and serves it. Run it from the
// repository root: go run ./examples/billing/serve
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const page = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sable billing</title>
<style>html, body { height: 100%; margin: 0; background: #0c1017 }</style>
<script src="wasm_exec.js"></script>
<script type="module">
  import { start } from "./qui.js";
  await start({ wasm: "app.wasm", root: document.body });
</script>
`

func goRoot() string {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		log.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func main() {
	address := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	flag.Parse()

	dir, err := os.MkdirTemp("", "qui-billing")
	if err != nil {
		log.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "app.wasm"), "./examples/billing")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		log.Fatal(err)
	}
	copies := map[string]string{
		"qui.js":       "web/qui.js",
		"wire.gen.js":  "web/wire.gen.js",
		"wasm_exec.js": filepath.Join(goRoot(), "lib/wasm/wasm_exec.js"),
	}
	for name, source := range copies {
		data, err := os.ReadFile(source)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Sable billing: http://%s\n", *address)
	log.Fatal(http.ListenAndServe(*address, http.FileServer(http.Dir(dir))))
}
