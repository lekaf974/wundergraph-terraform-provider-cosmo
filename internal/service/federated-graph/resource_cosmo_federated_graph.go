package federated_graph

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	common "github.com/wundergraph/cosmo/connect-go/gen/proto/wg/cosmo/common"
	platformv1 "github.com/wundergraph/cosmo/connect-go/gen/proto/wg/cosmo/platform/v1"
	"github.com/wundergraph/cosmo/terraform-provider-cosmo/internal/api"
	"github.com/wundergraph/cosmo/terraform-provider-cosmo/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &FederatedGraphResource{}
var _ resource.ResourceWithImportState = &FederatedGraphResource{}

func NewFederatedGraphResource() resource.Resource {
	return &FederatedGraphResource{}
}

// FederatedGraphResource defines the resource implementation for federated graphs.
type FederatedGraphResource struct {
	client *api.PlatformClient
}

// FederatedGraphResourceModel describes the resource data model for a federated graph.
type FederatedGraphResourceModel struct {
	Id                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Namespace              types.String `tfsdk:"namespace"`
	Readme                 types.String `tfsdk:"readme"`
	RoutingURL             types.String `tfsdk:"routing_url"`
	AdmissionWebhookUrl    types.String `tfsdk:"admission_webhook_url"`
	AdmissionWebhookSecret types.String `tfsdk:"admission_webhook_secret"`
	LabelMatchers          types.List   `tfsdk:"label_matchers"`
}

func (r *FederatedGraphResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_federated_graph"
}

func (r *FederatedGraphResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `
The federated graph resource is used to manage a single, unified data graph that is composed of multiple subgraphs.

For more information on federated graphs, please refer to the [Cosmo Documentation](https://cosmo-docs.wundergraph.com/cli/federated-graph).
		`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the federated graph resource, automatically generated by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the federated graph. This is used to identify the graph and must be unique within the namespace.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "The namespace in which the federated graph is located. Defaults to 'default' if not provided.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("default"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"readme": schema.StringAttribute{
				MarkdownDescription: "Readme content for the federated graph.",
				Optional:            true,
			},
			"admission_webhook_url": schema.StringAttribute{
				MarkdownDescription: "The URL for the admission webhook that will be triggered during graph operations.",
				Optional:            true,
			},
			"admission_webhook_secret": schema.StringAttribute{
				MarkdownDescription: "The secret token used to authenticate the admission webhook requests.",
				Optional:            true,
				Sensitive:           true,
			},
			"routing_url": schema.StringAttribute{
				MarkdownDescription: "The URL of the service that routes requests to the federated graph.",
				Required:            true,
			},
			"label_matchers": schema.ListAttribute{
				MarkdownDescription: "A list of label matchers used to select the services that will form the federated graph.",
				Optional:            true,
				ElementType:         types.StringType,
				Computed:            true,
				Default:             listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})),
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *FederatedGraphResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.PlatformClient)
	if !ok {
		utils.AddDiagnosticError(resp, ErrUnexpectedDataSourceType, fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}

	r.client = client
}

func (r *FederatedGraphResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FederatedGraphResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	response, apiError := r.createFederatedGraph(ctx, data, resp)
	if apiError != nil {
		if api.IsSubgraphCompositionFailedError(apiError) {
			utils.AddDiagnosticWarning(resp, ErrCompositionError, apiError.Error())
		} else {
			utils.AddDiagnosticError(resp, ErrCreatingGraph, apiError.Error())
			return
		}
		utils.AddDiagnosticWarning(resp, ErrCompositionError, apiError.Error())
	}

	graph := response.Graph
	data.Id = types.StringValue(graph.GetId())
	data.Name = types.StringValue(graph.GetName())
	data.Namespace = types.StringValue(graph.GetNamespace())
	data.RoutingURL = types.StringValue(graph.GetRoutingURL())

	utils.LogAction(ctx, DebugCreate, data.Id.ValueString(), data.Name.ValueString(), data.Namespace.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FederatedGraphResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FederatedGraphResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Id.IsNull() || data.Id.ValueString() == "" {
		utils.AddDiagnosticError(resp, ErrInvalidResourceID, "Cannot read federated graph without an ID.")
		return
	}

	apiResponse, err := r.client.GetFederatedGraph(ctx, data.Name.ValueString(), data.Namespace.ValueString())
	if err != nil {
		if api.IsNotFoundError(err) {
			utils.AddDiagnosticWarning(resp, "Graph not found", fmt.Sprintf("Graph '%s' not found will be recreated", data.Name.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		utils.AddDiagnosticError(resp, ErrReadingGraph, fmt.Sprintf("Could not fetch subgraph '%s': %s", data.Name.ValueString(), err))
		return
	}

	graph := apiResponse.Graph
	data.Id = types.StringValue(graph.GetId())
	data.Name = types.StringValue(graph.GetName())
	data.Namespace = types.StringValue(graph.GetNamespace())
	data.RoutingURL = types.StringValue(graph.GetRoutingURL())

	var labelMatchers []attr.Value
	for _, matcher := range graph.LabelMatchers {
		labelMatchers = append(labelMatchers, types.StringValue(matcher))
	}
	data.LabelMatchers = types.ListValueMust(types.StringType, labelMatchers)

	utils.LogAction(ctx, "read", data.Id.ValueString(), data.Name.ValueString(), data.Namespace.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FederatedGraphResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data FederatedGraphResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Id.IsNull() || data.Id.ValueString() == "" {
		utils.AddDiagnosticError(resp, ErrInvalidResourceID, fmt.Sprintf("Cannot update federated graph because the resource ID is missing. Graph name: %s, graph namespace: %s", data.Name.ValueString(), data.Namespace.ValueString()))
		return
	}

	labelMatchers, err := utils.ConvertAndValidateLabelMatchers(data.LabelMatchers, resp)
	if err != nil {
		return
	}

	graph := platformv1.FederatedGraph{
		Name:                data.Name.ValueString(),
		Namespace:           data.Namespace.ValueString(),
		RoutingURL:          data.RoutingURL.ValueString(),
		AdmissionWebhookUrl: data.AdmissionWebhookUrl.ValueStringPointer(),
		LabelMatchers:       labelMatchers,
		Readme:              data.Readme.ValueStringPointer(),
	}

	var admissionWebhookSecret *string
	if !data.AdmissionWebhookSecret.IsNull() {
		admissionWebhookSecret = data.AdmissionWebhookSecret.ValueStringPointer()
	}

	apiResponse, apiError := r.client.UpdateFederatedGraph(ctx, admissionWebhookSecret, &graph)
	if apiError != nil {
		if api.IsSubgraphCompositionFailedError(apiError) {
			utils.AddDiagnosticWarning(resp,
				ErrCompositionErrors,
				apiError.Error(),
			)
		} else {
			utils.AddDiagnosticError(resp,
				ErrUpdatingGraph,
				apiError.Error(),
			)
			return
		}
	}

	if len(apiResponse.CompositionErrors) > 0 {
		utils.AddDiagnosticWarning(resp, ErrCompositionError, fmt.Sprintf("Composition errors: %v, graph name: %s, graph namespace: %s", apiResponse.CompositionErrors, graph.GetName(), graph.GetNamespace()))
		return
	}

	utils.LogAction(ctx, "updated", data.Id.ValueString(), data.Name.ValueString(), data.Namespace.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *FederatedGraphResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data FederatedGraphResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Id.IsNull() || data.Id.ValueString() == "" {
		utils.AddDiagnosticError(resp, ErrInvalidResourceID, fmt.Sprintf("Cannot delete the federated graph because the resource ID is missing. Graph name: %s, graph namespace: %s", data.Name.ValueString(), data.Namespace.ValueString()))
		return
	}

	err := r.client.DeleteFederatedGraph(ctx, data.Name.ValueString(), data.Namespace.ValueString())

	if err != nil {
		utils.AddDiagnosticError(resp, ErrDeletingGraph, fmt.Sprintf("Could not delete federated graph: %s, graph name: %s, graph namespace: %s", err, data.Name.ValueString(), data.Namespace.ValueString()))
		return
	}

	utils.LogAction(ctx, "deleted", data.Id.ValueString(), data.Name.ValueString(), data.Namespace.ValueString())
}

func (r *FederatedGraphResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *FederatedGraphResource) createFederatedGraph(ctx context.Context, data FederatedGraphResourceModel, resp *resource.CreateResponse) (*platformv1.GetFederatedGraphByNameResponse, *api.ApiError) {
	labelMatchers, err := utils.ConvertAndValidateLabelMatchers(data.LabelMatchers, resp)
	if err != nil {
		return nil, &api.ApiError{Err: err, Reason: "CreateFederatedGraph", Status: common.EnumStatusCode_ERR}
	}

	apiGraph := platformv1.FederatedGraph{
		Name:                data.Name.ValueString(),
		Namespace:           data.Namespace.ValueString(),
		RoutingURL:          data.RoutingURL.ValueString(),
		AdmissionWebhookUrl: data.AdmissionWebhookUrl.ValueStringPointer(),
		Readme:              data.Readme.ValueStringPointer(),
		LabelMatchers:       labelMatchers,
	}

	var admissionWebhookSecret *string
	if !data.AdmissionWebhookSecret.IsNull() {
		admissionWebhookSecret = data.AdmissionWebhookSecret.ValueStringPointer()
	}

	utils.DebugAction(ctx, DebugCreate, data.Name.ValueString(), data.Namespace.ValueString(), map[string]interface{}{
		"admission_webhook_url": apiGraph.AdmissionWebhookUrl,
		"routing_url":           apiGraph.RoutingURL,
		"label_matchers":        labelMatchers,
	})

	_, apiError := r.client.CreateFederatedGraph(ctx, admissionWebhookSecret, &apiGraph)
	if apiError != nil {
		if api.IsSubgraphCompositionFailedError(apiError) {
			utils.AddDiagnosticWarning(resp, ErrCompositionError, apiError.Error())
		} else {
			return nil, apiError
		}
	}

	response, apiError := r.client.GetFederatedGraph(ctx, apiGraph.Name, apiGraph.Namespace)
	if apiError != nil {
		return nil, apiError
	}

	utils.DebugAction(ctx, DebugCreate, data.Name.ValueString(), data.Namespace.ValueString(), map[string]interface{}{
		"id":    response.Graph.GetId(),
		"graph": response.Graph,
	})

	return response, nil
}
