package auth

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go-core/authentication"
)

func NewMSGraphClient(httpClient *http.Client) (*msgraphsdk.GraphServiceClient, *azidentity.DefaultAzureCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Transport: httpClient,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error creating azure credential: %w", err)
	}

	scopes := []string{"https://graph.microsoft.com/.default"}

	auth, err := authentication.NewAzureIdentityAuthenticationProviderWithScopesAndValidHosts(
		cred,
		scopes,
		[]string{"graph.microsoft.com"},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating msgraph authentication provider: %w", err)
	}

	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		auth,
		nil,
		nil,
		httpClient,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating msgraph request adapter: %w", err)
	}

	return msgraphsdk.NewGraphServiceClient(adapter), cred, nil
}
