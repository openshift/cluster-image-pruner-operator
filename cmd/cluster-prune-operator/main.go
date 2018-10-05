package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	imageclset "github.com/openshift/client-go/image/clientset/versioned"
	pruneclset "github.com/openshift/cluster-prune-operator/pkg/generated/clientset/versioned"
	"github.com/openshift/cluster-prune-operator/pkg/operator"
)

const (
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which is the namespace that the pod is currently running in.
	WatchNamespaceEnvVar = "WATCH_NAMESPACE"
)

// getWatchNamespace returns the namespace the operator should be watching for changes
func getWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(WatchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", WatchNamespaceEnvVar)
	}
	return ns, nil
}

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	namespace, err := getWatchNamespace()
	if err != nil {
		glog.Fatalf("failed to get watch namespace: %s", err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		glog.Fatal(err.Error())
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		glog.Fatal(err.Error())
	}

	pruneClSet, err := pruneclset.NewForConfig(config)
	if err != nil {
		glog.Fatal(err.Error())
	}

	imageClSet, err := imageclset.NewForConfig(config)
	if err != nil {
		glog.Fatal(err.Error())
	}

	pruner := operator.Pruner{
		Client:      client,
		PruneClient: pruneClSet,
		ImageClient: imageClSet,
		Namespace:   namespace,
	}

	glog.Info("Controller is ready to run")

	pruner.Run(context.Background())
}
