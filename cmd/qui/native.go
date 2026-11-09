package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type source struct {
	url, sha256, dir string
}

var (
	llvmMingw = source{
		"https://github.com/mstorsjo/llvm-mingw/releases/download/20261104/llvm-mingw-20261104-ucrt-ubuntu-22.04-x86_64.tar.xz",
		"919380403de87dca3c581c2053230cd298af5e891286628ba0613b7fed370926",
		"llvm-mingw-20261104-ucrt-ubuntu-22.04-x86_64",
	}
	sdlSource = source{
		"https://github.com/libsdl-org/SDL/releases/download/release-3.4.20/SDL3-3.4.20.tar.gz",
		"f3bb06f55bed9557227a700af2ea62e49497ebdf58037732a731a5fb3ef8581f",
		"SDL3-3.4.20",
	}
	freetypeSource = source{
		"https://download.savannah.gnu.org/releases/freetype/freetype-2.14.1.tar.xz",
		"32427e8c471ac095853212a37aef816c60b42052d4d9e48230bab3bdf2936ccc",
		"freetype-2.14.1",
	}
	harfbuzzSource = source{
		"https://github.com/harfbuzz/harfbuzz/releases/download/15.1.0/harfbuzz-15.1.0.tar.xz",
		"0fc7c565443a5f4be7da65ac5ee5bd737d9b158dc908708b7689a0a5796b0c8d",
		"harfbuzz-15.1.0",
	}
)

// toolchain describes one cross-compile target.
type toolchain struct {
	name             string
	goos, goarch     string
	cc, cxx, ar      string
	cmake            []string
	prefix           string
	hbFlags          []string
	sdl, ftExtra     []string
	beforeFreetype   func(*toolchain) error
	pkgConfigLibsDir string
}

func cacheRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, "qui")
	return root, os.MkdirAll(root, 0o755)
}

func say(format string, args ...any) { fmt.Fprintf(os.Stderr, "qui: "+format+"\n", args...) }

func run(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", filepath.Base(name), strings.Join(args, " "), err, tail(output.String(), 4000))
	}
	return nil
}

func tail(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return "...\n" + text[len(text)-limit:]
}

func download(url, path, want string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, response.Status)
	}
	part := path + ".part"
	file, err := os.Create(part)
	if err != nil {
		return err
	}
	defer os.Remove(part)
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), response.Body); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		return fmt.Errorf("%s: sha256 is %s, expected %s", url, got, want)
	}
	return os.Rename(part, path)
}

// fetch unpacks a pinned archive into the cache and returns its folder.
func fetch(s source, folder string) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	base := filepath.Join(root, folder)
	dir := filepath.Join(base, s.dir)
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(base, filepath.Base(s.url))
	say("downloading %s", filepath.Base(s.url))
	if err := download(s.url, archive, s.sha256); err != nil {
		return "", err
	}
	defer os.Remove(archive)
	if err := run(base, nil, "tar", "xf", archive); err != nil {
		return "", err
	}
	return dir, nil
}

func (t *toolchain) cmakeBuild(sourceDir string, options ...string) error {
	build, err := os.MkdirTemp("", "qui-cmake")
	if err != nil {
		return err
	}
	defer os.RemoveAll(build)
	args := append([]string{"-S", sourceDir, "-B", build, "-DCMAKE_BUILD_TYPE=Release", "-DCMAKE_INSTALL_PREFIX=" + t.prefix,
		"-DCMAKE_INSTALL_LIBDIR=lib", "-DCMAKE_POLICY_VERSION_MINIMUM=3.5", "-DCMAKE_PREFIX_PATH=" + t.prefix}, t.cmake...)
	if err := run("", nil, "cmake", append(args, options...)...); err != nil {
		return err
	}
	if err := run("", nil, "cmake", "--build", build, "--parallel", strconv.Itoa(runtime.NumCPU())); err != nil {
		return err
	}
	return run("", nil, "cmake", "--install", build)
}

func (t *toolchain) buildHarfbuzz() error {
	dir, err := fetch(harfbuzzSource, "src")
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "qui-harfbuzz")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	var objects []string
	for _, unit := range []string{"harfbuzz", "hb-raster-draw", "hb-raster-image", "hb-raster-paint"} {
		object := filepath.Join(work, unit+".o")
		objects = append(objects, object)
		args := append([]string{"-c", filepath.Join(dir, "src", unit+".cc"), "-o", object, "-O2", "-std=c++17", "-fno-exceptions", "-fno-rtti",
			"-DHAVE_FREETYPE=1", "-DHAVE_FT_GET_VAR_BLEND_COORDINATES=1", "-DHAVE_FT_SET_VAR_BLEND_COORDINATES=1", "-DHAVE_FT_DONE_MM_VAR=1", "-DHAVE_FT_GET_TRANSFORM=1",
			"-I" + filepath.Join(t.prefix, "include", "freetype2")}, t.hbFlags...)
		if err := run("", nil, t.cxx, args...); err != nil {
			return err
		}
	}
	lib := filepath.Join(t.prefix, "lib")
	if err := os.MkdirAll(filepath.Join(lib, "pkgconfig"), 0o755); err != nil {
		return err
	}
	archive := filepath.Join(lib, "libharfbuzz.a")
	os.Remove(archive)
	if err := run("", nil, t.ar, append([]string{"rcs", archive}, objects...)...); err != nil {
		return err
	}
	include := filepath.Join(t.prefix, "include", "harfbuzz")
	if err := os.MkdirAll(include, 0o755); err != nil {
		return err
	}
	headers, err := filepath.Glob(filepath.Join(dir, "src", "hb*.h"))
	if err != nil {
		return err
	}
	for _, header := range headers {
		data, err := os.ReadFile(header)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(include, filepath.Base(header)), data, 0o644); err != nil {
			return err
		}
	}
	pc := fmt.Sprintf("prefix=%s\nName: harfbuzz\nDescription: HarfBuzz\nVersion: 15.1.0\nRequires: freetype2\nLibs: -L${prefix}/lib -lharfbuzz\nCflags: -I${prefix}/include/harfbuzz\n", t.prefix)
	return os.WriteFile(filepath.Join(lib, "pkgconfig", "harfbuzz.pc"), []byte(pc), 0o644)
}

const depsStamp = "SDL3-3.4.20 freetype-2.14.1 harfbuzz-15.1.0 libpng-1.6.50 v2"

// prepare builds the C libraries for a target once and caches them.
func (t *toolchain) prepare() error {
	root, err := cacheRoot()
	if err != nil {
		return err
	}
	t.prefix = filepath.Join(root, "deps", t.name)
	stamp := filepath.Join(t.prefix, ".stamp")
	if data, err := os.ReadFile(stamp); err == nil && string(data) == depsStamp {
		return nil
	}
	if err := os.RemoveAll(t.prefix); err != nil {
		return err
	}
	if err := os.MkdirAll(t.prefix, 0o755); err != nil {
		return err
	}
	say("building SDL, FreeType and HarfBuzz for %s (only needed once)", t.name)
	if t.beforeFreetype != nil {
		if err := t.beforeFreetype(t); err != nil {
			return err
		}
	}
	freetype, err := fetch(freetypeSource, "src")
	if err != nil {
		return err
	}
	if err := t.cmakeBuild(freetype, append([]string{"-DBUILD_SHARED_LIBS=OFF", "-DFT_DISABLE_BZIP2=ON", "-DFT_DISABLE_BROTLI=ON", "-DFT_DISABLE_HARFBUZZ=ON", "-DFT_DISABLE_ZLIB=ON"}, t.ftExtra...)...); err != nil {
		return err
	}
	if err := t.buildHarfbuzz(); err != nil {
		return err
	}
	sdl, err := fetch(sdlSource, "src")
	if err != nil {
		return err
	}
	if err := t.cmakeBuild(sdl, append([]string{"-DSDL_TEST_LIBRARY=OFF", "-DSDL_TESTS=OFF", "-DSDL_EXAMPLES=OFF"}, t.sdl...)...); err != nil {
		return err
	}
	return os.WriteFile(stamp, []byte(depsStamp), 0o644)
}

func (t *toolchain) goEnv() []string {
	pkgConfig := filepath.Join(t.prefix, "lib", "pkgconfig")
	return []string{"CGO_ENABLED=1", "GOOS=" + t.goos, "GOARCH=" + t.goarch, "CC=" + t.cc, "CXX=" + t.cxx,
		"PKG_CONFIG_LIBDIR=" + pkgConfig, "PKG_CONFIG_PATH=", "CGO_LDFLAGS_ALLOW=-mwindows|-mconsole"}
}

func findProgram(names ...string) string {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func mingwToolchain(arch string) (*toolchain, error) {
	triples := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}
	machine, ok := triples[arch]
	if !ok {
		return nil, fmt.Errorf("windows builds support amd64 and arm64, not %s", arch)
	}
	triple := machine + "-w64-mingw32"
	cc, cxx, ar := findProgram(triple+"-clang"), findProgram(triple+"-clang++"), findProgram(triple+"-ar")
	if cc == "" || cxx == "" {
		cc, cxx = findProgram(triple+"-gcc"), findProgram(triple+"-g++")
	}
	if cc == "" || cxx == "" {
		if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
			return nil, fmt.Errorf("no %s-clang or %s-gcc on PATH", triple, triple)
		}
		dir, err := fetch(llvmMingw, "toolchains")
		if err != nil {
			return nil, err
		}
		bin := filepath.Join(dir, "bin", triple)
		cc, cxx, ar = bin+"-clang", bin+"-clang++", bin+"-ar"
	}
	if ar == "" {
		return nil, fmt.Errorf("no %s-ar on PATH", triple)
	}
	t := &toolchain{name: "windows-" + arch, goos: "windows", goarch: arch, cc: cc, cxx: cxx, ar: ar, ftExtra: []string{"-DFT_DISABLE_PNG=ON"},
		sdl: []string{"-DSDL_SHARED=OFF", "-DSDL_STATIC=ON", "-DSDL_AUDIO=OFF", "-DSDL_CAMERA=OFF", "-DSDL_JOYSTICK=OFF", "-DSDL_HAPTIC=OFF",
			"-DSDL_SENSOR=OFF", "-DSDL_HIDAPI=OFF", "-DSDL_POWER=OFF", "-DSDL_GPU=OFF", "-DSDL_VULKAN=OFF", "-DSDL_RENDER=OFF"}}
	root, err := cacheRoot()
	if err != nil {
		return nil, err
	}
	file := filepath.Join(root, "toolchains", t.name+".cmake")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	content := fmt.Sprintf(`set(CMAKE_SYSTEM_NAME Windows)
set(CMAKE_SYSTEM_PROCESSOR %s)
set(CMAKE_C_COMPILER %q)
set(CMAKE_CXX_COMPILER %q)
set(CMAKE_RC_COMPILER %q)
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
`, machine, cc, cxx, strings.TrimSuffix(strings.TrimSuffix(cc, "-clang"), "-gcc")+"-windres")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return nil, err
	}
	t.cmake = []string{"-DCMAKE_TOOLCHAIN_FILE=" + file}
	return t, nil
}

func buildWindows(app, out string, o options) error {
	arch := o.arch
	if arch == "" {
		arch = "amd64"
	}
	if strings.Contains(arch, ",") {
		return errors.New("windows builds take one -arch")
	}
	t, err := mingwToolchain(arch)
	if err != nil {
		return err
	}
	if err := t.prepare(); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	ldflags := "-s -w"
	if !o.console {
		ldflags += " -H=windowsgui"
	}
	path := filepath.Join(out, filepath.Base(app)+".exe")
	say("compiling %s", filepath.Base(path))
	if err := run(app, t.goEnv(), "go", "build", "-trimpath", "-ldflags="+ldflags, "-o", path, "."); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Printf("%s (%.1f MB)\n", path, float64(info.Size())/1e6)
	return nil
}
