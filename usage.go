package config

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/infracost/go-proto/pkg/rat"
	"github.com/infracost/proto/gen/go/infracost/usage"
	yamlv3 "gopkg.in/yaml.v3"
)

type UsageYAML struct {
	Version      string      `yaml:"version"`
	TypeUsage    yamlv3.Node `yaml:"resource_type_default_usage"`
	AddressUsage yamlv3.Node `yaml:"resource_usage"`
}

func LoadUsageYAML(r io.Reader, defaults *usage.Usage) (*usage.Usage, error) {

	var base *usage.Usage
	if defaults != nil {
		base = defaults
	} else {
		base = &usage.Usage{
			ByResourceType: make(map[string]*usage.UsageItemMap),
			ByAddress:      make(map[string]*usage.UsageItemMap),
		}
	}

	var y UsageYAML
	if err := yamlv3.NewDecoder(r).Decode(&y); err != nil {
		return nil, fmt.Errorf("error parsing usage YAML: %w", err)
	}
	base.ByAddress = parseResourceUsagesFromYAMLNode(y.AddressUsage)
	for key, val := range parseResourceUsagesFromYAMLNode(y.TypeUsage) {
		base.ByResourceType[key] = val
	}
	return base, nil
}

func parseResourceUsagesFromYAMLNode(raw yamlv3.Node) map[string]*usage.UsageItemMap {
	// skip if the node is non-object
	if len(raw.Content)%2 != 0 {
		return nil
	}

	usageMap := make(map[string]*usage.UsageItemMap, len(raw.Content)/2)

	for i := 0; i < len(raw.Content); i += 2 {
		rawValue := raw.Content[i+1]

		// skip non-object values
		if len(rawValue.Content)%2 != 0 {
			continue
		}

		key := raw.Content[i].Value

		outputMap := make(map[string]*usage.UsageValue, len(rawValue.Content)/2)

		for i := 0; i < len(rawValue.Content); i += 2 {
			attrKeyNode := rawValue.Content[i]
			attrValNode := rawValue.Content[i+1]

			usageItem, err := parseUsageValueFromYAML(attrValNode)
			if err != nil {
				continue
			}

			outputMap[attrKeyNode.Value] = usageItem
		}

		usageMap[key] = &usage.UsageItemMap{
			Items: outputMap,
		}
	}

	return usageMap
}

func parseUsageValueFromYAML(valNode *yamlv3.Node) (*usage.UsageValue, error) {

	var output usage.UsageValue

	if valNode.ShortTag() == "!!map" {

		if len(valNode.Content)%2 != 0 {
			return nil, errors.New("unexpected YAML format")
		}

		mapData := make(map[string]*usage.UsageValue)

		for i := 0; i < len(valNode.Content); i += 2 {
			mapValNode := valNode.Content[i+1]
			val, err := parseUsageValueFromYAML(mapValNode)
			if err != nil {
				// skip bad nodes
				continue
			}
			mapKeyNode := valNode.Content[i]
			mapData[mapKeyNode.Value] = val
		}

		output.Value = &usage.UsageValue_Children{
			Children: &usage.UsageItemMap{
				Items: mapData,
			},
		}

		return &output, nil
	}

	switch valNode.ShortTag() {
	case "!!int", "!!float":
		r, err := rat.NewFromString(valNode.Value)
		if err != nil {
			return nil, err
		}
		output.Value = &usage.UsageValue_NumberValue{
			NumberValue: r.Proto(),
		}
	case "!!bool":
		b, _ := strconv.ParseBool(valNode.Value)
		output.Value = &usage.UsageValue_BoolValue{
			BoolValue: b,
		}
	case "!!str":
		output.Value = &usage.UsageValue_StringValue{
			StringValue: valNode.Value,
		}
	default:
		return nil, fmt.Errorf("unsupported yaml type")
	}

	return &output, nil
}
