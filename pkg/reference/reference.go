package reference

import (
	"strings"

	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/docker/distribution/reference"
)

type Image struct {
	Registry  string
	Namespace string
	Name      string
	Tag       string
	ID        string
}

func ParseImage(spec string) (ref Image, err error) {
	namedRef, err := reference.ParseNormalizedNamed(spec)
	if err != nil {
		return
	}

	ref.Registry = reference.Domain(namedRef)
	ref.Name = reference.Path(namedRef)

	if named, ok := namedRef.(reference.NamedTagged); ok {
		ref.Tag = named.Tag()
	}

	if named, ok := namedRef.(reference.Canonical); ok {
		ref.ID = named.Digest().String()
	}

	// It's not enough just to use the reference.ParseNamed(). We have to fill
	// ref.Namespace from ref.Name
	if i := strings.IndexRune(ref.Name, '/'); i != -1 {
		ref.Namespace, ref.Name = ref.Name[:i], ref.Name[i+1:]
	}

	return ref, nil
}

func (n *Image) String() (out string) {
	out = n.Registry

	if len(n.Namespace) > 0 {
		if len(out) > 0 {
			out += "/"
		}
		out += n.Namespace
	}

	if len(n.Name) > 0 {
		if len(out) > 0 {
			out += "/"
		}
		out += n.Name
	}

	if len(n.Tag) > 0 {
		out += ":" + n.Tag
	}

	if len(n.ID) > 0 {
		out += "@" + n.ID
	}

	return
}
