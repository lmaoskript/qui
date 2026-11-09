// Command qui builds a qui app into a static folder ready to host.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/andybalholm/brotli"
	"github.com/evanw/esbuild/pkg/api"
)

const quiModule = "github.com/torisetxd/qui"

const defaultPage = `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>html,body{height:100%%;margin:0;background:%s}</style>
%s
`

var (
	generated  = regexp.MustCompile(`^(index\.html|app\.[0-9a-f]{8}\.(wasm|js))(\.br|\.gz)?$`)
	closingTag = regexp.MustCompile(`(?i)</(head|body)>`)
)

type options struct {
	app, out, title, bg, sans, mono, preventKeys string
	target, arch, id                             string
	compress, console                            bool
}

type artifact struct {
	name         string
	data, gz, br []byte
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "build" {
		fmt.Fprintln(os.Stderr, "usage: qui build [flags] [app folder]")
		os.Exit(2)
	}
	o, err := parseArgs(os.Args[2:])
	if err == nil {
		err = build(o)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "qui build:", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, error) {
	o := options{app: "."}
	flags := flag.NewFlagSet("qui build", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: qui build [flags] [app folder]")
		flags.PrintDefaults()
	}
	flags.StringVar(&o.target, "target", "web", "web, windows or android")
	flags.StringVar(&o.arch, "arch", "", "CPU for native targets: amd64 or arm64 (android takes a comma-separated list, default both)")
	flags.StringVar(&o.id, "id", "", "android package id (default dev.qui.<app folder>)")
	flags.BoolVar(&o.console, "console", false, "windows: keep a console window for logging")
	flags.StringVar(&o.out, "o", "", "output folder (default <app folder>/dist, or dist/<target> for native targets)")
	flags.StringVar(&o.title, "title", "", "page title until the app sets its own (default the app folder name)")
	flags.StringVar(&o.bg, "bg", "#0e0f11", "page background shown before the app starts")
	flags.StringVar(&o.sans, "sans", "", "CSS font-family for normal text")
	flags.StringVar(&o.mono, "mono", "", "CSS font-family for monospace text")
	flags.StringVar(&o.preventKeys, "prevent-keys", "", "comma-separated browser shortcuts the app keeps, such as ctrl+k")
	flags.BoolVar(&o.compress, "compress", true, "write .br and .gz copies")

	var folders []string
	for {
		flags.Parse(args)
		if flags.NArg() == 0 {
			break
		}
		folders = append(folders, flags.Arg(0))
		args = flags.Args()[1:]
	}
	if len(folders) > 1 {
		return o, fmt.Errorf("expected one app folder, got %d", len(folders))
	}
	if len(folders) == 1 {
		o.app = folders[0]
	}
	return o, nil
}

func build(o options) error {
	app, err := filepath.Abs(o.app)
	if err != nil {
		return err
	}
	if info, err := os.Stat(app); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a folder", app)
	}
	if o.target != "web" && o.target != "windows" && o.target != "android" {
		return fmt.Errorf("unknown -target %s", o.target)
	}
	if o.out == "" {
		o.out = filepath.Join(app, "dist")
		if o.target != "web" {
			o.out = filepath.Join(app, "dist", o.target)
		}
	}
	out, err := filepath.Abs(o.out)
	if err != nil {
		return err
	}
	public := filepath.Join(app, "public")
	if out == app {
		return errors.New("the output folder cannot be the app folder")
	}
	if out == public || strings.HasPrefix(out, public+string(filepath.Separator)) {
		return errors.New("the output folder cannot be inside public")
	}
	if o.title == "" {
		o.title = filepath.Base(app)
	}

	pkg, err := goOutput(app, "list", "-f", "{{.Name}}", ".")
	if err != nil {
		return err
	}
	if pkg != "main" {
		return fmt.Errorf("%s is package %s, not package main", app, pkg)
	}
	if o.target == "windows" {
		return buildWindows(app, out, o)
	}
	if o.target == "android" {
		return buildAndroid(app, out, o)
	}
	// Resolved per app so the JS always matches the wire version the app was built against.
	quiDir, err := goOutput(app, "list", "-m", "-f", "{{.Dir}}", quiModule)
	if err != nil {
		return err
	}
	goRoot, err := goOutput(app, "env", "GOROOT")
	if err != nil {
		return err
	}

	wasm, err := buildWasm(app)
	if err != nil {
		return err
	}
	wasmName := hashedName(".wasm", wasm)
	script, err := bundle(wasmName, goRoot, quiDir, o)
	if err != nil {
		return err
	}
	scriptName := hashedName(".js", script)
	page, err := renderPage(app, scriptName, o)
	if err != nil {
		return err
	}

	artifacts := []*artifact{{name: "index.html", data: page}, {name: scriptName, data: script}, {name: wasmName, data: wasm}}
	if o.compress {
		if err := compressAll(artifacts); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := removeGenerated(out); err != nil {
		return err
	}
	if err := copyPublic(public, out); err != nil {
		return err
	}
	if err := writeArtifacts(out, artifacts); err != nil {
		return err
	}
	return report(out, artifacts, o.compress)
}

func goOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("go %s: %s", args[0], message)
	}
	return strings.TrimSpace(string(output)), nil
}

func buildWasm(app string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "qui-build")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "app.wasm")
	if _, err := goOutput(app, "build", "-trimpath", "-ldflags=-s -w", "-o", path, "."); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func hashedName(extension string, data []byte) string {
	sum := sha256.Sum256(data)
	return "app." + hex.EncodeToString(sum[:4]) + extension
}

func bundle(wasmName, goRoot, quiDir string, o options) ([]byte, error) {
	startOptions := struct {
		Sans        string   `json:"sans,omitempty"`
		Mono        string   `json:"mono,omitempty"`
		PreventKeys []string `json:"preventKeys,omitempty"`
	}{Sans: o.sans, Mono: o.mono}
	for _, key := range strings.Split(o.preventKeys, ",") {
		if key = strings.TrimSpace(key); key != "" {
			startOptions.PreventKeys = append(startOptions.PreventKeys, key)
		}
	}
	extra, err := json.Marshal(startOptions)
	if err != nil {
		return nil, err
	}
	boot := fmt.Sprintf(`import %q;
import { start } from %q;
start({ wasm: new URL(%q, import.meta.url).href, root: document.body, ...%s });
`, filepath.Join(goRoot, "lib", "wasm", "wasm_exec.js"), filepath.Join(quiDir, "web", "qui.js"), wasmName, extra)

	result := api.Build(api.BuildOptions{
		Stdin:             &api.StdinOptions{Contents: boot, ResolveDir: quiDir, Loader: api.LoaderJS},
		Bundle:            true,
		Format:            api.FormatESModule,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		Outfile:           "app.js",
	})
	if len(result.Errors) > 0 {
		messages := api.FormatMessages(result.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage})
		return nil, errors.New(strings.Join(messages, ""))
	}
	return result.OutputFiles[0].Contents, nil
}

func renderPage(app, scriptName string, o options) ([]byte, error) {
	tag := `<script type="module" src="./` + scriptName + `"></script>`
	custom, err := os.ReadFile(filepath.Join(app, "index.html"))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Appendf(nil, defaultPage, html.EscapeString(o.title), o.bg, tag), nil
	}
	if err != nil {
		return nil, err
	}
	at := len(custom)
	if match := closingTag.FindIndex(custom); match != nil {
		at = match[0]
	}
	return slices.Concat(custom[:at], []byte(tag+"\n"), custom[at:]), nil
}

func compressAll(artifacts []*artifact) error {
	var group sync.WaitGroup
	errs := make([]error, len(artifacts))
	for i, a := range artifacts {
		group.Add(1)
		go func() {
			defer group.Done()
			a.gz, a.br, errs[i] = compress(a.data)
		}()
	}
	group.Wait()
	return errors.Join(errs...)
}

func compress(data []byte) (gz, br []byte, err error) {
	var gzBuffer, brBuffer bytes.Buffer
	gzWriter, err := gzip.NewWriterLevel(&gzBuffer, gzip.BestCompression)
	if err != nil {
		return nil, nil, err
	}
	if _, err := gzWriter.Write(data); err != nil {
		return nil, nil, err
	}
	if err := gzWriter.Close(); err != nil {
		return nil, nil, err
	}
	brWriter := brotli.NewWriterLevel(&brBuffer, brotli.BestCompression)
	if _, err := brWriter.Write(data); err != nil {
		return nil, nil, err
	}
	if err := brWriter.Close(); err != nil {
		return nil, nil, err
	}
	return gzBuffer.Bytes(), brBuffer.Bytes(), nil
}

// Only files this command wrote are removed, so pointing -o at a shared folder is safe.
func removeGenerated(out string) error {
	entries, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && generated.MatchString(entry.Name()) {
			if err := os.Remove(filepath.Join(out, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyPublic(public, out string) error {
	if _, err := os.Stat(public); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(public, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(public, path)
		if err != nil {
			return err
		}
		target := filepath.Join(out, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func writeArtifacts(out string, artifacts []*artifact) error {
	for _, a := range artifacts {
		files := map[string][]byte{a.name: a.data}
		if a.gz != nil {
			files[a.name+".gz"] = a.gz
			files[a.name+".br"] = a.br
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func report(out string, artifacts []*artifact, compressed bool) error {
	fmt.Println(out)
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, a := range artifacts {
		fmt.Fprintf(table, "  %s\t%s", a.name, formatSize(len(a.data)))
		if compressed {
			fmt.Fprintf(table, "\tgzip %s\tbrotli %s", formatSize(len(a.gz)), formatSize(len(a.br)))
		}
		fmt.Fprintln(table)
	}
	return table.Flush()
}

func formatSize(n int) string {
	switch {
	case n >= 1e6:
		return fmt.Sprintf("%.2f MB", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.1f KB", float64(n)/1e3)
	}
	return fmt.Sprintf("%d B", n)
}
