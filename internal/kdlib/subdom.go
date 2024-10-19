package kdlib

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	apiclient "github.com/vpngen/domain-commander/subdomain-provisioner/gen-client/client"
	"github.com/vpngen/domain-commander/subdomain-provisioner/gen-client/client/operations"
	"github.com/vpngen/domain-commander/subdomain-provisioner/gen-client/models"
)

const APIRequestTimeout = 10 * time.Second

// ErrEmptySubdomain - is returned when the subdomain is empty
var ErrEmptySubdomain = errors.New("empty subdomain")

// ErrEmptyNameServers - is returned when the name servers are empty
var ErrEmptyNameServers = errors.New("empty name servers")

func createSubdomainAPIClient(host, token string) (*apiclient.Subdomapi, runtime.ClientAuthInfoWriter) {
	// create the transport
	transport := httptransport.New(host, "", nil)

	// create the API client, with the transport
	client := apiclient.New(transport, strfmt.Default)

	// bearerToken - the token to authenticate the request
	bearerToken := httptransport.BearerToken(token)

	return client, bearerToken
}

func SubdomainPick(host, token, srvZone string) (string, string, error) {
	client, bearerToken := createSubdomainAPIClient(host, token)

	fmt.Fprintf(os.Stderr, "Picking subdomain for %s\n", srvZone)
	fmt.Fprintf(os.Stderr, "Using API host: %s\n", host)
	fmt.Fprintf(os.Stderr, "Using API token: %s\n", token)

	param := operations.NewPostSubdomainParams()
	param = param.WithBody(&models.SubdomainRequest{ServiceZone: srvZone})
	param = param.WithTimeout(APIRequestTimeout)

	// make the request
	resp, err := client.Operations.PostSubdomain(
		param,
		bearerToken,
	)
	if err != nil {
		return "", "", fmt.Errorf("post subdomain: %w", err)
	}

	if resp.Payload.SubdomainName == nil || resp.Payload.NameServers == nil {
		return "", "", ErrEmptySubdomain
	}

	if resp.Payload.NameServers == nil {
		return "", "", ErrEmptyNameServers
	}

	return swag.StringValue(resp.Payload.SubdomainName), swag.StringValue(resp.Payload.NameServers), nil
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
