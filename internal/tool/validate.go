package tool

import (
	"bytes"
	"encoding/json"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"WorkBaby/internal/pkg"
)

// ValidateArgs 校验工具参数是否符合 JSON Schema。
//
// 用 jsonschema/v6 做完整 draft 2020-12 校验：$ref / allOf / oneOf / pattern /
// minimum-maximum / additionalProperties 全部覆盖——自研子集漏掉这些约束会让
// LLM 产出的错参直接穿透到工具执行层。schema 为空或 "{}" 时直接通过。
func ValidateArgs(schema, args json.RawMessage) error {
	if len(schema) == 0 || string(schema) == "null" || string(schema) == "{}" {
		return nil
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(args))
	if err != nil {
		return pkg.Wrap(4004, "invalid tool args json", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return pkg.Wrap(4004, "invalid tool schema json", err)
	}
	c := jsonschema.NewCompiler()
	// 工具 schema 不声明 $schema，指定方言避免编译器按 URL 去拉取元 schema。
	c.DefaultDraft(jsonschema.Draft2020)
	// 资源 URL 用自定义 scheme：给相对名会让编译器解析成本地文件绝对路径并带进错误信息。
	if err := c.AddResource("workbaby://tool/schema.json", doc); err != nil {
		return pkg.Wrap(4004, "invalid tool schema json", err)
	}
	sch, err := c.Compile("workbaby://tool/schema.json")
	if err != nil {
		return pkg.Wrap(4004, "invalid tool schema json", err)
	}
	if err := sch.Validate(inst); err != nil {
		return pkg.New(4004, err.Error(), "")
	}
	return nil
}

// CompileSchema 注册期编译校验：坏 Schema 挡在启动期，调用期只做参数校验。
// schema 为空 / "{}" 视为无约束，直接通过。
func CompileSchema(schema json.RawMessage) error {
	if len(schema) == 0 || string(schema) == "null" || string(schema) == "{}" {
		return nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return pkg.Wrap(4004, "invalid tool schema json", err)
	}
	// 剥离 $schema：MCP 工具常带外部方言声明，编译器会按 URL 联网拉取元 schema，启动期不可依赖网络
	if m, ok := doc.(map[string]any); ok {
		delete(m, "$schema")
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource("workbaby://tool/schema.json", doc); err != nil {
		return pkg.Wrap(4004, "invalid tool schema json", err)
	}
	if _, err := c.Compile("workbaby://tool/schema.json"); err != nil {
		return pkg.New(4004, "invalid tool schema: "+err.Error(), "")
	}
	return nil
}
