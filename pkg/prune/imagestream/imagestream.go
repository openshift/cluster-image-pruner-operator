package imagestream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/meta"
	metaapi "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/pager"

	imageapi "github.com/openshift/api/image/v1"
	imageclset "github.com/openshift/client-go/image/clientset/versioned"

	pruneapi "github.com/openshift/cluster-image-pruner-operator/pkg/apis/prune/v1alpha1"
	"github.com/openshift/cluster-image-pruner-operator/pkg/imagereference"
	"github.com/openshift/cluster-image-pruner-operator/pkg/reference"
)

type Pruner struct {
	Config      *pruneapi.PruneServiceImages
	ImageClient *imageclset.Clientset
	References  *imagereference.References
}

type pruneJob struct {
	index   int
	changed bool
}

func (p *Pruner) pruneTag(ctx context.Context, is *imageapi.ImageStream, index int, resultChan chan *pruneJob, imageChan chan string) {
	timeThreshold := time.Now()

	if p.Config.KeepTagYoungerThan != nil {
		timeThreshold.Add(-*p.Config.KeepTagYoungerThan)
	}

	istag := is.Status.Tags[index]

	namedRef := &reference.Image{
		Name:      is.GetName(),
		Namespace: is.GetNamespace(),
		Tag:       istag.Tag,
	}

	prunableTagItems := map[*imageapi.TagEvent]struct{}{}

	for i, revtag := range istag.Items {
		namedRef.ID = revtag.Image

		if p.Config.KeepTagRevisions != nil && i < *p.Config.KeepTagRevisions {
			if glog.V(5) {
				glog.Infof("ignore imagestream tag revision due keep-tag-revisions limit: %s", namedRef.String())
			}
			continue
		}

		if revtag.Created.After(timeThreshold) {
			if glog.V(5) {
				glog.Infof("ignore imagestream tag revision due age limit: %s", namedRef.String())
			}
			continue
		}

		if p.References.IsExist(namedRef.String()) {
			continue
		}

		prunableTagItems[&istag.Items[i]] = struct{}{}

		imageChan <- istag.Items[i].Image
	}

	namedRef.ID = ""

	if p.References.IsExist(namedRef.String()) && len(prunableTagItems) == len(istag.Items) {
		// If there is a reference to the tag without a revision,
		// then we must leave at least one last revision.
		delete(prunableTagItems, &istag.Items[0])
	}

	switch len(prunableTagItems) {
	case 0:
		// Nothing to do.
		if glog.V(5) {
			glog.Infof("imagestream tag %s is not touched, %d revisions is found", namedRef.String(), len(istag.Items))
		}

		resultChan <- &pruneJob{
			index: index,
		}

		return

	case len(istag.Items):
		// Tags without revisions do not make sense.
		if glog.V(5) {
			glog.Infof("remove all revisions in %s", namedRef.String())
		}

		istag.Items = istag.Items[:0]

		resultChan <- &pruneJob{
			index:   index,
			changed: true,
		}

		return
	}

	var tagItems []imageapi.TagEvent

	for j := range istag.Items {
		if _, ok := prunableTagItems[&istag.Items[j]]; !ok {
			tagItems = append(tagItems, istag.Items[j])
		} else {
			namedRef.Tag = istag.Tag
			namedRef.ID = istag.Items[j].Image

			glog.Infof("prune tag revision at position %s: %s", j, namedRef.String())
		}
	}

	istag.Items = tagItems

	resultChan <- &pruneJob{
		index:   index,
		changed: true,
	}
}

func (p *Pruner) pruneImageStream(ctx context.Context, is *imageapi.ImageStream) {
	namedRef := &reference.Image{
		Name:      is.GetName(),
		Namespace: is.GetNamespace(),
	}

	glog.Infof("prune imagestream %s", namedRef.String())

	droppedImages := map[string]struct{}{}

	imagesChan := make(chan string)
	resultChan := make(chan *pruneJob)

	for i := 0; i < len(is.Status.Tags); i++ {
		go p.pruneTag(ctx, is, i, resultChan, imagesChan)
	}

	isTagEmpty := false
	isChanged := false

	for i := 0; i != len(is.Status.Tags); {
		select {
		case imageName := <-imagesChan:
			droppedImages[imageName] = struct{}{}
			continue
		default:
		}

		select {
		case state := <-resultChan:
			if len(is.Status.Tags[state.index].Items) == 0 {
				// Even if the tag has not changed, we delete empty tags.
				isTagEmpty = true
			}
			if state.changed {
				isChanged = true
			}
			i += 1
		}
	}

	isEmpty := true

	for i := 0; i < len(is.Status.Tags); i++ {
		if len(is.Status.Tags[i].Items) > 0 {
			isEmpty = false
			break
		}
	}

	if isEmpty {
		// TODO(legion) delete imagestream
		glog.Infof("delete imagestream %s", namedRef.String())
		return
	}

	if isTagEmpty {
		var tags []imageapi.NamedTagEventList

		for i := range is.Status.Tags {
			if len(is.Status.Tags[i].Items) > 0 {
				tags = append(tags, is.Status.Tags[i])
			} else {
				namedRef.Tag = is.Status.Tags[i].Tag
				glog.Infof("prune tag %s", namedRef.String())
			}
		}

		is.Status.Tags = tags
		isChanged = true
	}

	if isChanged {
		namedRef.Tag = ""

		// TODO(legion) update imagestream
		glog.Infof("update imagestream %s:", namedRef.String())
	}
}

func (p *Pruner) Run(ctx context.Context) error {
	if p.References == nil {
		return nil
	}

	glog.Infof("imagestreams pruning ...")

	listObj, err := pager.New(func(ctx context.Context, opts metaapi.ListOptions) (runtime.Object, error) {
		select {
		case <-ctx.Done():
			return nil, nil
		default:
		}
		return p.ImageClient.Image().ImageStreams(metaapi.NamespaceAll).List(opts)
	}).List(ctx, metaapi.ListOptions{})

	if err != nil {
		return fmt.Errorf("unable to fetch imagestreams: %s", err)
	}

	var wg sync.WaitGroup

	err = meta.EachListItem(listObj, func(o runtime.Object) error {
		is := o.(*imageapi.ImageStream)

		wg.Add(1)
		go func(is *imageapi.ImageStream) {
			defer wg.Done()
			p.pruneImageStream(ctx, is)
		}(is)

		return nil
	})

	if err != nil {
		return fmt.Errorf("unable to process imagestreams: %s", err)
	}

	wg.Wait()

	glog.Infof("imagestreams pruning done")
	return nil
}
