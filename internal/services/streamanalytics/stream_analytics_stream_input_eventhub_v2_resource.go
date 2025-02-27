package streamanalytics

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-sdk/resource-manager/streamanalytics/2020-03-01/inputs"
	"github.com/hashicorp/go-azure-sdk/resource-manager/streamanalytics/2020-03-01/streamingjobs"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-azurerm/internal/sdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

type StreamInputEventHubV2Resource struct{}

var _ sdk.ResourceWithCustomImporter = StreamInputEventHubV2Resource{}

type StreamInputEventHubV2ResourceModel struct {
	Name                      string `tfschema:"name"`
	StreamAnalyticsJobId      string `tfschema:"stream_analytics_job_id"`
	ServiceBusNamespace       string `tfschema:"servicebus_namespace"`
	EventHubName              string `tfschema:"eventhub_name"`
	EventHubConsumerGroupName string `tfschema:"eventhub_consumer_group_name"`

	SharedAccessPolicyKey  string          `tfschema:"shared_access_policy_key"`
	SharedAccessPolicyName string          `tfschema:"shared_access_policy_name"`
	PartitionKey           string          `tfschema:"partition_key"`
	AuthenticationMode     string          `tfschema:"authentication_mode"`
	Serialization          []Serialization `tfschema:"serialization"`
}

type Serialization struct {
	Type           string `tfschema:"type"`
	FieldDelimiter string `tfschema:"field_delimiter"`
	Encoding       string `tfschema:"encoding"`
}

func (r StreamInputEventHubV2Resource) Arguments() map[string]*pluginsdk.Schema {
	return map[string]*pluginsdk.Schema{
		"name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"stream_analytics_job_id": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: streamingjobs.ValidateStreamingJobID,
		},

		"servicebus_namespace": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"eventhub_name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"eventhub_consumer_group_name": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"shared_access_policy_key": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			Sensitive:    true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"shared_access_policy_name": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"partition_key": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			ValidateFunc: validation.StringIsNotEmpty,
		},

		"authentication_mode": {
			Type:     pluginsdk.TypeString,
			Optional: true,
			Default:  string(inputs.AuthenticationModeConnectionString),
			ValidateFunc: validation.StringInSlice([]string{
				string(inputs.AuthenticationModeMsi),
				string(inputs.AuthenticationModeConnectionString),
			}, false),
		},

		"serialization": schemaStreamAnalyticsStreamInputSerialization(),
	}
}

func (r StreamInputEventHubV2Resource) Attributes() map[string]*schema.Schema {
	return map[string]*pluginsdk.Schema{}
}

func (r StreamInputEventHubV2Resource) ModelObject() interface{} {
	return &StreamInputEventHubV2ResourceModel{}
}

func (r StreamInputEventHubV2Resource) ResourceType() string {
	return "azurerm_stream_analytics_stream_input_eventhub_v2"
}

func (r StreamInputEventHubV2Resource) Create() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			var model StreamInputEventHubV2ResourceModel
			if err := metadata.Decode(&model); err != nil {
				return err
			}

			client := metadata.Client.StreamAnalytics.InputsClient
			subscriptionId := metadata.Client.Account.SubscriptionId

			streamingJobStruct, err := streamingjobs.ParseStreamingJobID(model.StreamAnalyticsJobId)
			if err != nil {
				return err
			}
			id := inputs.NewInputID(subscriptionId, streamingJobStruct.ResourceGroupName, streamingJobStruct.JobName, model.Name)

			existing, err := client.Get(ctx, id)
			if err != nil && !response.WasNotFound(existing.HttpResponse) {
				return fmt.Errorf("checking for presence of existing %s: %+v", id, err)
			}

			if !response.WasNotFound(existing.HttpResponse) {
				return metadata.ResourceRequiresImport(r.ResourceType(), id)
			}

			props := &inputs.EventHubStreamInputDataSourceProperties{
				ServiceBusNamespace: utils.String(model.ServiceBusNamespace),
				EventHubName:        utils.String(model.EventHubName),
				ConsumerGroupName:   utils.String(model.EventHubConsumerGroupName),
				AuthenticationMode:  utils.ToPtr(inputs.AuthenticationMode(model.AuthenticationMode)),
			}

			if v := model.SharedAccessPolicyKey; v != "" {
				props.SharedAccessPolicyKey = utils.String(v)
			}

			if v := model.SharedAccessPolicyName; v != "" {
				props.SharedAccessPolicyName = utils.String(v)
			}

			serialization, err := expandStreamAnalyticsStreamInputSerializationTyped(model.Serialization)
			if err != nil {
				return fmt.Errorf("expanding `serialization`: %+v", err)
			}

			payload := inputs.Input{
				Name: utils.String(model.Name),
				Properties: &inputs.StreamInputProperties{
					Datasource: &inputs.EventHubV2StreamInputDataSource{
						Properties: props,
					},
					Serialization: serialization,
					PartitionKey:  utils.String(model.PartitionKey),
				},
			}

			var opts inputs.CreateOrReplaceOperationOptions
			if _, err = client.CreateOrReplace(ctx, id, payload, opts); err != nil {
				return fmt.Errorf("creating %s: %+v", id, err)
			}

			metadata.SetID(id)

			return nil
		},
	}
}

func (r StreamInputEventHubV2Resource) Update() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.StreamAnalytics.InputsClient
			id, err := inputs.ParseInputID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			var state StreamInputEventHubV2ResourceModel
			if err := metadata.Decode(&state); err != nil {
				return fmt.Errorf("decoding: %+v", err)
			}

			d := metadata.ResourceData

			if d.HasChangesExcept("name", "stream_analytics_job_id") {
				props := &inputs.EventHubStreamInputDataSourceProperties{
					ServiceBusNamespace: utils.String(state.ServiceBusNamespace),
					EventHubName:        utils.String(state.EventHubName),
					ConsumerGroupName:   utils.String(state.EventHubConsumerGroupName),
					AuthenticationMode:  utils.ToPtr(inputs.AuthenticationMode(state.AuthenticationMode)),
				}

				serialization, err := expandStreamAnalyticsStreamInputSerializationTyped(state.Serialization)
				if err != nil {
					return fmt.Errorf("expanding `serialization`: %+v", err)
				}

				payload := inputs.Input{
					Name: utils.String(state.Name),
					Properties: &inputs.StreamInputProperties{
						Datasource: &inputs.EventHubV2StreamInputDataSource{
							Properties: props,
						},
						Serialization: serialization,
						PartitionKey:  utils.String(state.PartitionKey),
					},
				}

				var opts inputs.UpdateOperationOptions
				if _, err := client.Update(ctx, *id, payload, opts); err != nil {
					return fmt.Errorf("updating %s: %+v", *id, err)
				}
			}

			return nil
		},
	}
}

func (r StreamInputEventHubV2Resource) Read() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 5 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.StreamAnalytics.InputsClient
			id, err := inputs.ParseInputID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			resp, err := client.Get(ctx, *id)
			if err != nil {
				if response.WasNotFound(resp.HttpResponse) {
					return metadata.MarkAsGone(id)
				}
				return fmt.Errorf("reading %s: %+v", *id, err)
			}

			streamingJobId := streamingjobs.NewStreamingJobID(id.SubscriptionId, id.ResourceGroupName, id.JobName)

			state := StreamInputEventHubV2ResourceModel{
				Name:                 id.InputName,
				StreamAnalyticsJobId: streamingJobId.ID(),
			}

			if model := resp.Model; model != nil {
				if props := model.Properties; props != nil {
					input, ok := props.(inputs.InputProperties)
					if !ok {
						return fmt.Errorf("converting %s to an Input", *id)
					}

					streamInput, ok := input.(inputs.StreamInputProperties)
					if !ok {
						return fmt.Errorf("converting %s to a Stream Input", *id)
					}

					eventHubV2Input, ok := streamInput.Datasource.(inputs.EventHubV2StreamInputDataSource)
					if !ok {
						return fmt.Errorf("converting %s to an EventHub V2 Stream Input", *id)
					}

					if eventHubV2InputProps := eventHubV2Input.Properties; eventHubV2InputProps != nil {
						servicebusNamespace := ""
						if v := eventHubV2InputProps.ServiceBusNamespace; v != nil {
							servicebusNamespace = *v
						}

						eventHubName := ""
						if v := eventHubV2InputProps.EventHubName; v != nil {
							eventHubName = *v
						}

						eventHubConsumerGroup := ""
						if v := eventHubV2InputProps.ConsumerGroupName; v != nil {
							eventHubConsumerGroup = *v
						}

						authenticationMode := ""
						if v := eventHubV2InputProps.AuthenticationMode; v != nil {
							authenticationMode = string(*v)
						}

						sharedAccessPolicyName := ""
						if v := eventHubV2InputProps.SharedAccessPolicyName; v != nil {
							sharedAccessPolicyName = *v
						}

						serialization := flattenStreamAnalyticsStreamInputSerializationTyped(streamInput.Serialization)

						partitionKey := ""
						if v := streamInput.PartitionKey; v != nil {
							partitionKey = *v
						}

						state.ServiceBusNamespace = servicebusNamespace
						state.EventHubName = eventHubName
						state.EventHubConsumerGroupName = eventHubConsumerGroup
						state.AuthenticationMode = authenticationMode
						state.SharedAccessPolicyName = sharedAccessPolicyName
						state.SharedAccessPolicyKey = metadata.ResourceData.Get("shared_access_policy_key").(string)
						state.Serialization = []Serialization{serialization}
						state.PartitionKey = partitionKey

						return metadata.Encode(&state)
					}
				}
			}
			return nil
		},
	}
}

func (r StreamInputEventHubV2Resource) Delete() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.StreamAnalytics.InputsClient
			id, err := inputs.ParseInputID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			metadata.Logger.Infof("deleting %s", *id)

			if resp, err := client.Delete(ctx, *id); err != nil {
				if !response.WasNotFound(resp.HttpResponse) {
					return fmt.Errorf("deleting %s: %+v", *id, err)
				}
			}
			return nil
		},
	}
}

func (r StreamInputEventHubV2Resource) IDValidationFunc() pluginsdk.SchemaValidateFunc {
	return inputs.ValidateInputID
}

func (r StreamInputEventHubV2Resource) CustomImporter() sdk.ResourceRunFunc {
	return func(ctx context.Context, metadata sdk.ResourceMetaData) error {
		id, err := inputs.ParseInputID(metadata.ResourceData.Id())
		if err != nil {
			return err
		}

		client := metadata.Client.StreamAnalytics.InputsClient
		resp, err := client.Get(ctx, *id)
		if err != nil || resp.Model == nil || resp.Model.Properties == nil {
			return fmt.Errorf("retrieving %s: %+v", *id, err)
		}

		props := resp.Model.Properties

		input, ok := props.(inputs.InputProperties)
		if !ok {
			return fmt.Errorf("specified resource is not an Input: %+v", err)
		}

		streamInput, ok := input.(inputs.StreamInputProperties)
		if !ok {
			return fmt.Errorf("specified resource is not a Stream Input: %+v", err)
		}

		if _, ok := streamInput.Datasource.(inputs.EventHubV2StreamInputDataSource); !ok {
			return fmt.Errorf("specified input is not of type EventHubV2: %+v", err)
		}

		return nil
	}
}
