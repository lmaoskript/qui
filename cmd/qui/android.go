package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	//go:embed android/QuiActivity.java
	quiActivity []byte
	//go:embed android/AndroidManifest.xml
	androidManifest string
)

var libpngSource = source{
	"https://github.com/pnggroup/libpng/archive/refs/tags/v1.6.50.tar.gz",
	"71158e53cfdf2877bc99bcab33641d78df3f48e6e0daad030afe9cb8c031aa46",
	"libpng-1.6.50",
}

var androidABIs = map[string]struct{ abi, triple string }{
	"arm64": {"arm64-v8a", "aarch64-linux-android"},
	"amd64": {"x86_64", "x86_64-linux-android"},
}

const androidAPI = "24"

type androidSDK struct {
	root, ndk, buildTools, platform string
}

func newestChild(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return ""
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	slices.Sort(names)
	return filepath.Join(dir, names[len(names)-1])
}

func findAndroidSDK() (*androidSDK, error) {
	home, _ := os.UserHomeDir()
	var roots []string
	for _, name := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if value := os.Getenv(name); value != "" {
			roots = append(roots, value)
		}
	}
	roots = append(roots, filepath.Join(home, "Android", "Sdk"), filepath.Join(home, "Android"))
	for _, root := range roots {
		sdk := &androidSDK{root: root, ndk: os.Getenv("ANDROID_NDK_HOME"), buildTools: newestChild(filepath.Join(root, "build-tools")), platform: newestChild(filepath.Join(root, "platforms"))}
		if sdk.ndk == "" {
			sdk.ndk = newestChild(filepath.Join(root, "ndk"))
		}
		if sdk.ndk != "" && sdk.buildTools != "" && sdk.platform != "" {
			return sdk, nil
		}
	}
	return nil, errors.New("no Android SDK found: set ANDROID_HOME, and install an NDK, build-tools and a platform with sdkmanager")
}

func ndkToolchain(sdk *androidSDK, arch string) (*toolchain, error) {
	target, ok := androidABIs[arch]
	if !ok {
		return nil, fmt.Errorf("android builds support arm64 and amd64, not %s", arch)
	}
	bin := filepath.Join(sdk.ndk, "toolchains", "llvm", "prebuilt", "linux-x86_64", "bin")
	compiler := filepath.Join(bin, target.triple+androidAPI+"-clang")
	return &toolchain{
		name: "android-" + arch, goos: "android", goarch: arch,
		cc: compiler, cxx: compiler + "++", ar: filepath.Join(bin, "llvm-ar"),
		cmake: []string{"-DCMAKE_TOOLCHAIN_FILE=" + filepath.Join(sdk.ndk, "build", "cmake", "android.toolchain.cmake"),
			"-DANDROID_ABI=" + target.abi, "-DANDROID_PLATFORM=android-" + androidAPI, "-DANDROID_STL=c++_static",
			"-DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON", "-DCMAKE_POSITION_INDEPENDENT_CODE=ON"},
		hbFlags: []string{"-fPIC", "-DHAVE_PTHREAD=1"},
		ftExtra: []string{"-DFT_DISABLE_PNG=OFF", "-DFT_REQUIRE_PNG=ON"},
		sdl:     []string{"-DSDL_SHARED=ON", "-DSDL_STATIC=OFF"},
		beforeFreetype: func(t *toolchain) error {
			png, err := fetch(libpngSource, "src")
			if err != nil {
				return err
			}
			if err := t.cmakeBuild(png, "-DPNG_SHARED=OFF", "-DPNG_STATIC=ON", "-DPNG_TESTS=OFF", "-DPNG_TOOLS=OFF", "-DPNG_HARDWARE_OPTIMIZATIONS=OFF"); err != nil {
				return err
			}
			// The NDK's toolchain file roots every CMake search inside its sysroot.
			t.ftExtra = append(t.ftExtra, "-DPNG_LIBRARY="+filepath.Join(t.prefix, "lib", "libpng16.a"), "-DPNG_PNG_INCLUDE_DIR="+filepath.Join(t.prefix, "include"))
			// The NDK provides zlib without a pkg-config file, and libpng's refers to one.
			dir := filepath.Join(t.prefix, "lib", "pkgconfig")
			return os.WriteFile(filepath.Join(dir, "zlib.pc"), []byte("Name: zlib\nDescription: zlib\nVersion: 1.3\nLibs: -lz\n"), 0o644)
		},
	}, nil
}

var notPackageChars = regexp.MustCompile(`[^a-z0-9]+`)

func androidID(app string, o options) string {
	if o.id != "" {
		return o.id
	}
	name := notPackageChars.ReplaceAllString(strings.ToLower(filepath.Base(app)), "")
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		name = "app" + name
	}
	return "dev.qui." + name
}

func debugKeystore() (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "android-debug.keystore")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return path, run("", nil, "keytool", "-genkeypair", "-keystore", path, "-storepass", "android", "-alias", "qui", "-keypass", "android",
		"-keyalg", "RSA", "-keysize", "2048", "-validity", "10000", "-dname", "CN=qui debug")
}

func compileJava(sdk *androidSDK, work string) (string, error) {
	sdl, err := fetch(sdlSource, "src")
	if err != nil {
		return "", err
	}
	sources, err := filepath.Glob(filepath.Join(sdl, "android-project", "app", "src", "main", "java", "org", "libsdl", "app", "*.java"))
	if err != nil {
		return "", err
	}
	activity := filepath.Join(work, "QuiActivity.java")
	if err := os.WriteFile(activity, quiActivity, 0o644); err != nil {
		return "", err
	}
	classes := filepath.Join(work, "classes")
	androidJar := filepath.Join(sdk.platform, "android.jar")
	args := append([]string{"-nowarn", "-Xlint:none", "--release", "11", "-classpath", androidJar, "-d", classes}, append(sources, activity)...)
	if err := run("", nil, "javac", args...); err != nil {
		return "", err
	}
	var compiled []string
	if err := filepath.WalkDir(classes, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".class") {
			compiled = append(compiled, path)
		}
		return err
	}); err != nil {
		return "", err
	}
	dex := filepath.Join(work, "dex")
	if err := os.MkdirAll(dex, 0o755); err != nil {
		return "", err
	}
	args = append([]string{"--release", "--min-api", androidAPI, "--lib", androidJar, "--output", dex}, compiled...)
	if err := run("", nil, filepath.Join(sdk.buildTools, "d8"), args...); err != nil {
		return "", err
	}
	return filepath.Join(dex, "classes.dex"), nil
}

func addZipFile(writer *zip.Writer, name, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	file, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return err
}

func addNatives(base, out, dex string, libs map[string]string) error {
	input, err := zip.OpenReader(base)
	if err != nil {
		return err
	}
	defer input.Close()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range input.File {
		reader, err := file.Open()
		if err != nil {
			return err
		}
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: file.Name, Method: file.Method})
		if err != nil {
			reader.Close()
			return err
		}
		_, err = io.Copy(entry, reader)
		reader.Close()
		if err != nil {
			return err
		}
	}
	if err := addZipFile(writer, "classes.dex", dex); err != nil {
		return err
	}
	names := make([]string, 0, len(libs))
	for name := range libs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := addZipFile(writer, name, libs[name]); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return os.WriteFile(out, buffer.Bytes(), 0o644)
}

func buildAndroid(app, out string, o options) error {
	sdk, err := findAndroidSDK()
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "qui-android")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	arches := []string{"arm64", "amd64"}
	if o.arch != "" {
		arches = strings.Split(o.arch, ",")
	}
	libs := map[string]string{}
	for _, arch := range arches {
		t, err := ndkToolchain(sdk, arch)
		if err != nil {
			return err
		}
		if err := t.prepare(); err != nil {
			return err
		}
		abi := androidABIs[arch].abi
		say("compiling libmain.so for %s", abi)
		shared := filepath.Join(work, "libmain-"+abi+".so")
		env := append(t.goEnv(), "CGO_LDFLAGS_ALLOW=.*")
		if err := run(app, env, "go", "build", "-buildmode=c-shared", "-trimpath", "-ldflags=-s -w", "-o", shared, "."); err != nil {
			return err
		}
		libs["lib/"+abi+"/libmain.so"] = shared
		libs["lib/"+abi+"/libSDL3.so"] = filepath.Join(t.prefix, "lib", "libSDL3.so")
	}

	say("packaging the apk")
	dex, err := compileJava(sdk, work)
	if err != nil {
		return err
	}
	manifest := strings.NewReplacer("{{.ID}}", androidID(app, o), "{{.Title}}", html.EscapeString(o.title)).Replace(androidManifest)
	manifestPath := filepath.Join(work, "AndroidManifest.xml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		return err
	}
	base := filepath.Join(work, "base.apk")
	if err := run("", nil, filepath.Join(sdk.buildTools, "aapt2"), "link", "-o", base, "-I", filepath.Join(sdk.platform, "android.jar"), "--manifest", manifestPath); err != nil {
		return err
	}
	unaligned := filepath.Join(work, "unaligned.apk")
	if err := addNatives(base, unaligned, dex, libs); err != nil {
		return err
	}
	aligned := filepath.Join(work, "aligned.apk")
	if err := run("", nil, filepath.Join(sdk.buildTools, "zipalign"), "-f", "-p", "4", unaligned, aligned); err != nil {
		return err
	}
	keystore, err := debugKeystore()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	path := filepath.Join(out, filepath.Base(app)+".apk")
	if err := run("", nil, filepath.Join(sdk.buildTools, "apksigner"), "sign", "--ks", keystore, "--ks-pass", "pass:android", "--key-pass", "pass:android", "--out", path, aligned); err != nil {
		return err
	}
	os.Remove(path + ".idsig")
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Printf("%s (%.1f MB, debug-signed, %s)\n", path, float64(info.Size())/1e6, androidID(app, o))
	return nil
}
