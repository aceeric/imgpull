package main

import (
	"fmt"
	"imgpull/pkg/imgpull"
	"os"
	"runtime"
	"strings"
)

var (
	buildVer string = "SET BY MAKE FILE"
	buildDtm string = "SET BY MAKE FILE"
)

// optName is a unique option name. Such as "user", if "--user" is
// a supported option.
type optName string

// opt defines a command line option. The Name is intended to be used as its
// key in a map. Short and long are like -u and --user respectively. Value holds the
// value parsed from the actual command line and Dflt is an optional default if
// no value is provided on the cmdline.
type opt struct {
	Name     optName
	Short    string
	Long     string
	Value    string
	Dflt     string
	IsSwitch bool
	Func     func()
}

// All the supported options
const (
	imageOpt     optName = "image"
	destOpt      optName = "dest"
	osOpt        optName = "os"
	archOpt      optName = "arch"
	namespaceOpt optName = "namespace"
	usernameOpt  optName = "user"
	passwordOpt  optName = "password"
	schemeOpt    optName = "scheme"
	certOpt      optName = "cert"
	keyOpt       optName = "key"
	caOpt        optName = "cacert"
	versionOpt   optName = "version"
	helpOpt      optName = "help"
)

// optMap holds the parsed command line
type optMap map[optName]opt

var usageText = `
Usage:

imgpull <image ref> <tar file> [-o|--os os] [-a|--arch arch] [-n|--ns namespace]
 [-u|--user username] [-p|--password password] [-s|--scheme scheme] [-c|--cert tls cert]
 [-k|--key tls key] [-x|--cacert tls ca cert] [-v|--version] [-h|--help]

The image ref and tar file are required. OS and arch default to your system's values.

Example:

imgpull docker.io/hello-world:latest ./hello-world.tar -o linux -a amd64

The example pulls the image for linux/amd64 to hello-world.tar in the working directory.
`

// parseArgs parses and validates args, returning them in a map. The 'ToRegistryOpts'
// function can convert the returned map to a 'RegistryOpts' struct.
func parseArgs() (optMap, bool) {
	opts := optMap{
		imageOpt:     {Name: imageOpt},
		destOpt:      {Name: destOpt},
		osOpt:        {Name: osOpt, Short: "o", Long: "os", Dflt: runtime.GOOS},
		archOpt:      {Name: archOpt, Short: "a", Long: "arch", Dflt: runtime.GOARCH},
		namespaceOpt: {Name: namespaceOpt, Short: "n", Long: "ns"},
		usernameOpt:  {Name: usernameOpt, Short: "u", Long: "user"},
		passwordOpt:  {Name: passwordOpt, Short: "p", Long: "password"},
		schemeOpt:    {Name: schemeOpt, Short: "s", Long: "scheme", Dflt: "https"},
		certOpt:      {Name: certOpt, Short: "c", Long: "cert"},
		keyOpt:       {Name: keyOpt, Short: "k", Long: "key"},
		caOpt:        {Name: caOpt, Short: "x", Long: "cacert"},
		versionOpt:   {Name: versionOpt, Short: "v", Long: "version", IsSwitch: true, Func: showVersionAndExit},
		helpOpt:      {Name: helpOpt, Short: "h", Long: "help", IsSwitch: true, Func: showUsageAndExit},
	}
	for i := 1; i < len(os.Args); i++ {
		parsed := false
		for _, option := range opts {
			val, newi := getval(option.Short, option.Long, option.IsSwitch, os.Args, i)
			if val != "" {
				if option.Func != nil {
					option.Func()
				}
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
			if opts[imageOpt].Value == "" {
				setVal(opts, imageOpt, os.Args[i])
			} else if opts[destOpt].Value == "" {
				setVal(opts, destOpt, os.Args[i])
			} else {
				return opts, false
			}
		}
	}
	// need the image to pull and the tarball to save it to
	if opts[imageOpt].Value == "" || opts[destOpt].Value == "" {
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

// toRegistryOpts returns the passed map containing parsed args in a
// 'RegistryOpts' struct.
func toRegistryOpts(opts optMap) imgpull.RegistryOpts {
	return imgpull.RegistryOpts{
		Url:       opts.getVal(imageOpt),
		Scheme:    opts.getVal(schemeOpt),
		Dest:      opts.getVal(destOpt),
		OSType:    opts.getVal(osOpt),
		ArchType:  opts.getVal(archOpt),
		Namespace: opts.getVal(namespaceOpt),
		Username:  opts.getVal(usernameOpt),
		Password:  opts.getVal(passwordOpt),
		TlsCert:   opts.getVal(certOpt),
		TlsKey:    opts.getVal(keyOpt),
		CACert:    opts.getVal(caOpt),
	}
}

// getval gets a value from a command line arg. Several forms are supported:
//
//	--foo (this is a switch-style)
//	--foo bar
//	--foo=bar
//	-f (also switch-style)
//	-f bar
//	-fbar
//	-f=bar
func getval(short, long string, isswitch bool, args []string, i int) (string, int) {
	if short == "" && long == "" {
		// positional param
		return "", 0
	}
	if long != "" {
		opt := "--" + long
		// --foo
		if args[i] == opt && isswitch {
			return "true", i
		}
		// --foo bar
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
		opt := "-" + short
		// -f
		if args[i] == opt && isswitch {
			return "true", i
		}
		// -f bar
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
func setVal(opts map[optName]opt, name optName, value string) {
	opt := opts[name]
	opt.Value = value
	opts[name] = opt
}

// getVal gets the named value from the options map
func (m optMap) getVal(n optName) string {
	return m[n].Value
}

// showUsageAndExit prints usage instructions and exits with a zero
// error code.
func showUsageAndExit() {
	fmt.Println(usageText)
	os.Exit(0)
}

// showVersionAndExit prints version info and exits with a zero
// error code.
func showVersionAndExit() {
	fmt.Printf("imgpull version: %s build date: %s\n", buildVer, buildDtm)
	os.Exit(0)
}
