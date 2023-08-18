/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testtype

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/charlestai66/provider-test/apis/testgroup/v1alpha1"
	apisv1alpha1 "github.com/charlestai66/provider-test/apis/v1alpha1"
	"github.com/charlestai66/provider-test/internal/clients"
	"github.com/charlestai66/provider-test/internal/features"

	// CT
	idpssdk "github.intuit.com/idps/idps-go-sdk/v3/idps-sdk"
	idpsconfig "github.intuit.com/idps/idps-go-sdk/v3/idps-sdk/config"
	idpsitems "github.intuit.com/idps/idps-go-sdk/v3/idps-sdk/items"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// ~CT
)

const (
	errNotTestType  = "managed resource is not a TestType custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"

	// CT
	errNewKubernetesClient      = "cannot create new Kubernetes client"
	errFailedToCreateRestConfig = "cannot create new REST config using provider secret"
	// ~CT
)

// A NoOpService does nothing.
type NoOpService struct{}

var (
	newNoOpService = func(_ []byte) (interface{}, error) { return &NoOpService{}, nil }
)

// Setup adds a controller that reconciles TestType managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.TestTypeGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.TestTypeGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:            mgr.GetClient(),
			usage:           resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			logger:          o.Logger,
			newServiceFn:    newNoOpService,
			kcfgExtractorFn: resource.CommonCredentialExtractor,
			newRESTConfigFn: clients.NewRESTConfig,
			newKubeClientFn: clients.NewKubeClient,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.TestType{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube   client.Client
	usage  resource.Tracker
	logger logging.Logger

	newServiceFn func(creds []byte) (interface{}, error)

	// CT
	kcfgExtractorFn func(ctx context.Context, src xpv1.CredentialsSource, c client.Client, ccs xpv1.CommonCredentialSelectors) ([]byte, error)
	newRESTConfigFn func(kubeconfig []byte) (*rest.Config, error)
	newKubeClientFn func(config *rest.Config) (client.Client, error)
	// ~CT
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {

	// CT
	fmt.Printf("Connect for Managed:  %+v\n", mg)

	cr, ok := mg.(*v1alpha1.TestType)
	if !ok {
		return nil, errors.New(errNotTestType)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	// CT
	var rc *rest.Config
	var err error

	// cd := pc.Spec.Credentials
	// data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	// if err != nil {
	// 	return nil, errors.Wrap(err, errGetCreds)
	// }

	var data []byte
	svc, err := c.newServiceFn(data)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}
	// ~CT

	// CT
	switch cd := pc.Spec.Credentials; cd.Source { //nolint:exhaustive
	case xpv1.CredentialsSourceInjectedIdentity:

		// CT
		fmt.Printf("Switch case: In-Cluster\n")

		rc, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, errFailedToCreateRestConfig)
		}
	default:
		// CT
		fmt.Printf("Switch case: Default\n")

		kc, err := c.kcfgExtractorFn(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
		if err != nil {

			// CT
			fmt.Printf("kcfgExtractorFn Error: %+v\n", err)

			return nil, errors.Wrap(err, errGetCreds)
		}

		if rc, err = c.newRESTConfigFn(kc); err != nil {
			return nil, errors.Wrap(err, errFailedToCreateRestConfig)
		}
	}

	k, err := c.newKubeClientFn(rc)
	if err != nil {

		// CT
		fmt.Printf("newKubeClientFn Error: %+v\n", err)

		return nil, errors.Wrap(err, errNewKubernetesClient)
	}

	return &external{
		service: svc,
		logger:  c.logger,
		client: resource.ClientApplicator{
			Client:     k,
			Applicator: resource.NewAPIPatchingApplicator(k),
		},
		localClient: c.kube,
	}, nil
	// ~CT

	// return &external{service: svc}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	service interface{}

	// CT
	logger logging.Logger
	client resource.ClientApplicator
	// localClient is specifically used to connect to local cluster
	localClient client.Client
	// ~CT
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.TestType)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotTestType)
	}

	// These fmt statements should be removed in the real implementation.
	fmt.Printf("Observing: %+v\n", cr)

	// CT
	ns := &v1.Namespace{}
	err := c.client.Get(ctx, client.ObjectKey{}, ns)
	resExists := (err == nil)

	if err != nil {
		fmt.Printf("Namespace exists: %+v\n", ns)
	} else {
		fmt.Printf("Namespace does not exist: %+v. error: %+v\n", ns, err)
	}
	// ~CT

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: resExists,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: true,

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.TestType)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotTestType)
	}

	fmt.Printf("Creating: %+v\n", cr)

	// CT
	name := mg.GetName()
	ns := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec:   v1.NamespaceSpec{},
		Status: v1.NamespaceStatus{},
	}

	fmt.Printf("Creating namespace: %+v\n", ns)

	err := c.client.Create(ctx, ns)
	if err != nil {
		fmt.Printf("Namespace not created: %+v. error: %+v\n", ns, err)
	} else {
		fmt.Printf("Namespace created: %+v\n", ns)
	}

	props := idpsconfig.ClientProperties{
		ApiEndPoint: cr.Spec.ForProvider.ApiEndPoint,
		PolicyId:    cr.Spec.ForProvider.PolicyId,
	}
	client, err := idpssdk.IdpsClientInstanceWithConfig(&props)
	if err != nil {
		fmt.Printf("Cannot get IDPS client: %+v. error: %+v\n", props, err)
	} else {
		fmt.Printf("Obtained IDPS client: %+v\n", client)
	}

	secretVar := idpsitems.SecretVariables{
		SecretName: cr.Spec.ForProvider.SslPath,
	}
	secret, err := client.GetSecret(&secretVar, 0)
	if err != nil {
		fmt.Printf("Cannot get secret: %+v. error: %+v\n", secretVar, err)
	} else {
		fmt.Printf("Obtained secret: %+v\n", secret)
	}

	// ~CT

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.TestType)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotTestType)
	}

	fmt.Printf("Updating: %+v\n", cr)

	// CT
	name := mg.GetName()
	ns := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec:   v1.NamespaceSpec{},
		Status: v1.NamespaceStatus{},
	}

	fmt.Printf("Updating namespace: %+v\n", ns)

	err := c.client.Update(ctx, ns)
	if err != nil {
		fmt.Printf("Namespace not updated: %+v. error: %+v\n", ns, err)
	} else {
		fmt.Printf("Namespace updated: %+v\n", ns)
	}
	// ~CT

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.TestType)
	if !ok {
		return errors.New(errNotTestType)
	}

	fmt.Printf("Deleting: %+v\n", cr)

	// CT
	name := mg.GetName()
	ns := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec:   v1.NamespaceSpec{},
		Status: v1.NamespaceStatus{},
	}

	fmt.Printf("Deleting namespace: %+v\n", ns)

	err := c.client.Delete(ctx, ns)
	if err != nil {
		fmt.Printf("Namespace not updated: %+v. error: %+v\n", ns, err)
	} else {
		fmt.Printf("Namespace deleted: %+v\n", ns)
	}
	// ~CT

	return nil
}
