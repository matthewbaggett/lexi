package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"git.grey.ooo/grey.ooo/lexi/internal/report"
)

// Replaced in tests.
var (
	getenv = os.Getenv
	// gitDiff returns the patch between two revisions.
	gitDiff = func(root, from, to string) ([]byte, error) {
		return exec.Command("git", "-C", root, "diff-tree", "-p", "-M", "--full-index", "--no-commit-id", from, to, "--").Output()
	}
)

// commitRange returns the two revisions to compare, from the environment of
// Travis CI or GitHub Actions. It returns nil outside CI.
func commitRange() []string {
	if r := getenv("TRAVIS_COMMIT_RANGE"); r != "" {
		return strings.Split(r, "...")
	}
	if sha := getenv("GITHUB_SHA"); sha != "" {
		base, head := getenv("GITHUB_BASE_REF"), getenv("GITHUB_HEAD_REF")
		if base != "" && head != "" {
			return []string{filepath.Base(base), filepath.Base(head)}
		}
		return []string{sha + "^1", sha}
	}
	return nil
}

// gitRoot returns the directory above dir that holds .git, or "".
func gitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

var hunk = regexp.MustCompile(`^@@ -(\d+),?(\d+)? \+(\d+),?(\d+)? @@`)

// addedLines reads the line ranges that a patch adds to each file. Like
// unified-diff, it counts every line of a hunk, so a deleted line moves the
// ranges after it.
func addedLines(patch []byte, root string) map[string][][2]int {
	out := map[string][][2]int{}
	var file string
	start, index := 0, 0
	open := false
	pos := -1
	sc := bufio.NewScanner(strings.NewReader(string(patch)))
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = ""
			if p, ok := strings.CutPrefix(line, "+++ b/"); ok {
				file = filepath.Join(root, p)
			}
			open = false
		case hunk.MatchString(line):
			m := hunk.FindStringSubmatch(line)
			n, _ := strconv.Atoi(m[3])
			start, index, open, pos = n-1, 0, file != "", -1
		case open:
			index++
			if strings.HasPrefix(line, "+") {
				no := start + index
				if pos < 0 {
					pos = len(out[file])
					out[file] = append(out[file], [2]int{no, no})
				} else {
					out[file][pos][1] = no
				}
			} else {
				pos = -1
			}
		}
	}
	return out
}

// diffFilter keeps the messages on lines that the commit range changed. It
// changes nothing outside CI.
func diffFilter(cwd, name string, msgs []report.Message) []report.Message {
	rng := commitRange()
	if len(rng) != 2 || name == "<stdin>" {
		return msgs
	}
	abs := filepath.Join(cwd, name)
	root := gitRoot(filepath.Dir(abs))
	if root == "" {
		return []report.Message{{Line: 1, Column: 1, Fatal: true, Reason: "Not in a git repository"}}
	}
	patch, err := gitDiff(root, rng[0], rng[1])
	if err != nil {
		return []report.Message{{Line: 1, Column: 1, Fatal: true, Reason: err.Error()}}
	}
	ranges := addedLines(patch, root)[abs]
	var out []report.Message
	for _, m := range msgs {
		for _, r := range ranges {
			if m.Line >= r[0] && m.Line <= r[1] {
				out = append(out, m)
				break
			}
		}
	}
	return out
}
