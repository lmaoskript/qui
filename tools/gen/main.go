package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type entry struct {
	name  string
	value int
}

type spec struct {
	version int
	groups  map[string][]entry
	themes  []string
	values  map[string]map[string][4]float64
}

func parse(path string) spec {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	s := spec{groups: map[string][]entry{}, values: map[string]map[string][4]float64{}}
	scan := bufio.NewScanner(f)
	for line := 1; scan.Scan(); line++ {
		text := strings.TrimSpace(scan.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		switch fields[0] {
		case "version":
			s.version, _ = strconv.Atoi(fields[1])
		case "theme":
			if len(fields) != 7 {
				log.Fatalf("spec.txt:%d: theme needs a name, token and four channels", line)
			}
			if _, ok := s.values[fields[1]]; !ok {
				s.themes = append(s.themes, fields[1])
				s.values[fields[1]] = map[string][4]float64{}
			}
			var c [4]float64
			for i := range c {
				c[i], err = strconv.ParseFloat(fields[3+i], 64)
				if err != nil {
					log.Fatalf("spec.txt:%d: %v", line, err)
				}
			}
			s.values[fields[1]][fields[2]] = c
		default:
			if len(fields) != 3 {
				log.Fatalf("spec.txt:%d: expected `group name value`", line)
			}
			v, err := strconv.Atoi(fields[2])
			if err != nil {
				log.Fatalf("spec.txt:%d: %v", line, err)
			}
			s.groups[fields[0]] = append(s.groups[fields[0]], entry{fields[1], v})
		}
	}
	for _, theme := range s.themes {
		for _, token := range s.groups["token"] {
			if _, ok := s.values[theme][token.name]; !ok {
				log.Fatalf("theme %s is missing token %s", theme, token.name)
			}
		}
	}
	return s
}

func pack(c [4]float64) uint32 {
	b := func(v float64) uint32 { return uint32(math.Round(v * 255)) }
	return b(c[0])<<24 | b(c[1])<<16 | b(c[2])<<8 | b(c[3])
}

var cppEnums = []struct{ group, typeName, base, prefix string }{
	{"op", "Op", "uint8_t", "Op"},
	{"kind", "Kind", "uint8_t", "Kind"},
	{"dir", "Dir", "uint8_t", "Dir"},
	{"size", "SizeMode", "uint8_t", "Size"},
	{"flag", "Flag", "uint32_t", "Flag"},
	{"token", "Token", "uint8_t", "Token"},
}

func cpp(s spec) string {
	var b strings.Builder
	b.WriteString("#pragma once\n\n#include <cstdint>\n\nnamespace qui {\n\nnamespace wire {\n\n")
	fmt.Fprintf(&b, "inline constexpr uint8_t kVersion = %d;\n", s.version)
	fmt.Fprintf(&b, "inline constexpr int kThemeCount = %d;\n", len(s.themes))
	fmt.Fprintf(&b, "inline constexpr int kTokenCount = %d;\n\n", len(s.groups["token"]))
	for _, e := range cppEnums {
		fmt.Fprintf(&b, "enum %s : %s {\n", e.typeName, e.base)
		for _, v := range s.groups[e.group] {
			fmt.Fprintf(&b, "  %s%s = %d,\n", e.prefix, v.name, v.value)
		}
		b.WriteString("};\n\n")
	}
	fmt.Fprintf(&b, "inline constexpr uint32_t kThemes[kThemeCount][kTokenCount + 1] = {\n")
	for _, theme := range s.themes {
		b.WriteString("    {0")
		for _, token := range s.groups["token"] {
			fmt.Fprintf(&b, ", 0x%08x", pack(s.values[theme][token.name]))
		}
		b.WriteString("},\n")
	}
	b.WriteString("};\n\n}  // namespace wire\n\n")
	for _, v := range s.groups["align"] {
		fmt.Fprintf(&b, "inline constexpr uint8_t %s = %d;\n", v.name, v.value)
	}
	b.WriteString("\n")
	for _, v := range s.groups["role"] {
		fmt.Fprintf(&b, "inline constexpr uint8_t Role%s = %d;\n", v.name, v.value)
	}
	b.WriteString("\n")
	for i, theme := range s.themes {
		fmt.Fprintf(&b, "inline constexpr uint8_t %s = %d;\n", theme, i)
	}
	b.WriteString("\n")
	for _, v := range s.groups["token"] {
		fmt.Fprintf(&b, "inline constexpr uint32_t %s = %du << 24;\n", v.name, v.value)
	}
	b.WriteString("\n}  // namespace qui\n")
	return b.String()
}

func goSource(s spec) string {
	var b strings.Builder
	b.WriteString("package qui\n\n")
	fmt.Fprintf(&b, "const wireVersion = %d\n\n", s.version)
	for _, group := range []string{"op", "kind", "dir", "size", "flag"} {
		b.WriteString("const (\n")
		for _, v := range s.groups[group] {
			fmt.Fprintf(&b, "\t%s%s = %d\n", group, v.name, v.value)
		}
		b.WriteString(")\n\n")
	}
	b.WriteString("const (\n")
	for _, v := range s.groups["align"] {
		fmt.Fprintf(&b, "\t%s Align = %d\n", v.name, v.value)
	}
	b.WriteString(")\n\nconst (\n")
	for _, v := range s.groups["role"] {
		fmt.Fprintf(&b, "\tRole%s Role = %d\n", v.name, v.value)
	}
	b.WriteString(")\n\nconst (\n")
	for i, theme := range s.themes {
		fmt.Fprintf(&b, "\t%s Theme = %d\n", theme, i)
	}
	b.WriteString(")\n\nconst (\n")
	for _, v := range s.groups["token"] {
		fmt.Fprintf(&b, "\t%s Color = %d << 24\n", v.name, v.value)
	}
	b.WriteString(")\n")
	return b.String()
}

func js(s spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "export const Version = %d;\n\n", s.version)
	for _, e := range []struct{ group, name string }{
		{"op", "Op"}, {"kind", "Kind"}, {"dir", "Dir"}, {"size", "Size"},
		{"align", "Align"}, {"role", "Role"}, {"flag", "Flag"},
	} {
		fmt.Fprintf(&b, "export const %s = {", e.name)
		for i, v := range s.groups[e.group] {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s: %d", v.name, v.value)
		}
		b.WriteString("};\n")
	}
	b.WriteString("\nexport const Tokens = [\"\"")
	for _, v := range s.groups["token"] {
		fmt.Fprintf(&b, ", %q", v.name)
	}
	b.WriteString("];\n\nexport const Themes = [\n")
	for _, theme := range s.themes {
		b.WriteString("  {")
		for i, token := range s.groups["token"] {
			if i > 0 {
				b.WriteString(", ")
			}
			c := s.values[theme][token.name]
			fmt.Fprintf(&b, "%s: \"rgba(%d,%d,%d,%.3g)\"", token.name,
				int(math.Round(c[0]*255)), int(math.Round(c[1]*255)), int(math.Round(c[2]*255)), c[3])
		}
		b.WriteString("},\n")
	}
	b.WriteString("];\n")
	return b.String()
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	s := parse(filepath.Join(root, "spec.txt"))
	files := map[string]string{
		"wire.gen.hpp":    cpp(s),
		"wire_gen.go":     goSource(s),
		"web/wire.gen.js": js(s),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
