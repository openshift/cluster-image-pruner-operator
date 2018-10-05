package imagereference

import (
	"context"
	"reflect"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metaapi "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/pager"
	"k8s.io/client-go/util/jsonpath"

	pruneapi "github.com/openshift/cluster-prune-operator/pkg/apis/prune/v1alpha1"
	pruneclset "github.com/openshift/cluster-prune-operator/pkg/generated/clientset/versioned"
)

type ImageReferenceFunc func(context.Context, *References) error

type Fetcher struct {
	Namespace   string
	DynClient   dynamic.Interface
	PruneClient *pruneclset.Clientset
}

func (f *Fetcher) getResources() ([]pruneapi.ImageReference, error) {
	list, err := f.PruneClient.Prune().ImageReferences(f.Namespace).List(metaapi.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (f *Fetcher) fetch(ctx context.Context, ref pruneapi.ImageReference, refChan chan string) error {
	resource := schema.GroupVersionResource{
		Group:    ref.Spec.Group,
		Version:  ref.Spec.APIVersion,
		Resource: ref.Spec.Name,
	}

	listObj, err := pager.New(func(ctx context.Context, opts metaapi.ListOptions) (runtime.Object, error) {
		select {
		case <-ctx.Done():
			return nil, nil
		default:
		}
		return f.DynClient.Resource(resource).Namespace(metaapi.NamespaceAll).List(opts)
	}).List(ctx, metaapi.ListOptions{})

	if err != nil {
		return err
	}

	return meta.EachListItem(listObj, func(o runtime.Object) error {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		item := o.(*unstructured.Unstructured)

		if glog.V(5) {
			glog.Infof("fetcher item %s/%s", item.GetNamespace(), item.GetName())
		}

		for _, pattern := range ref.Spec.ImageFieldSelectors {
			if pattern[0] != '{' || pattern[len(pattern)-1] != '}' {
				glog.Errorf("constants are not allowed in %s resource: %s", item.GetName(), pattern)
				continue
			}

			jp := jsonpath.New(item.GetName()).AllowMissingKeys(true)

			if err = jp.Parse(pattern); err != nil {
				glog.Errorf("bad pattern in %s resource: %s: %s", item.GetName(), pattern, err)
				continue
			}

			res, err := jp.FindResults(item.UnstructuredContent())
			if err != nil {
				return err
			}

			for i := range res {
				for j := range res[i] {
					if res[i][j].Kind() != reflect.Interface {
						glog.Errorf("unexpected match type: %v, expect Interface", res[i][j].Kind())
						continue
					}

					v := res[i][j].Elem()

					if v.Kind() != reflect.String {
						glog.Errorf("unsupported value kind: %v, expected String", v.Kind())
						continue
					}

					refChan <- v.String()
				}
			}
		}
		return nil
	})
}

func (f *Fetcher) Run(ctx context.Context, period time.Duration, handler ImageReferenceFunc) {
Loop:
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(period):
		}

		resources, err := f.getResources()
		if err != nil {
			if !errors.IsNotFound(err) {
				glog.Error(err.Error())
			}
			continue
		}

		refs := New()

		refChan := make(chan string, 1000)
		errChan := make(chan error, len(resources))

		for i := range resources {
			go func(resource pruneapi.ImageReference) {
				errChan <- f.fetch(ctx, resource, refChan)
			}(resources[i])
		}

		for done := 0; done != len(resources); {
			select {
			case refname := <-refChan:
				err = refs.Add(refname)
				if err != nil {
					glog.Errorf("unable to add reference: %s", err)
				}
			case err := <-errChan:
				if err != nil {
					glog.Errorf("unable to sync data: %s", err)
					continue Loop
				}
				done += 1
			}
		}

		err = handler(ctx, refs)
		if err != nil {
			return
		}
	}
}
