package git

import "github.com/cli/go-gh/v2/pkg/api"

// restClient / gqlClient: the GitHub clients (gh's login)
func restClient() (*api.RESTClient, error) { return api.DefaultRESTClient() }

func gqlClient() (*api.GraphQLClient, error) { return api.DefaultGraphQLClient() }
