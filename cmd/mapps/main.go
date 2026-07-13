// Command mapps sets up and manages a multi-repo workspace: it clones
// repos listed in repos.list into apps/ and generates a root Makefile.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rus-lan/multiApps/internal/workspace"
)

const usage = `Usage:
  mapps init [<url>...]            set up / update the workspace
  mapps add <url> [dir] [branch]   add one repo and clone it
  mapps rm <name> [--force]        remove one repo (dir name under apps/)
  mapps version | -v | --version   print the version
  mapps help | -h | --help         print this message
`

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches on args (os.Args without the program name) and returns
// the process exit code. Kept as a pure function so cmd dispatch can be
// tested without touching the filesystem.
func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return 0

	case "init":
		return runInit(args[1:])

	case "add":
		return runAdd(args[1:])

	case "rm":
		return runRm(args[1:])

	case "version", "-v", "--version":
		fmt.Println(version)
		return 0

	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
}

func runInit(urls []string) int {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := workspace.Init(root, urls); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runAdd(args []string) int {
	if len(args) == 0 || len(args) > 3 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	url := args[0]
	var dir, branch string
	if len(args) >= 2 {
		dir = args[1]
	}
	if len(args) == 3 {
		branch = args[2]
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := workspace.Add(root, url, dir, branch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runRm(args []string) int {
	var name string
	force := false
	for _, arg := range args {
		switch {
		case arg == "--force":
			force = true
		case strings.HasPrefix(arg, "-"):
			fmt.Fprint(os.Stderr, usage)
			return 2
		case name != "":
			fmt.Fprint(os.Stderr, usage)
			return 2
		default:
			name = arg
		}
	}
	if name == "" {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := workspace.Remove(root, name, force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
