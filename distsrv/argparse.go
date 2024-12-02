package distsrv

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

type Opt struct {
	Name  OptName
	Short string
	Long  string
	Value string
	Dflt  string
}

type OptName string

const ImageOpt OptName = "image"
const DestOpt OptName = "dest"
const OsOpt OptName = "os"
const ArchOpt OptName = "arch"
const NamespaceOpt OptName = "namespace"

type OptMap map[OptName]Opt

var usage = `
Usage:

imgpull <image ref> <tar file> [-o|--os os] [-a|--arch arch] [-n|--ns namespace]

The image ref and tar file are required. OS and arch default to your system's values.

Example:

imgpull localhost:8080/hello-world:latest ./hello-world.tar -o linux -a amd64 -n docker.io
`

func ParseArgs() (OptMap, bool) {
	opts := OptMap{
		ImageOpt:     {Name: ImageOpt},
		DestOpt:      {Name: DestOpt},
		OsOpt:        {Name: OsOpt, Short: "o", Long: "os", Dflt: runtime.GOOS},
		ArchOpt:      {Name: ArchOpt, Short: "a", Long: "arch", Dflt: runtime.GOARCH},
		NamespaceOpt: {Name: NamespaceOpt, Short: "n", Long: "ns"},
	}
	for i := 1; i < len(os.Args); i++ {
		parsed := false
		for _, option := range opts {
			val, newi := getval(option.Short, option.Long, os.Args, i)
			if val != "" {
				if option.Value != "" {
					// option specified twice
					return opts, false
				}
				setVal(opts, option.Name, val)
				i = newi
				parsed = true
				break
			}
		}
		if !parsed {
			if opts[ImageOpt].Value == "" {
				setVal(opts, ImageOpt, os.Args[i])
			} else if opts[DestOpt].Value == "" {
				setVal(opts, DestOpt, os.Args[i])
			} else {
				return opts, false
			}
		}
	}
	if opts[ImageOpt].Value == "" || opts[DestOpt].Value == "" {
		return opts, false
	}
	for _, option := range opts {
		if option.Value == "" && option.Dflt != "" {
			setVal(opts, option.Name, option.Dflt)
		}
	}
	return opts, true
}

func getval(short, long string, args []string, i int) (string, int) {
	if short == "" && long == "" {
		// positional param
		return "", 0
	}
	// --foo bar
	opt := "--" + long
	if args[i] == opt && i < len(args)-1 {
		return args[i+1], i + 1
	}
	// --foo=bar
	opt += "="
	if strings.HasPrefix(args[i], opt) {
		return (args[i])[len(opt):], i
	}
	// -f bar
	opt = "-" + short
	if args[i] == opt && i < len(args)-1 {
		return args[i+1], i + 1
	}
	// -fbar
	if strings.HasPrefix(args[i], opt) && len(args[i]) > len(opt) {
		return (args[i])[len(opt):], i
	}
	// --f=bar
	opt += "="
	if strings.HasPrefix(args[i], opt) {
		return (args[i])[len(opt):], i
	}
	return "", 0
}

func setVal(opts map[OptName]Opt, name OptName, value string) {
	opt := opts[name]
	opt.Value = value
	opts[name] = opt
}

func (m OptMap) Val(n OptName) string {
	return m[n].Value
}

func Usage() {
	fmt.Println(usage)
}
