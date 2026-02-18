package tools

import (
	"k8s.io/client-go/kubernetes"
	"kubeminds/internal/agent"
)

// ListTools returns a list of all available tools
func ListTools(client kubernetes.Interface) []agent.Tool {
	return []agent.Tool{
		// Pod tools
		NewGetPodLogsTool(client),
		NewGetPodEventsTool(client),
		NewGetPodSpecTool(client),
		// Node tools
		NewGetNodeStatusTool(client),
		NewGetNodeEventsTool(client),
		// Service tools
		NewGetServiceSpecTool(client),
		NewGetEndpointsTool(client),
		// Volume tools
		NewGetPVCStatusTool(client),
		NewGetPVStatusTool(client),
		// Write operation tools
		NewDeletePodTool(client),
		NewPatchDeploymentTool(client),
		NewScaleStatefulSetTool(client),
	}
}
