package monograph

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/wundergraph/cosmo/terraform-provider-cosmo/internal/api"
	"github.com/wundergraph/cosmo/terraform-provider-cosmo/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &MonographDataSource{}

func NewMonographDataSource() datasource.DataSource {
	return &MonographDataSource{}
}

// MonographDataSource defines the data source implementation.
type MonographDataSource struct {
	client *api.PlatformClient
}

// MonographDataSourceModel describes the data source data model.
type MonographDataSourceModel struct {
	Id                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Namespace              types.String `tfsdk:"namespace"`
	Readme                 types.String `tfsdk:"readme"`
	RoutingURL             types.String `tfsdk:"routing_url"`
	AdmissionWebhookSecret types.String `tfsdk:"admission_webhook_secret"`
	LabelMatchers          types.List   `tfsdk:"label_matchers"`
	WebsocketSubprotocol   types.String `tfsdk:"websocket_subprotocol"`
	SubscriptionProtocol   types.String `tfsdk:"subscription_protocol"`
	AdmissionWebhookURL    types.String `tfsdk:"admission_webhook_url"`
}

// Metadata returns the data source type name.
func (d *MonographDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monograph"
}

// Schema defines the schema for the data source.
func (d *MonographDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cosmo Monograph Data Source",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the monograph resource, automatically generated by the system.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the monograph.",
			},
			"namespace": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The namespace in which the monograph is located.",
			},
			"readme": schema.StringAttribute{
				MarkdownDescription: "Readme content for the federated graph.",
				Computed:            true,
			},
			"admission_webhook_url": schema.StringAttribute{
				MarkdownDescription: "The URL for the admission webhook that will be triggered during graph operations.",
				Computed:            true,
			},
			"admission_webhook_secret": schema.StringAttribute{
				MarkdownDescription: "The secret token used to authenticate the admission webhook requests.",
				Computed:            true,
				Sensitive:           true,
			},
			"label_matchers": schema.ListAttribute{
				MarkdownDescription: "A list of label matchers used to select the services that will form the federated graph.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"websocket_subprotocol": schema.StringAttribute{
				MarkdownDescription: "The websocket subprotocol for the monograph.",
				Computed:            true,
			},
			"subscription_protocol": schema.StringAttribute{
				MarkdownDescription: "The subscription protocol for the monograph.",
				Computed:            true,
			},
			"routing_url": schema.StringAttribute{
				MarkdownDescription: "The URL for the federated graph.",
				Computed:            true,
			},
		},
	}
}

// Configure prepares the data source for reading.
func (d *MonographDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.PlatformClient)
	if !ok {
		utils.AddDiagnosticError(resp, ErrUnexpectedDataSourceType, fmt.Sprintf("Expected *client.PlatformClient, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}

	d.client = client
}

// Read refreshes the data source data.
func (d *MonographDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data MonographDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Name.IsNull() || data.Name.ValueString() == "" {
		utils.AddDiagnosticError(resp, ErrInvalidMonographName, fmt.Sprintf("The 'name' attribute is required for monograph in namespace: %s", data.Namespace.ValueString()))
		return
	}

	namespace := data.Namespace.ValueString()
	if namespace == "" {
		namespace = "default"
	}

	monograph, err := d.client.GetMonograph(ctx, data.Name.ValueString(), namespace)
	if err != nil {
		utils.AddDiagnosticError(resp, ErrReadingMonograph, fmt.Sprintf("Could not read monograph: %s, name: %s, namespace: %s", err, data.Name.ValueString(), namespace))
		return
	}

	data.Id = types.StringValue(monograph.GetId())
	data.Name = types.StringValue(monograph.GetName())
	data.Namespace = types.StringValue(monograph.GetNamespace())
	data.RoutingURL = types.StringValue(monograph.GetRoutingURL())
	data.Readme = types.StringValue(monograph.GetReadme())
	data.AdmissionWebhookURL = types.StringValue(monograph.GetAdmissionWebhookUrl())

	tflog.Trace(ctx, "Read monograph data source", map[string]interface{}{
		"id": data.Id.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
