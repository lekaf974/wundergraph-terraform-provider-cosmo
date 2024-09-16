package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/wundergraph/cosmo/connect-go/gen/proto/wg/cosmo/common"
	platformv1 "github.com/wundergraph/cosmo/connect-go/gen/proto/wg/cosmo/platform/v1"
)

func (p PlatformClient) CreateToken(ctx context.Context, name, graphName, namespace string) (string, error) {
	request := connect.NewRequest(&platformv1.CreateFederatedGraphTokenRequest{
		GraphName: graphName,
		Namespace: namespace,
		TokenName: name,
	})

	response, err := p.Client.CreateFederatedGraphToken(ctx, request)
	if err != nil {
		return "", err
	}

	if response.Msg == nil {
		return "", fmt.Errorf("failed to create token, the server response is nil")
	}

	if response.Msg.GetResponse().Code != common.EnumStatusCode_OK {
		return "", fmt.Errorf("failed to create token: %s", response.Msg.GetResponse().GetDetails())
	}

	return response.Msg.Token, nil
}

func (p PlatformClient) DeleteToken(ctx context.Context, tokenName, graphName, namespace string) error {
	request := connect.NewRequest(&platformv1.DeleteRouterTokenRequest{
		TokenName: tokenName,
		Namespace: namespace,
	})

	response, err := p.Client.DeleteRouterToken(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	if response.Msg == nil {
		return fmt.Errorf("failed to delete token, the server response is nil")
	}

	if response.Msg.GetResponse().Code != common.EnumStatusCode_OK {
		return fmt.Errorf("failed to delete token: %s", response.Msg)
	}

	return nil
}
