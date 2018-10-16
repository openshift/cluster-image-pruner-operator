package pager

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ListPageFunc func(metav1.ListOptions) (runtime.Object, error)
type ItemFunc func(runtime.Object) error

func EachListItem(ctx context.Context, options metav1.ListOptions, PageFn ListPageFunc, ItemFn ItemFunc) error {
	for {
		obj, err := PageFn(options)
		if err != nil {
			return err
		}

		m, err := meta.ListAccessor(obj)
		if err != nil {
			return fmt.Errorf("returned object must be a list: %v", err)
		}

		err = meta.EachListItem(obj, ItemFn)
		if err != nil {
			return err
		}

		// if we have no more items, return the list
		if len(m.GetContinue()) == 0 {
			return nil
		}

		// set the next loop up
		options.Continue = m.GetContinue()
	}
}
