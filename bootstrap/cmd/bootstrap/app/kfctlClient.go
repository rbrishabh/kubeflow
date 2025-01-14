package app

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/ratelimit"
	httptransport "github.com/go-kit/kit/transport/http"
	kfdefs "github.com/kubeflow/kubeflow/bootstrap/v3/pkg/apis/apps/kfdef/v1alpha1"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"net/url"
	"strings"
	"time"
)

// KfctlClient provides a client to the KfctlServer
type KfctlClient struct {
	createEndpoint endpoint.Endpoint
	getEndpoint    endpoint.Endpoint
}

// NewKfctlClient returns a KfctlClient backed by an HTTP server living at the
// remote instance.
func NewKfctlClient(instance string) (KfctlService, error) {
	// Quickly sanitize the instance string.
	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	u, err := url.Parse(instance)
	if err != nil {
		return nil, err
	}

	// We construct a single ratelimiter middleware, to limit the total outgoing
	// QPS from this client to all methods on the remote instance. We also
	// construct per-endpoint circuitbreaker middlewares to demonstrate how
	// that's done, although they could easily be combined into a single breaker
	// for the entire remote instance, too.
	limiter := ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100))

	// Each individual endpoint is an http/transport.Client (which implements
	// endpoint.Endpoint) that gets wrapped with various middlewares. If you
	// made your own client library, you'd do this work there, so your server
	// could rely on a consistent set of client behavior.
	var createEndpoint endpoint.Endpoint
	{
		createEndpoint = httptransport.NewClient(
			"POST",
			copyURL(u, KfctlCreatePath),
			encodeHTTPGenericRequest,
			decodeHTTPKfdefResponse,
		).Endpoint()
		createEndpoint = limiter(createEndpoint)
	}
	var getEndpoint endpoint.Endpoint
	{
		getEndpoint = httptransport.NewClient(
			"POST",
			copyURL(u, KfctlCreatePath),
			encodeHTTPGenericRequest,
			decodeHTTPKfdefResponse,
		).Endpoint()
		getEndpoint = limiter(getEndpoint)
	}

	// Returning the endpoint.Set as a service.Service relies on the
	// endpoint.Set implementing the Service methods. That's just a simple bit
	// of glue code.
	return &KfctlClient{
		createEndpoint: createEndpoint,
		getEndpoint:    getEndpoint,
	}, nil
}

// CreateDeployment issues a CreateDeployment to the requested backend
func (c *KfctlClient) CreateDeployment(ctx context.Context, req kfdefs.KfDef) (*kfdefs.KfDef, error) {
	var resp interface{}
	var err error
	// Add retry logic
	bo := backoff.WithMaxRetries(backoff.NewConstantBackOff(2*time.Second), 30)
	permErr := backoff.Retry(func() error {
		resp, err = c.createEndpoint(ctx, req)
		if err != nil {
			return err
		}
		return nil
	}, bo)

	if permErr != nil {
		return nil, permErr
	}
	response, ok := resp.(*kfdefs.KfDef)

	if ok {
		return response, nil
	}

	log.Info("Response is not type *KfDef")
	resErr, ok := resp.(*httpError)

	if ok {
		return nil, resErr
	}

	log.Info("Response is not type *httpError")

	pRes, _ := Pformat(resp)
	log.Errorf("Recieved unexpected response; %v", pRes)
	return nil, fmt.Errorf("Recieved unexpected response; %v", pRes)
}

func (c *KfctlClient) GetLatestKfdef(req kfdefs.KfDef) (*kfdefs.KfDef, error) {
	resp, err := c.getEndpoint(context.Background(), req)
	if err != nil {
		return nil, err
	}
	response, ok := resp.(*kfdefs.KfDef)

	if ok {
		return response, nil
	}

	log.Info("Response is not type *KfDef")
	resErr, ok := resp.(*httpError)

	if ok {
		return nil, resErr
	}

	log.Info("Response is not type *httpError")

	pRes, _ := Pformat(resp)
	log.Errorf("Recieved unexpected response; %v", pRes)
	return nil, fmt.Errorf("Recieved unexpected response; %v", pRes)
}
