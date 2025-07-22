package client

// Provides a common client for functions outside of controllers to use.

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	commonClient client.Client
	kubeconf     *rest.Config
)

func SetClient(c client.Client) {
	commonClient = c
}

func NewClientFromConfig(cfg *rest.Config) (c client.Client, err error) {
	commonClient, err = client.New(cfg, client.Options{})
	return commonClient, err
}

func GetClient() client.Client {
	return commonClient
}

func SetKubeconf(kcfg *rest.Config) {
	kubeconf = kcfg
}

func GetKubeconf() *rest.Config {
	return kubeconf
}

func CreateOrUpdate(ctx context.Context, obj client.Object) error {
	err := commonClient.Create(ctx, obj)
	// if err is alreadyexists, try update
	if errors.IsAlreadyExists(err) {
		return commonClient.Update(ctx, obj)
	}
	return err
}
