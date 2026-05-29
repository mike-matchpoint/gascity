package api

import "github.com/danielgtaylor/huma/v2"

func documentWorkSelectorAnyClauses(oapi *huma.OpenAPI) {
	if oapi == nil || oapi.Components == nil || oapi.Components.Schemas == nil {
		return
	}
	workSelector := oapi.Components.Schemas.Map()["WorkSelector"]
	if workSelector == nil || workSelector.Properties == nil {
		return
	}
	anySchema := workSelector.Properties["Any"]
	if anySchema == nil {
		return
	}

	clauseProperties := make(map[string]*huma.Schema, len(workSelector.Properties)-1)
	for name, schema := range workSelector.Properties {
		if name == "Any" {
			continue
		}
		clauseProperties[name] = schema
	}
	clauseRequired := make([]string, 0, len(workSelector.Required))
	for _, name := range workSelector.Required {
		if name != "Any" {
			clauseRequired = append(clauseRequired, name)
		}
	}

	anySchema.Items = &huma.Schema{
		Type:                 "object",
		Properties:           clauseProperties,
		Required:             clauseRequired,
		AdditionalProperties: false,
	}
}
