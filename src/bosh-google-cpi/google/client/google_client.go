package client

import (
	"net/http"
	"os"
	"time"

	bosherr "github.com/cloudfoundry/bosh-utils/errors"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"

	"bosh-google-cpi/google/config"

	"golang.org/x/oauth2"
	oauthgoogle "golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/storage/v1"
)

const (
	computeScope = compute.ComputeScope
	storageScope = storage.DevstorageFullControlScope
	metadataHost = "metadata.google.internal"
	retryMax     = 5
	retryDelay   = 200
)

type GoogleClient struct {
	config         config.Config
	computeService *compute.Service
	storageService *storage.Service
	logger         boshlog.Logger
}

type RetryTransport struct {
	Transport http.RoundTripper
}

func (rt *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	var try uint64

	for try = 0; try < retryMax; try++ {
		resp, err = rt.Transport.RoundTrip(req)
		if err != nil {
			break
		}
		if resp.StatusCode < 500 || resp.StatusCode > 599 {
			break
		}
		time.Sleep(retryDelay << try * time.Millisecond)
	}
	return resp, err
}

func NewGoogleClient(
	config config.Config,
	logger boshlog.Logger,
) (GoogleClient, error) {
	var err error
	var computeClient, storageClient *http.Client
	userAgent := "bosh-google-cpi/0.0.1"

	if config.JSONKey != "" {
		computeJwtConf, err := oauthgoogle.JWTConfigFromJSON([]byte(config.JSONKey), computeScope)
		if err != nil {
			return GoogleClient{}, bosherr.WrapError(err, "Reading Google JSON Key")
		}
		computeClient = computeJwtConf.Client(oauth2.NoContext)

		storageJwtConf, err := oauthgoogle.JWTConfigFromJSON([]byte(config.JSONKey), storageScope)
		if err != nil {
			return GoogleClient{}, bosherr.WrapError(err, "Reading Google JSON Key")
		}
		storageClient = storageJwtConf.Client(oauth2.NoContext)
	} else {
		if v := os.Getenv("GCE_METADATA_HOST"); v == "" {
			os.Setenv("GCE_METADATA_HOST", metadataHost)
		}
		computeClient, err = oauthgoogle.DefaultClient(oauth2.NoContext, computeScope)
		if err != nil {
			return GoogleClient{}, bosherr.WrapError(err, "Creating a Google default client")
		}

		storageClient, err = oauthgoogle.DefaultClient(oauth2.NoContext, storageScope)
		if err != nil {
			return GoogleClient{}, bosherr.WrapError(err, "Creating a Google default client")
		}
	}

	// Custom RoundTripper for retries
	computeClient.Transport = &RetryTransport{
		Transport: computeClient.Transport,
	}
	computeService, err := compute.New(computeClient)
	if err != nil {
		return GoogleClient{}, bosherr.WrapError(err, "Creating a Google Compute Service client")
	}
	computeService.UserAgent = userAgent

	// Custom RoundTripper for retries
	storageClient.Transport = &RetryTransport{
		Transport: computeClient.Transport,
	}
	storageService, err := storage.New(storageClient)
	if err != nil {
		return GoogleClient{}, bosherr.WrapError(err, "Creating a Google Storage Service client")
	}
	storageService.UserAgent = userAgent

	return GoogleClient{
		config:         config,
		computeService: computeService,
		storageService: storageService,
		logger:         logger,
	}, nil
}

func (c GoogleClient) Project() string {
	return c.config.Project
}

func (c GoogleClient) DefaultRootDiskSizeGb() int {
	return c.config.DefaultRootDiskSizeGb
}

func (c GoogleClient) DefaultRootDiskType() string {
	return c.config.DefaultRootDiskType
}

func (c GoogleClient) DefaultZone() string {
	return c.config.DefaultZone
}

func (c GoogleClient) ComputeService() *compute.Service {
	return c.computeService
}

func (c GoogleClient) StorageService() *storage.Service {
	return c.storageService
}
