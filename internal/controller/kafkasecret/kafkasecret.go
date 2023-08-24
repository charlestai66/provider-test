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

package kafkasecret

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/charlestai66/provider-test/apis/kcpaved/v1alpha1"
	apisv1alpha1 "github.com/charlestai66/provider-test/apis/v1alpha1"
	"github.com/charlestai66/provider-test/internal/clients"
	"github.com/charlestai66/provider-test/internal/features"

	// CT
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	idpssdk "github.intuit.com/idps/idps-go-sdk/v3/idps-sdk"
	idpsconfig "github.intuit.com/idps/idps-go-sdk/v3/idps-sdk/config"
	idpsitems "github.intuit.com/idps/idps-go-sdk/v3/idps-sdk/items"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	// ~CT
)

const (
	errNotKafkaSecret = "managed resource is not a KafkaSecret custom resource"
	errTrackPCUsage   = "cannot track ProviderConfig usage"
	errGetPC          = "cannot get ProviderConfig"
	errGetCreds       = "cannot get credentials"

	errNewClient = "cannot create new Service"

	// CT
	errNewKubernetesClient       = "cannot create new Kubernetes client"
	errFailedToCreateRestConfig  = "cannot create new REST config using provider secret"
	errNewIdpsClient             = "cannot create new IDPS client"
	errGetTruststoreCertFromIdps = "cannot get truststore cert from IDPS"

	nameEventBusSecret = "eventbus-secret"
	nameTruststoreCert = "truststore_ca.crt"
	nameKeystoreKey    = "keystore_key.pem"
	nameKeystoreCert   = "keystore_crt.pem"
	// ~CT
)

// A NoOpService does nothing.
type NoOpService struct{}

var (
	newNoOpService = func(_ []byte) (interface{}, error) { return &NoOpService{}, nil }
)

// Setup adds a controller that reconciles KafkaSecret managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.KafkaSecretGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.KafkaSecretGroupVersionKind),
		// CT
		// managed.WithExternalConnecter(&connector{
		// 	kube:         mgr.GetClient(),
		// 	usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
		// 	newServiceFn: newNoOpService}),
		managed.WithExternalConnecter(&connector{
			kube:            mgr.GetClient(),
			usage:           resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			logger:          o.Logger,
			newServiceFn:    newNoOpService,
			kcfgExtractorFn: resource.CommonCredentialExtractor,
			newRESTConfigFn: clients.NewRESTConfig,
			newKubeClientFn: clients.NewKubeClient,
		}),
		// ~CT

		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.KafkaSecret{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte) (interface{}, error)

	// CT
	logger          logging.Logger
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

	cr, ok := mg.(*v1alpha1.KafkaSecret)
	if !ok {
		return nil, errors.New(errNotKafkaSecret)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	// CT

	// cd := pc.Spec.Credentials
	// data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	// if err != nil {
	// 	return nil, errors.Wrap(err, errGetCreds)
	// }

	var data []byte
	// ~CT

	svc, err := c.newServiceFn(data)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	// CT
	// get Kubernetes client
	kubeClient, err := c.getKubeClient(ctx, pc)
	if err != nil {
		return nil, err
	}

	// get IDPS client
	idpsClient, err := c.getIdpsClient(cr)
	if err != nil {
		return nil, err
	}

	return &external{
		service: svc,
		logger:  c.logger,
		client: resource.ClientApplicator{
			Client:     kubeClient,
			Applicator: resource.NewAPIPatchingApplicator(kubeClient),
		},
		localClient: c.kube,
		idpsClient:  idpsClient,
	}, nil

	// return &external{service: svc}, nil
	// ~CT
}

func (c *connector) getKubeClient(ctx context.Context, pc *apisv1alpha1.ProviderConfig) (client.Client, error) {

	var rc *rest.Config
	var err error

	switch cd := pc.Spec.Credentials; cd.Source { //nolint:exhaustive
	case xpv1.CredentialsSourceInjectedIdentity:
		fmt.Printf("Switch case: In-Cluster\n")

		rc, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, errFailedToCreateRestConfig)
		}
	default:
		fmt.Printf("Switch case: Default\n")

		kc, err := c.kcfgExtractorFn(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
		if err != nil {

			fmt.Printf("kcfgExtractorFn Error: %+v\n", err)

			return nil, errors.Wrap(err, errGetCreds)
		}

		if rc, err = c.newRESTConfigFn(kc); err != nil {
			return nil, errors.Wrap(err, errFailedToCreateRestConfig)
		}
	}

	kubeClient, err := c.newKubeClientFn(rc)
	if err != nil {

		fmt.Printf("newKubeClientFn Error: %+v\n", err)

		return nil, errors.Wrap(err, errNewKubernetesClient)
	}
	return kubeClient, nil
}

func (*connector) getIdpsClient(cr *v1alpha1.KafkaSecret) (*idpssdk.IdpsClient, error) {

	props := idpsconfig.ClientProperties{
		ApiEndPoint:   cr.Spec.ForProvider.ApiEndPoint,
		PolicyId:      cr.Spec.ForProvider.PolicyId,
		ExpiryRequest: 3,
	}
	idpsClient, err := idpssdk.IdpsClientInstanceWithConfig(&props)
	if err != nil {
		fmt.Printf("Cannot get IDPS client: %+v. error: %+v\n", props, err)
		return nil, errors.Wrap(err, errNewIdpsClient)
	} else {
		fmt.Printf("Obtained IDPS client: %+v\n", idpsClient)
	}
	return idpsClient, nil
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

	idpsClient *idpssdk.IdpsClient
	// ~CT
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KafkaSecret)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKafkaSecret)
	}

	// These fmt statements should be removed in the real implementation.
	fmt.Printf("Observing: %+v\n", cr)

	// CT
	secretKey := client.ObjectKey{
		Name:      nameEventBusSecret,
		Namespace: cr.Spec.ForProvider.Namespace,
	}
	secret := &v1.Secret{}
	secretErr := c.client.Get(ctx, secretKey, secret)
	secretExists := (secretErr == nil)
	if secretExists {
		fmt.Printf("Secret exists: %+v\n", secret)
	} else {
		fmt.Printf("Secret does not exist: %+v. error: %+v\n", secretKey, secretErr)
	}

	// fmt.Printf("Secret value truststore cert: %+v\n", base64.StdEncoding.EncodeToString(secret.Data[nameTruststoreCert]))
	// fmt.Printf("Secret value keystore key: %+v\n", base64.StdEncoding.EncodeToString(secret.Data[nameKeystoreKey]))
	// fmt.Printf("Secret value keystore cert: %+v\n", base64.StdEncoding.EncodeToString(secret.Data[nameKeystoreCert]))

	idpsSecretValues := c.getSecretValuesFromIdps(cr)

	// fmt.Printf("IDPS value truststore cert: %+v\n", base64.StdEncoding.EncodeToString(idpsSecretValues[nameTruststoreCert]))
	// fmt.Printf("IDPS value keystore key: %+v\n", base64.StdEncoding.EncodeToString(idpsSecretValues[nameKeystoreKey]))
	// fmt.Printf("IDPS value keystore cert: %+v\n", base64.StdEncoding.EncodeToString(idpsSecretValues[nameKeystoreCert]))

	// check if secret is up to date
	truststoreCertUpToDate := bytes.Equal(secret.Data[nameTruststoreCert], idpsSecretValues[nameTruststoreCert])
	keystoreKeyUpToDate := bytes.Equal(secret.Data[nameKeystoreKey], idpsSecretValues[nameKeystoreKey])
	keystoreCertUpToDate := bytes.Equal(secret.Data[nameKeystoreCert], idpsSecretValues[nameKeystoreCert])
	secretUpToDate := truststoreCertUpToDate && keystoreKeyUpToDate && keystoreCertUpToDate

	fmt.Printf("Secret up to date status: %+v\n", secretUpToDate)
	fmt.Printf("Secret value trustore cert up to date status: %+v\n", truststoreCertUpToDate)
	fmt.Printf("Secret value keystore key up to date status: %+v\n", keystoreKeyUpToDate)
	fmt.Printf("Secret value keystore cert up to date status: %+v\n", keystoreCertUpToDate)
	// ~CT

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.

		// CT
		ResourceExists: secretExists,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.

		// CT
		ResourceUpToDate: secretUpToDate,

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KafkaSecret)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKafkaSecret)
	}

	fmt.Printf("Creating: %+v\n", cr)

	// CT
	secretValues := c.getSecretValuesFromIdps(cr)

	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nameEventBusSecret,
			Namespace: cr.Spec.ForProvider.Namespace,
		},
		Data: secretValues,
	}

	fmt.Printf("Creating secret: %+v\n", secret)

	err := c.client.Create(ctx, secret)
	if err != nil {
		fmt.Printf("Secret not created: %+v. error: %+v\n", secret, err)
	} else {
		fmt.Printf("Secret created: %+v\n", secret)

		cr.SetConditions(xpv1.Available())

		fmt.Printf("Secret condition updated: %+v\n", xpv1.Available())
	}
	// ~CT

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.KafkaSecret)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotKafkaSecret)
	}

	fmt.Printf("Updating: %+v\n", cr)

	// CT
	secretValues := c.getSecretValuesFromIdps(cr)

	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nameEventBusSecret,
			Namespace: cr.Spec.ForProvider.Namespace,
		},
		Data: secretValues,
	}

	fmt.Printf("Updating secret: %+v\n", secret)

	err := c.client.Update(ctx, secret)
	if err != nil {
		fmt.Printf("Secret not updated: %+v. error: %+v\n", secret, err)
	} else {
		fmt.Printf("Secret updated: %+v\n", secret)

		cr.SetConditions(xpv1.Available())

		fmt.Printf("Secret condition updated: %+v\n", xpv1.Available())
	}
	// ~CT

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.KafkaSecret)
	if !ok {
		return errors.New(errNotKafkaSecret)
	}

	fmt.Printf("Deleting: %+v\n", cr)

	// CT
	secretKey := client.ObjectKey{
		Name:      nameEventBusSecret,
		Namespace: cr.Spec.ForProvider.Namespace,
	}
	secret := &v1.Secret{}
	secretErr := c.client.Get(ctx, secretKey, secret)
	secretExists := (secretErr == nil)
	if secretExists {
		fmt.Printf("Secret exists. Deleting secret: %+v\n", secret)

		err := c.client.Delete(ctx, secret)
		if err != nil {
			fmt.Printf("Secret not deleted: %+v. error: %+v\n", secretKey, err)
		} else {
			fmt.Printf("Secret deleted: %+v\n", secret)
		}
	} else {
		fmt.Printf("Secret does not exist: %+v. error: %+v\n", secret, secretErr)
	}
	// ~CT

	return nil
}

func (c *external) getSecretValuesFromIdps(cr *v1alpha1.KafkaSecret) map[string][]byte {

	// get truststore cert
	truststoreCertVar := idpsitems.SecretVariables{
		SecretName: cr.Spec.ForProvider.TruststoreCertPath,
	}
	truststoreCert, err := c.idpsClient.GetSecret(&truststoreCertVar, 0)
	var truststoreCertValue []byte
	if err != nil {
		fmt.Printf("Cannot get truststore cert from IDPS: %+v. error: %+v\n", truststoreCertVar, err)
	} else {
		fmt.Printf("Obtained truststore cert from IDPS: %+v\n", truststoreCert)
		truststoreCertValue, err = base64.StdEncoding.DecodeString(truststoreCert.SecretValue)
		if err != nil {
			fmt.Printf("Cannot decode truststore cert from IDPS: %+v. error: %+v\n", truststoreCert, err)
		}
	}

	// get keystore key
	keystoreKeyVar := idpsitems.SecretVariables{
		SecretName: cr.Spec.ForProvider.KeystoreKeyPath,
	}
	keystoreKey, err := c.idpsClient.GetSecret(&keystoreKeyVar, 0)
	var keystoreKeyValue []byte
	if err != nil {
		fmt.Printf("Cannot get keystore key from IDPS: %+v. error: %+v\n", keystoreKeyVar, err)
	} else {
		fmt.Printf("Obtained keystore key from IDPS: %+v\n", keystoreKey)
		keystoreKeyValue, err = base64.StdEncoding.DecodeString(keystoreKey.SecretValue)
		if err != nil {
			fmt.Printf("Cannot decode keystore key from IDPS: %+v. error: %+v\n", keystoreKey, err)
		}
	}

	// get keystor cert
	keystoreCertVar := idpsitems.SecretVariables{
		SecretName: cr.Spec.ForProvider.KeystoreCertPath,
	}
	keystoreCert, err := c.idpsClient.GetSecret(&keystoreCertVar, 0)
	var keystoreCertValue []byte
	if err != nil {
		fmt.Printf("Cannot get keystore cert from IDPS: %+v. error: %+v\n", keystoreCertVar, err)
	} else {
		fmt.Printf("Obtained keystore cert from IDPS: %+v\n", keystoreCert)
		keystoreCertValue, err = base64.StdEncoding.DecodeString(keystoreCert.SecretValue)
		if err != nil {
			fmt.Printf("Cannot decode keystore cert from IDPS: %+v. error: %+v\n", keystoreCert, err)
		}
	}

	return map[string][]byte{
		nameTruststoreCert: truststoreCertValue,
		nameKeystoreKey:    keystoreKeyValue,
		nameKeystoreCert:   keystoreCertValue,
	}
}
