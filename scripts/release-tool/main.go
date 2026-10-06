// release-tool is the repository-pinned, standard-library-only evidence generator.
// It never executes a deliverable or copies raw Go build settings into public output.
package main

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

const project = "github.com/tlfjar/ArcourtDownloader"
const generator = "arcourt-release-tool-1"

type object = map[string]any
type material struct{ Path, SHA256, Text string }
type component struct {
	Name, Version, License, Source, Sum string
	Materials                           []material
}
type catalog struct {
	Components []component
	Embeds     map[string]string
}
type pkg struct {
	ImportPath, Dir string
	Standard        bool
	Module          *debug.Module
	EmbedFiles      []string
}
type binary struct {
	Name, Role, Version, Commit, Release, GoVersion string
	Modules                                         []*debug.Module
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(p string) []byte { b, e := os.ReadFile(p); must(e); return b }
func hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func norm(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }
func command(dir string, args ...string) []byte {
	c := exec.Command("go", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GOWORK=off", "GOOS=windows", "GOARCH=amd64", "GOFLAGS=-mod=readonly", "GOEXPERIMENT=", "CGO_ENABLED=0")
	var stderr bytes.Buffer
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		panic(fmt.Sprintf("go %s failed: %s", strings.Join(args, " "), stderr.String()))
	}
	return b
}
func graph(repo, role string) []pkg {
	dir := repo
	args := []string{"list", "-deps", "-json", "./cmd/arcourt-download"}
	if role == "gui" {
		dir = filepath.Join(repo, "desktop")
		args = []string{"list", "-deps", "-tags=desktop,wv2runtime.error,production", "-json", "."}
	}
	d := json.NewDecoder(bytes.NewReader(command(dir, args...)))
	var out []pkg
	for {
		var p pkg
		e := d.Decode(&p)
		if e == io.EOF {
			break
		}
		must(e)
		out = append(out, p)
	}
	return out
}

var marker = regexp.MustCompile(`ARCOURT_BUILD_V1\|([^|\x00]+)\|([^|\x00]+)\|(true|false)\|(cli|gui)\|END_ARCOURT_BUILD`)

func inspect(path, role, version, commit, release string) binary {
	b := read(path)
	matches := marker.FindAllSubmatch(b, -1)
	if len(matches) != 1 {
		panic("Missing or ambiguous build identity: " + filepath.Base(path))
	}
	m := matches[0]
	out := binary{filepath.Base(path), string(m[4]), string(m[1]), string(m[2]), string(m[3]), "", nil}
	if out.Role != role || out.Version != version || out.Commit != commit || out.Release != release {
		panic("Wrong version/commit/release/role: " + out.Name)
	}
	i, e := buildinfo.ReadFile(path)
	must(e)
	out.GoVersion = i.GoVersion
	out.Modules = i.Deps
	expected := project + "/cmd/arcourt-download"
	if role == "gui" {
		expected = project + "/desktop"
	}
	if i.Path != expected {
		panic("Wrong executable entry point")
	}
	settings := map[string]string{}
	for _, s := range i.Settings {
		settings[s.Key] = s.Value
	}
	for k, v := range map[string]string{"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0", "-trimpath": "true", "-buildmode": "exe"} {
		if settings[k] != v {
			panic("Unapproved build setting: " + k)
		}
	}
	tags := settings["-tags"]
	if role == "gui" && tags != "desktop,wv2runtime.error,production" {
		panic("Non-production/fixture GUI tags")
	}
	if role == "cli" && tags != "" {
		panic("Unapproved CLI tags")
	}
	if settings["GOEXPERIMENT"] != "" {
		panic("Unreviewed Go experiment")
	}
	if revision := settings["vcs.revision"]; release == "true" && revision != "" && (revision != commit || settings["vcs.modified"] != "false") {
		panic("Wrong/dirty VCS identity")
	}
	return out
}
func id(s string) string         { return "SPDXRef-" + hash([]byte(s))[:24] }
func checksum(b []byte) []object { return []object{{"algorithm": "SHA256", "checksumValue": hash(b)}} }
func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, r)
			os.Exit(1)
		}
	}()
	mode := flag.String("mode", "generate", "generate, notices, inventory, or inspect")
	repo := flag.String("repo", ".", "source root")
	bins := flag.String("binaries", "", "binaries directory")
	version := flag.String("version", "development", "expected version")
	commit := flag.String("commit", "", "expected commit")
	release := flag.String("release", "false", "expected release flag")
	out := flag.String("out", "", "output file")
	flag.Parse()
	if *mode == "inspect" {
		var all []binary
		for _, r := range []string{"cli", "gui"} {
			n := "arcourt-download.exe"
			if r == "gui" {
				n = "ArcourtDownloader.exe"
			}
			all = append(all, inspect(filepath.Join(*bins, n), r, *version, *commit, *release))
		}
		b, e := json.Marshal(all)
		must(e)
		fmt.Println(string(b))
		return
	}
	var cat catalog
	must(json.Unmarshal(read(filepath.Join(*repo, "scripts/licenses/catalog.json")), &cat))
	byKey := map[string]component{}
	var notices strings.Builder
	notices.WriteString("THIRD-PARTY NOTICES\n\nArcourt Downloader's 0BSD license does not relicense the components below.\nTexts are preserved from the reviewed upstream sources; line endings are normalized.\n\n")
	for _, c := range cat.Components {
		key := c.Name + "@" + c.Version
		if c.License == "" || len(c.Materials) == 0 {
			panic("Unresolved license: " + key)
		}
		for _, token := range strings.Fields(c.License) {
			switch token {
			case "AND", "OR", "MIT", "Unlicense", "BSD-1-Clause", "BSD-2-Clause", "BSD-3-Clause", "Apache-2.0", "Unicode-3.0", "SunPro", "LicenseRef-Cephes", "LicenseRef-Rijndael":
			default:
				panic("Unreviewed license expression: " + key)
			}
		}
		if _, exists := byKey[key]; exists {
			panic("Duplicate license component: " + key)
		}
		byKey[key] = c
		fmt.Fprintf(&notices, "============================================================\n%s\nLicense: %s\nSource: %s\n", key, c.License, c.Source)
		for _, m := range c.Materials {
			if hash([]byte(m.Text)) != m.SHA256 || len(m.Text) < 30 {
				panic("License text mismatch: " + key)
			}
			fmt.Fprintf(&notices, "\n--- %s ---\n%s\n", m.Path, m.Text)
		}
	}
	noticeText := strings.TrimRight(notices.String(), "\n") + "\n"
	if *mode == "notices" {
		for _, role := range []string{"cli", "gui"} {
			for _, p := range graph(*repo, role) {
				if p.Module == nil || strings.HasPrefix(p.Module.Path, project) {
					continue
				}
				key := p.Module.Path + "@" + p.Module.Version
				c, ok := byKey[key]
				if !ok || c.Sum != p.Module.Sum || p.Module.Replace != nil {
					panic("Unreviewed notice dependency: " + key)
				}
			}
		}
		must(os.WriteFile(*out, []byte(noticeText), 0644))
		return
	}
	if *mode != "inventory" && norm(read(filepath.Join(*repo, "THIRD_PARTY_NOTICES.txt"))) != noticeText {
		panic("Missing/stale THIRD_PARTY_NOTICES.txt; regenerate and review")
	}
	// Downloaded module content is verified against both go.sum files by the caller.
	// Check every reviewed license excerpt against the resolved upstream file.
	for _, c := range cat.Components {
		base := ""
		if c.Name == "Go standard library" {
			if c.Version != runtime.Version() {
				panic(fmt.Sprintf("Unreviewed Go toolchain: found %s; license catalog requires %s. Set GOTOOLCHAIN=%s before running release-tool.", runtime.Version(), c.Version, c.Version))
			}
			base = runtime.GOROOT()
		}
		if strings.HasPrefix(c.Name, "Wails ") {
			var m struct{ Dir string }
			must(json.Unmarshal(command(*repo, "mod", "download", "-json", "github.com/wailsapp/wails/v2@v2.16.0"), &m))
			base = m.Dir
		}
		if c.Sum != "" {
			var m struct{ Dir, Sum string }
			must(json.Unmarshal(command(*repo, "mod", "download", "-json", c.Name+"@"+c.Version), &m))
			if m.Sum != c.Sum {
				panic("Module checksum mismatch: " + c.Name)
			}
			base = m.Dir
		}
		if base != "" {
			for _, m := range c.Materials {
				// External copied-code licenses are vendored, source-attributed and
				// hash-pinned in the reviewed catalog, not fetched during packaging.
				if strings.HasPrefix(m.Path, "https://") {
					continue
				}
				if !strings.Contains(norm(read(filepath.Join(base, m.Path))), m.Text) {
					panic("Upstream license changed: " + c.Name + "/" + m.Path)
				}
			}
		}
	}
	graphs := map[string][]pkg{}
	embeds := map[string]string{}
	for _, role := range []string{"cli", "gui"} {
		graphs[role] = graph(*repo, role)
		name := "arcourt-download.exe"
		if role == "gui" {
			name = "ArcourtDownloader.exe"
		}
		executable := read(filepath.Join(*bins, name))
		for _, p := range graphs[role] {
			for _, f := range p.EmbedFiles {
				if strings.HasPrefix(p.ImportPath, project) {
					continue
				}
				content := read(filepath.Join(p.Dir, f))
				if bytes.Contains(executable, content) {
					embeds[p.ImportPath+"/"+f] = hash(content)
				}
			}
		}
	}
	if *mode == "inventory" {
		b, e := json.MarshalIndent(embeds, "", "  ")
		must(e)
		fmt.Println(string(b))
		return
	}
	a, _ := json.Marshal(embeds)
	b, _ := json.Marshal(cat.Embeds)
	if !bytes.Equal(a, b) {
		panic("Embedded upstream assets changed; review catalog")
	}
	var packages, relationships, files []object
	coreID := id(project + "@" + *version)
	packages = append(packages, object{"SPDXID": coreID, "name": project, "versionInfo": *version, "downloadLocation": "NOASSERTION", "filesAnalyzed": false, "licenseDeclared": "0BSD", "licenseConcluded": "0BSD", "copyrightText": "See LICENSE", "sourceInfo": "Local source at commit " + *commit + "; desktop replace resolves to this source, not v0.0.0"})
	used := map[string]bool{}
	for _, role := range []string{"cli", "gui"} {
		name := "arcourt-download.exe"
		if role == "gui" {
			name = "ArcourtDownloader.exe"
		}
		path := filepath.Join(*bins, name)
		bin := inspect(path, role, *version, *commit, *release)
		if bin.GoVersion != runtime.Version() {
			panic("Binary/toolchain mismatch")
		}
		pid := id(name)
		relationships = append(relationships, object{"spdxElementId": pid, "relationshipType": "DEPENDS_ON", "relatedSpdxElement": coreID})
		fid := id("file:" + name)
		packages = append(packages, object{"SPDXID": pid, "name": name, "versionInfo": *version, "downloadLocation": "NOASSERTION", "filesAnalyzed": false, "licenseDeclared": "0BSD", "licenseConcluded": "NOASSERTION", "copyrightText": "See LICENSE", "checksums": checksum(read(path)), "sourceInfo": "Source commit " + *commit + "; windows/amd64; release=" + *release})
		files = append(files, object{"SPDXID": fid, "fileName": "./" + name, "checksums": checksum(read(path)), "licenseConcluded": "NOASSERTION", "copyrightText": "See LICENSE and THIRD_PARTY_NOTICES.txt"})
		relationships = append(relationships, object{"spdxElementId": "SPDXRef-DOCUMENT", "relationshipType": "DESCRIBES", "relatedSpdxElement": pid}, object{"spdxElementId": pid, "relationshipType": "CONTAINS", "relatedSpdxElement": fid})
		expected := map[string]string{}
		for _, p := range graphs[role] {
			if p.Module != nil && !strings.HasPrefix(p.Module.Path, project) {
				expected[p.Module.Path+"@"+p.Module.Version] = p.Module.Sum
			}
		}
		for _, m := range bin.Modules {
			if m.Path == project && role == "gui" {
				if m.Replace == nil || m.Replace.Path != ".." {
					panic("Desktop local replace mismatch")
				}
				continue
			}
			key := m.Path + "@" + m.Version
			c, ok := byKey[key]
			if !ok || c.Sum != m.Sum || expected[key] != m.Sum || m.Replace != nil {
				panic("Unreviewed or wrong binary dependency: " + key)
			}
			delete(expected, key)
			used[key] = true
			relationships = append(relationships, object{"spdxElementId": pid, "relationshipType": "DEPENDS_ON", "relatedSpdxElement": id(key)})
		}
		if len(expected) != 0 {
			panic("Binary dependency graph differs from target source")
		}
		for _, c := range cat.Components {
			if c.Sum != "" {
				continue
			}
			if role == "cli" && c.Name != "Go standard library" && c.Name != "Unicode data" {
				continue
			}
			key := c.Name + "@" + c.Version
			used[key] = true
			relationships = append(relationships, object{"spdxElementId": pid, "relationshipType": "CONTAINS", "relatedSpdxElement": id(key)})
		}
		if role == "gui" {
			own := map[string]bool{"frontend/dist/app.js": true, "frontend/dist/state.js": true, "frontend/dist/index.html": true, "frontend/dist/style.css": true}
			for _, p := range graphs[role] {
				if p.ImportPath != project+"/desktop" {
					continue
				}
				for _, f := range p.EmbedFiles {
					if !own[f] {
						panic("Unexpected embedded application asset: " + f)
					}
					delete(own, f)
					content := read(filepath.Join(p.Dir, f))
					if !bytes.Equal(content, read(filepath.Join(*repo, "desktop/frontend/src", filepath.Base(f)))) || !bytes.Contains(read(path), content) {
						panic("Stale frontend asset")
					}
					eid := id(f)
					files = append(files, object{"SPDXID": eid, "fileName": "./embedded/" + f, "checksums": checksum(content), "licenseConcluded": "0BSD", "copyrightText": "See LICENSE"})
					relationships = append(relationships, object{"spdxElementId": pid, "relationshipType": "CONTAINS", "relatedSpdxElement": eid})
				}
			}
			if len(own) != 0 {
				panic("Missing embedded frontend")
			}
		}
	}
	for _, c := range cat.Components {
		key := c.Name + "@" + c.Version
		if !used[key] {
			panic("Catalog contains unshipped component: " + key)
		}
		p := object{"SPDXID": id(key), "name": c.Name, "versionInfo": c.Version, "downloadLocation": c.Source, "filesAnalyzed": false, "licenseDeclared": c.License, "licenseConcluded": c.License, "copyrightText": "See THIRD_PARTY_NOTICES.txt: " + key}
		if c.Sum != "" {
			p["externalRefs"] = []object{{"referenceCategory": "PACKAGE-MANAGER", "referenceType": "purl", "referenceLocator": "pkg:golang/" + c.Name + "@" + c.Version}}
			p["sourceInfo"] = "Go module content checksum " + c.Sum
		}
		packages = append(packages, p)
	}
	keys := make([]string, 0, len(embeds))
	for k := range embeds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		eid := id(k)
		files = append(files, object{"SPDXID": eid, "fileName": "./embedded/" + k, "checksums": []object{{"algorithm": "SHA256", "checksumValue": embeds[k]}}, "licenseConcluded": "MIT", "copyrightText": "See THIRD_PARTY_NOTICES.txt"})
		relationships = append(relationships, object{"spdxElementId": id("ArcourtDownloader.exe"), "relationshipType": "CONTAINS", "relatedSpdxElement": eid})
	}
	// Fixed timestamp and a namespace derived from content keep evidence reproducible.
	seed, _ := json.Marshal(object{"packages": packages, "files": files, "commit": *commit, "version": *version})
	doc := object{"spdxVersion": "SPDX-2.3", "dataLicense": "CC0-1.0", "SPDXID": "SPDXRef-DOCUMENT", "name": "ArcourtDownloader-" + *version + "-windows-amd64", "documentNamespace": "https://spdx.org/spdxdocs/ArcourtDownloader-" + hash(seed), "creationInfo": object{"created": "2026-01-01T00:00:00Z", "creators": []string{"Tool: " + generator}, "comment": "Reproducible metadata epoch; not a build/signing timestamp. Generator source is pinned by the source commit."}, "packages": packages, "files": files, "relationships": relationships}
	var extracted []object
	for _, c := range cat.Components {
		if c.Name == "Go standard library" {
			for _, m := range c.Materials {
				licenseID := ""
				if m.Path == "src/math/atan.go" {
					licenseID = "LicenseRef-Cephes"
				}
				if m.Path == "src/crypto/internal/fips140/aes/aes_generic.go" {
					licenseID = "LicenseRef-Rijndael"
				}
				if licenseID != "" {
					extracted = append(extracted, object{"licenseId": licenseID, "extractedText": m.Text, "seeAlsos": []string{"https://github.com/golang/go/blob/go1.26.8/" + m.Path}})
				}
			}
		}
	}
	doc["hasExtractedLicensingInfos"] = extracted
	b, e := json.MarshalIndent(doc, "", "  ")
	must(e)
	must(os.WriteFile(*out, append(b, '\n'), 0644))
}
