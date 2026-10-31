package buildinfo

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	BuiltAt = "unknown"
)

type Info struct {
	Name    string
	Version string
	Commit  string
	BuiltAt string
}

func Current(name string) Info {
	return Info{
		Name:    name,
		Version: Version,
		Commit:  Commit,
		BuiltAt: BuiltAt,
	}
}

func (i Info) String() string {
	return fmt.Sprintf("%s %s (commit %s, built %s)", i.Name, i.Version, i.Commit, i.BuiltAt)
}
