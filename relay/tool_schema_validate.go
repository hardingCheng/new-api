package relay

import (
	"fmt"
	"sort"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// 工具参数结构校验：官方对非法的 function.parameters 直接回 400 并给出校验详情，
// 而多数上游原样收下（实测三条渠道 chat 侧都放行），客户按官方写的用例因此判不合规。
// 这里做 JSON Schema 的结构性校验——只查形状，不做取值域推断，避免误伤合法但少见的写法。
// 默认关闭，按渠道开启（ChannelSettings.ValidateToolSchema）。

var jsonSchemaTypes = map[string]bool{
	"object": true, "array": true, "string": true,
	"number": true, "integer": true, "boolean": true, "null": true,
}

func validateToolSchemasIfNeeded(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) error {
	if info == nil || info.ChannelMeta == nil || request == nil || !info.ChannelSetting.ValidateToolSchema {
		return nil
	}
	for _, tool := range request.Tools {
		if tool.Function.Parameters == nil {
			continue
		}
		name := tool.Function.Name
		if name == "" {
			name = "(unnamed)"
		}
		if err := validateSchemaNode(tool.Function.Parameters, "parameters"); err != nil {
			return fmt.Errorf("Invalid schema for function '%s': %s", name, err.Error())
		}
	}
	return nil
}

// validateSchemaNode 递归检查一个 schema 节点。path 用于把错误定位到具体字段，
// 与官方"给出校验详情"的口径一致。
func validateSchemaNode(node any, path string) error {
	schema, ok := node.(map[string]any)
	if !ok {
		return fmt.Errorf("'%s' must be an object", path)
	}
	if rawType, exists := schema["type"]; exists {
		if err := validateSchemaType(rawType, path); err != nil {
			return err
		}
	}
	if rawProps, exists := schema["properties"]; exists {
		props, ok := rawProps.(map[string]any)
		if !ok {
			return fmt.Errorf("'%s.properties' must be an object", path)
		}
		keys := make([]string, 0, len(props))
		for key := range props {
			keys = append(keys, key)
		}
		// 固定顺序，保证同一份非法 schema 每次报同一条错
		sort.Strings(keys)
		for _, key := range keys {
			if err := validateSchemaNode(props[key], path+".properties."+key); err != nil {
				return err
			}
		}
	}
	if rawRequired, exists := schema["required"]; exists {
		items, ok := rawRequired.([]any)
		if !ok {
			return fmt.Errorf("'%s.required' must be an array of strings", path)
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("'%s.required' must be an array of strings", path)
			}
		}
	}
	if rawItems, exists := schema["items"]; exists {
		if err := validateSchemaNode(rawItems, path+".items"); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaType(rawType any, path string) error {
	switch value := rawType.(type) {
	case string:
		if !jsonSchemaTypes[value] {
			return fmt.Errorf("'%s.type' must be one of %s, got '%s'", path, allowedTypeList(), value)
		}
	case []any:
		// JSON Schema 允许 type 是数组（联合类型）
		for _, item := range value {
			text, ok := item.(string)
			if !ok || !jsonSchemaTypes[text] {
				return fmt.Errorf("'%s.type' must be one of %s", path, allowedTypeList())
			}
		}
	default:
		return fmt.Errorf("'%s.type' must be a string or an array of strings", path)
	}
	return nil
}

func allowedTypeList() string {
	names := make([]string, 0, len(jsonSchemaTypes))
	for name := range jsonSchemaTypes {
		names = append(names, "'"+name+"'")
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
