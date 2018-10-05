package imagereference

import (
	"fmt"
	"sync"

	"github.com/openshift/cluster-prune-operator/pkg/reference"
)

type References struct {
	mu sync.RWMutex
	v  map[string]struct{}
}

func New() *References {
	return &References{
		v: make(map[string]struct{}),
	}
}

func (i *References) Replace(n *References) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.v = n.v
}

func (i *References) Add(spec string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	ref, err := reference.ParseImage(spec)
	if err != nil {
		return fmt.Errorf("failed to parse image reference: %s: %s", spec, err)
	}

	// We ignore the host part completely. This is not a hack. Any objects
	// of any imagestreams can be on the registry server due to mirroring.
	ref.Registry = ""

	i.v[ref.String()] = struct{}{}

	return nil
}

func (i *References) IsExist(spec string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	_, ok := i.v[spec]
	return ok
}

func (i *References) Names() (names []string) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for k := range i.v {
		names = append(names, k)
	}

	return
}
