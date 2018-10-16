package prune

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/errors"
	metaapi "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	imageapi "github.com/openshift/api/image/v1"
	imageclset "github.com/openshift/client-go/image/clientset/versioned"

	pruneapi "github.com/openshift/cluster-image-pruner-operator/pkg/apis/prune/v1alpha1"
	"github.com/openshift/cluster-image-pruner-operator/pkg/imagereference"
	"github.com/openshift/cluster-image-pruner-operator/pkg/pager"
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
			imageChan <- istag.Items[i].Image
			continue
		}

		if revtag.Created.After(timeThreshold) {
			if glog.V(5) {
				glog.Infof("ignore imagestream tag revision due age limit: %s", namedRef.String())
			}
			imageChan <- istag.Items[i].Image
			continue
		}

		if p.References.IsExist(namedRef.String()) {
			imageChan <- istag.Items[i].Image
			continue
		}

		prunableTagItems[&istag.Items[i]] = struct{}{}
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

func (p *Pruner) pruneImageStream(ctx context.Context, is *imageapi.ImageStream, imageChan chan string, errChan chan error) {
	namedRef := &reference.Image{
		Name:      is.GetName(),
		Namespace: is.GetNamespace(),
	}

	glog.Infof("prune imagestream %s", namedRef.String())

	resultChan := make(chan *pruneJob)

	for i := 0; i < len(is.Status.Tags); i++ {
		go p.pruneTag(ctx, is, i, resultChan, imageChan)
	}

	isTagEmpty := false
	isChanged := false

	for i := 0; i != len(is.Status.Tags); {
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
		if glog.V(4) {
			glog.Infof("delete imagestream %s", namedRef.String())
		}

		uid := is.GetUID()

		err := p.ImageClient.Image().ImageStreams(is.GetNamespace()).Delete(namedRef.String(), &metaapi.DeleteOptions{
			Preconditions: &metaapi.Preconditions{UID: &uid},
		})
		errChan <- err
		return
	}

	if isTagEmpty {
		var tags []imageapi.NamedTagEventList

		for i := range is.Status.Tags {
			if len(is.Status.Tags[i].Items) > 0 {
				tags = append(tags, is.Status.Tags[i])
			} else {
				namedRef.Tag = is.Status.Tags[i].Tag
				if glog.V(4) {
					glog.Infof("prune tag %s", namedRef.String())
				}
			}
		}

		is.Status.Tags = tags
		isChanged = true
	}

	if isChanged {
		namedRef.Tag = ""

		if glog.V(4) {
			glog.Infof("update imagestream %s", namedRef.String())
		}

		_, err := p.ImageClient.Image().ImageStreams(is.GetNamespace()).UpdateStatus(is)

		errChan <- err
		return
	}

	errChan <- nil
}

func (p *Pruner) Run(ctx context.Context) error {
	if p.References == nil {
		return nil
	}

	glog.Infof("imagestreams pruning ...")

	n := 0

	errChan := make(chan error)
	imageChan := make(chan string)

	opts := metaapi.ListOptions{
		Limit: 300,
	}

	err := pager.EachListItem(ctx, opts,
		func(opts metaapi.ListOptions) (runtime.Object, error) {
			select {
			case <-ctx.Done():
				return nil, nil
			default:
			}
			return p.ImageClient.Image().ImageStreams(metaapi.NamespaceAll).List(opts)
		},
		func(o runtime.Object) error {
			is := o.(*imageapi.ImageStream)

			n++
			go func(is *imageapi.ImageStream) {
				p.pruneImageStream(ctx, is, imageChan, errChan)
			}(is)

			return nil
		},
	)

	if err != nil {
		return fmt.Errorf("unable to prune imagestreams: %s", err)
	}

	interupt := false
	referencedImages := map[string]struct{}{}

	for n != 0 {
		select {
		case imageName := <-imageChan:
			referencedImages[imageName] = struct{}{}
		case err := <-errChan:
			if err != nil {
				glog.Errorf("imagestreams pruning failed: %s", err)
				interupt = true
			}
			n--
		}
	}

	if interupt {
		return fmt.Errorf("imagestreams pruning aborted due to imagestream update errors")
	}

	glog.Infof("imagestreams pruning done")

	opts = metaapi.ListOptions{
		Limit: 300,
	}

	err = pager.EachListItem(ctx, opts,
		func(opts metaapi.ListOptions) (runtime.Object, error) {
			select {
			case <-ctx.Done():
				return nil, nil
			default:
			}
			return p.ImageClient.Image().Images().List(opts)
		},
		func(o runtime.Object) error {
			image := o.(*imageapi.Image)
			imageName := image.GetName()

			if _, ok := referencedImages[imageName]; ok {
				return nil
			}

			if glog.V(4) {
				glog.Infof("delete image %s:", imageName)
			}

			uid := image.GetUID()

			err := p.ImageClient.Image().Images().Delete(imageName, &metaapi.DeleteOptions{
				Preconditions: &metaapi.Preconditions{UID: &uid},
			})
			if !errors.IsNotFound(err) {
				return err
			}
			return nil
		},
	)

	if err != nil {
		return fmt.Errorf("unable to prune images: %s", err)
	}

	glog.Infof("images pruning done")

	return nil
}
