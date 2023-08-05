package kdlib

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	apiclient "github.com/vpngen/domain-commander/subdomain-provisioner/gen-client/client"
	"github.com/vpngen/domain-commander/subdomain-provisioner/gen-client/client/operations"
)

const APIRequestTimeout = 10 * time.Second

// ErrEmptySubdomain - is returned when the subdomain is empty
var ErrEmptySubdomain = errors.New("empty subdomain")

func createSubdomainAPIClient(host, token string) (*apiclient.Subdomapi, runtime.ClientAuthInfoWriter) {
	// create the transport
	transport := httptransport.New(host, "", nil)

	// create the API client, with the transport
	client := apiclient.New(transport, strfmt.Default)

	// bearerToken - the token to authenticate the request
	bearerToken := httptransport.BearerToken(token)

	return client, bearerToken
}

func SubdomainPick(host, token string) (string, error) {
	client, bearerToken := createSubdomainAPIClient(host, token)

	// make the request
	resp, err := client.Operations.PostSubdomain(
		operations.NewPostSubdomainParams().WithTimeout(APIRequestTimeout),
		bearerToken,
	)
	if err != nil {
		return "", fmt.Errorf("post subdomain: %w", err)
	}

	if resp.Payload.SubdomainName == nil {
		return "", ErrEmptySubdomain
	}

	return *resp.Payload.SubdomainName, nil
}

func SubdomainDelete(host, token, subdomain string) error {
	client, bearerToken := createSubdomainAPIClient(host, token)

	// make the request
	_, err := client.Operations.DeleteSubdomainSubdomain(
		operations.NewDeleteSubdomainSubdomainParams().WithSubdomain(subdomain).WithTimeout(APIRequestTimeout),
		bearerToken,
	)
	if err != nil {
		return fmt.Errorf("delete subdomain: %w", err)
	}

	return nil
}
