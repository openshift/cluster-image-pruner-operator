package operator

import (
	"context"
	"sync"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/errors"
	metaapi "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	imageclset "github.com/openshift/client-go/image/clientset/versioned"

	pruneclset "github.com/openshift/cluster-prune-operator/pkg/generated/clientset/versioned"
	"github.com/openshift/cluster-prune-operator/pkg/imagereference"
	"github.com/openshift/cluster-prune-operator/pkg/prune/imagestream"
)

type Pruner struct {
	Namespace   string
	Client      dynamic.Interface
	ImageClient *imageclset.Clientset
	PruneClient *pruneclset.Clientset

	imageReferences *imagereference.References
}

func (p *Pruner) Run(ctx context.Context) {
	var wg sync.WaitGroup

	fetcher := imagereference.Fetcher{
		Namespace:   p.Namespace,
		DynClient:   p.Client,
		PruneClient: p.PruneClient,
	}

	go fetcher.Run(ctx, 3*time.Second, func(ctx context.Context, refs *imagereference.References) error {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if p.imageReferences == nil {
			p.imageReferences = imagereference.New()
		}
		p.imageReferences.Replace(refs)
		return nil
	})

	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}

			cr, err := p.PruneClient.Prune().PruneServices(p.Namespace).Get("prune", metaapi.GetOptions{})
			if err != nil {
				if !errors.IsNotFound(err) {
					glog.Error(err.Error())
				}
				continue
			}

			isPruner := &imagestream.Pruner{
				Config:      &cr.Spec.Images,
				ImageClient: p.ImageClient,
				References:  p.imageReferences,
			}

			err = isPruner.Run(ctx)
			if err != nil {
				glog.Error(err.Error())
			}
		}
	}()

	wg.Wait()
}
