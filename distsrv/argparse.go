package distsrv

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// OptName is a unique option name. Such as "user", if "--user" is
// a supported option.
type OptName string

// Opt defines a command line option. The Name is intended to be used as its
// key in a map. Short and long are like -u and --user respectively. Value holds the
// value parsed from the actual command line and Dflt is an optional default if
// no value is provided on the cmdline.
type Opt struct {
	Name  OptName
	Short string
	Long  string
	Value string
	Dflt  string
}

// All the supported options
const (
	ImageOpt     OptName = "image"
	DestOpt      OptName = "dest"
	OsOpt        OptName = "os"
	ArchOpt      OptName = "arch"
	NamespaceOpt OptName = "namespace"
	UsernameOpt  OptName = "user"
	PasswordOpt  OptName = "password"
	SchemeOpt    OptName = "scheme"
	CertOpt      OptName = "cert"
	KeyOpt       OptName = "key"
	CAOpt        OptName = "cacert"
)

// OptMap holds the parsed command line
type OptMap map[OptName]Opt

var usage = `
Usage:

imgpull <image ref> <tar file> [-o|--os os] [-a|--arch arch] [-n|--ns namespace]
 [-u|--user username] [-p|--password password] [-s|--scheme scheme] [-c|--cert tls cert]
 [-k|--key tls key] [-x|--cacert tls ca cert]

The image ref and tar file are required. OS and arch default to your system's values.

Example:

imgpull docker.io/hello-world:latest ./hello-world.tar -o linux -a amd64

The example pulls the image for linux/amd64 to hello-world.tar in the working directory.
`

// ParseArgs parses and validates args, returning them in a map. The 'ToRegistryOpts'
// function can convert the returned map to a 'RegistryOpts' struct.
func ParseArgs() (OptMap, bool) {
	opts := OptMap{
		ImageOpt:     {Name: ImageOpt},
		DestOpt:      {Name: DestOpt},
		OsOpt:        {Name: OsOpt, Short: "o", Long: "os", Dflt: runtime.GOOS},
		ArchOpt:      {Name: ArchOpt, Short: "a", Long: "arch", Dflt: runtime.GOARCH},
		NamespaceOpt: {Name: NamespaceOpt, Short: "n", Long: "ns"},
		UsernameOpt:  {Name: UsernameOpt, Short: "u", Long: "user"},
		PasswordOpt:  {Name: PasswordOpt, Short: "p", Long: "password"},
		SchemeOpt:    {Name: SchemeOpt, Short: "s", Long: "scheme", Dflt: "https"},
		CertOpt:      {Name: CertOpt, Short: "c", Long: "cert"},
		KeyOpt:       {Name: KeyOpt, Short: "k", Long: "key"},
		CAOpt:        {Name: CAOpt, Short: "x", Long: "cacert"},
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
	// need the image to pull and the tarball to save it to
	if opts[ImageOpt].Value == "" || opts[DestOpt].Value == "" {
		return opts, false
	}
	// apply any defaults if an override was not provided on the cmdline
	for _, option := range opts {
		if option.Value == "" && option.Dflt != "" {
			setVal(opts, option.Name, option.Dflt)
		}
	}
	return opts, true
}

// ToRegistryOpts returns the passed map containing parsed args in a
// 'RegistryOpts' struct.
func ToRegistryOpts(opts OptMap) RegistryOpts {
	return RegistryOpts{
		Url:      opts.getVal(ImageOpt),
		Scheme:   opts.getVal(SchemeOpt),
		Dest:     opts.getVal(DestOpt),
		OSType:   opts.getVal(OsOpt),
		ArchType: opts.getVal(ArchOpt),
		Username: opts.getVal(UsernameOpt),
		Password: opts.getVal(PasswordOpt),
		TlsCert:  opts.getVal(CertOpt),
		TlsKey:   opts.getVal(KeyOpt),
		CACert:   opts.getVal(CAOpt),
	}
}

// getval gets a value from a command line arg. Several forms are supported:
//
//	--foo bar
//	--foo=bar
//	-f bar
//	-fbar
//	-f=bar
func getval(short, long string, args []string, i int) (string, int) {
	if short == "" && long == "" {
		// positional param
		return "", 0
	}
	if long != "" {
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
	}
	if short != "" {
		// -f bar
		opt := "-" + short
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
	}
	return "", 0
}

// setVal sets the value for the entry in the options map of the passed
// name
func setVal(opts map[OptName]Opt, name OptName, value string) {
	opt := opts[name]
	opt.Value = value
	opts[name] = opt
}

// getVal gets the named value from the options map
func (m OptMap) getVal(n OptName) string {
	return m[n].Value
}

// Usage prints usage instructions
func Usage() {
	fmt.Println(usage)
}
