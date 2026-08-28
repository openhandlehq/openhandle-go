package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type document struct {
	Components struct {
		Schemas map[string]schema `json:"schemas"`
	} `json:"components"`
	Paths map[string]map[string]operation `json:"paths"`
}

type stringTypes []string

func (s *stringTypes) UnmarshalJSON(data []byte) error {
	var single string
	if json.Unmarshal(data, &single) == nil {
		*s = []string{single}
		return nil
	}
	var multiple []string
	if err := json.Unmarshal(data, &multiple); err != nil {
		return err
	}
	*s = multiple
	return nil
}

type schema struct {
	Ref                  string            `json:"$ref"`
	Type                 stringTypes       `json:"type"`
	Format               string            `json:"format"`
	Description          string            `json:"description"`
	Properties           map[string]schema `json:"properties"`
	Required             []string          `json:"required"`
	Items                *schema           `json:"items"`
	AllOf                []schema          `json:"allOf"`
	AnyOf                []schema          `json:"anyOf"`
	OneOf                []schema          `json:"oneOf"`
	Enum                 []any             `json:"enum"`
	Const                any               `json:"const"`
	AdditionalProperties json.RawMessage   `json:"additionalProperties"`
}

type parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Schema      schema `json:"schema"`
}

type operation struct {
	Summary     string              `json:"summary"`
	Description string              `json:"description"`
	Parameters  []parameter         `json:"parameters"`
	RequestBody *requestBody        `json:"requestBody"`
	Responses   map[string]response `json:"responses"`
	SDK         sdkOperation        `json:"x-openhandle-sdk"`
}

type requestBody struct {
	Content map[string]mediaType `json:"content"`
}

type response struct {
	Content map[string]mediaType `json:"content"`
}

type mediaType struct {
	Schema schema `json:"schema"`
}

type sdkOperation struct {
	Path      string     `json:"path"`
	Scope     []sdkScope `json:"scope"`
	Operation string     `json:"operation"`
	Paginated bool       `json:"paginated"`
}

type sdkScope struct {
	Name      string `json:"name"`
	Parameter string `json:"parameter"`
	Reference string `json:"reference"`
}

type operationDefinition struct {
	APIPath string
	Method  string
	Value   operation
}

type treeNode struct {
	Path       []string
	Scope      *sdkScope
	Children   map[string]*treeNode
	Operations map[string]operationDefinition
}

var check = flag.Bool("check", false, "fail when generated files are stale")
var componentSchemas map[string]schema

func main() {
	flag.Parse()
	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	input, err := os.ReadFile(filepath.Join(root, "openapi", "openhandle.json"))
	if err != nil {
		fatal(err)
	}
	var contract document
	if err := json.Unmarshal(input, &contract); err != nil {
		fatal(fmt.Errorf("decode OpenAPI contract: %w", err))
	}
	componentSchemas = contract.Components.Schemas
	definitions, err := operations(contract)
	if err != nil {
		fatal(err)
	}

	writeGenerated(root, "models.gen.go", generateModels(contract.Components.Schemas))
	writeGenerated(root, "resources.gen.go", generateResources(definitions))
}

func repositoryRoot() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for candidate := workingDirectory; ; candidate = filepath.Dir(candidate) {
		if _, err := os.Stat(filepath.Join(candidate, "openapi", "openhandle.json")); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", errors.New("openhandle generator: repository root not found")
		}
	}
}

func operations(contract document) ([]operationDefinition, error) {
	var definitions []operationDefinition
	seen := map[string]bool{}
	for apiPath, pathItem := range contract.Paths {
		for method, value := range pathItem {
			if !contains([]string{"get", "post", "put", "patch", "delete"}, method) {
				continue
			}
			if value.SDK.Path == "" || value.SDK.Operation == "" || len(value.SDK.Scope) == 0 && value.SDK.Operation != "fetch" {
				return nil, fmt.Errorf("%s %s has no valid x-openhandle-sdk mapping", strings.ToUpper(method), apiPath)
			}
			if seen[value.SDK.Path] {
				return nil, fmt.Errorf("duplicate SDK path %s", value.SDK.Path)
			}
			seen[value.SDK.Path] = true
			definitions = append(definitions, operationDefinition{APIPath: apiPath, Method: method, Value: value})
		}
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Value.SDK.Path < definitions[j].Value.SDK.Path })
	return definitions, nil
}

func generateModels(schemas map[string]schema) []byte {
	var output strings.Builder
	output.WriteString(generatedHeader())
	output.WriteString("package openhandle\n\n")
	output.WriteString("import \"time\"\n\n")
	names := sortedKeys(schemas)
	for _, name := range names {
		output.WriteString(modelDeclaration(pascalCase(name), schemas[name]))
		output.WriteString("\n")
	}
	return []byte(output.String())
}

func modelDeclaration(name string, value schema) string {
	if value.Ref != "" {
		return fmt.Sprintf("type %s = %s\n", name, referenceName(value.Ref))
	}
	if len(value.AllOf) > 0 {
		return fmt.Sprintf("type %s %s\n", name, structType(value, ""))
	}
	if hasType(value, "object") || len(value.Properties) > 0 {
		return fmt.Sprintf("type %s %s\n", name, structType(value, ""))
	}
	return fmt.Sprintf("type %s %s\n", name, goType(value, true))
}

func structType(value schema, indent string) string {
	var fields []string
	seenEmbeds := map[string]bool{}
	for _, part := range value.AllOf {
		if part.Ref != "" {
			name := referenceName(part.Ref)
			if !seenEmbeds[name] {
				fields = append(fields, name)
				seenEmbeds[name] = true
			}
		}
	}
	properties := map[string]schema{}
	for name, property := range value.Properties {
		properties[name] = property
	}
	for _, part := range value.AllOf {
		for name, property := range part.Properties {
			properties[name] = property
		}
	}
	required := requiredSet(value)
	for _, part := range value.AllOf {
		for _, name := range part.Required {
			required[name] = true
		}
	}
	for _, propertyName := range sortedKeys(properties) {
		property := properties[propertyName]
		fieldName := pascalCase(propertyName)
		fieldType := goType(property, required[propertyName])
		tag := propertyName
		if !required[propertyName] {
			tag += ",omitempty"
		}
		fields = append(fields, fmt.Sprintf("%s %s `json:%s`", fieldName, fieldType, strconv.Quote(tag)))
	}
	if len(fields) == 0 {
		if additionalPropertiesAllowed(value.AdditionalProperties) {
			return "map[string]any"
		}
		return "struct{}"
	}
	return "struct {\n\t" + strings.Join(fields, "\n\t") + "\n}"
}

func goType(value schema, required bool) string {
	if len(value.AnyOf) > 0 {
		for _, variant := range value.AnyOf {
			if hasType(variant, "null") {
				continue
			}
			result := goType(variant, true)
			if !strings.HasPrefix(result, "[]") && !strings.HasPrefix(result, "map[") && result != "any" && !strings.HasPrefix(result, "*") {
				result = "*" + result
			}
			return result
		}
		return "any"
	}
	nullable := hasType(value, "null") || !required
	var result string
	switch {
	case value.Ref != "":
		result = referenceName(value.Ref)
	case len(value.AllOf) == 1 && value.AllOf[0].Ref != "":
		result = referenceName(value.AllOf[0].Ref)
	case len(value.AllOf) > 0:
		result = structType(value, "")
	case hasType(value, "array"):
		if value.Items == nil {
			result = "[]any"
		} else {
			result = "[]" + strings.TrimPrefix(goType(*value.Items, true), "*")
		}
	case hasType(value, "object") || len(value.Properties) > 0:
		result = structType(value, "")
	case hasType(value, "integer"):
		result = "int64"
	case hasType(value, "number"):
		result = "float64"
	case hasType(value, "boolean"):
		result = "bool"
	case hasType(value, "string") && value.Format == "date-time":
		result = "time.Time"
	case hasType(value, "string"):
		result = "string"
	default:
		result = "any"
	}
	if nullable && !strings.HasPrefix(result, "[]") && !strings.HasPrefix(result, "map[") && result != "any" && !strings.HasPrefix(result, "*") {
		result = "*" + result
	}
	return result
}

func generateResources(definitions []operationDefinition) []byte {
	root := &treeNode{Children: map[string]*treeNode{}, Operations: map[string]operationDefinition{}}
	for _, definition := range definitions {
		node := root
		for _, scopeValue := range definition.Value.SDK.Scope {
			child := node.Children[scopeValue.Name]
			if child == nil {
				scopeCopy := scopeValue
				child = &treeNode{
					Path:       append(append([]string{}, node.Path...), scopeValue.Name),
					Scope:      &scopeCopy,
					Children:   map[string]*treeNode{},
					Operations: map[string]operationDefinition{},
				}
				node.Children[scopeValue.Name] = child
			}
			node = child
		}
		node.Operations[definition.Value.SDK.Operation] = definition
	}

	var output strings.Builder
	output.WriteString(generatedHeader())
	output.WriteString("package openhandle\n\n")
	output.WriteString("import (\n\t\"context\"\n\t\"encoding/json\"\n\t\"errors\"\n\t\"time\"\n)\n\n")
	generateClient(&output, root)
	var nodes []*treeNode
	collectNodes(root, &nodes)
	for _, node := range nodes {
		generateNode(&output, node)
	}
	for _, definition := range definitions {
		generateOperationTypes(&output, definition)
	}
	generateFetchTypes(&output)
	return []byte(output.String())
}

func generateClient(output *strings.Builder, root *treeNode) {
	output.WriteString("// Client is a reusable Openhandle API client.\n")
	output.WriteString("type Client struct {\n\tcore *clientCore\n")
	for _, name := range sortedKeys(root.Children) {
		child := root.Children[name]
		output.WriteString(fmt.Sprintf("\t%s *%s\n", pascalCase(name), nodeType(child)))
	}
	output.WriteString("}\n\n")
	output.WriteString("func newGeneratedClient(core *clientCore) *Client {\n\tclient := &Client{core: core}\n")
	for _, name := range sortedKeys(root.Children) {
		child := root.Children[name]
		output.WriteString(fmt.Sprintf("\tclient.%s = new%s(client, nil, nil)\n", pascalCase(name), nodeType(child)))
	}
	output.WriteString("\treturn client\n}\n\n")
	if definition, ok := root.Operations["fetch"]; ok {
		generateMethod(output, root, "fetch", definition)
	}
}

func collectNodes(node *treeNode, result *[]*treeNode) {
	for _, name := range sortedKeys(node.Children) {
		child := node.Children[name]
		*result = append(*result, child)
		collectNodes(child, result)
	}
}

func generateNode(output *strings.Builder, node *treeNode) {
	typeName := nodeType(node)
	output.WriteString(fmt.Sprintf("type %s struct {\n\tclient *Client\n\tbindings map[string]string\n\treferenceErr error\n", typeName))
	for _, name := range sortedKeys(node.Children) {
		child := node.Children[name]
		if child.Scope.Parameter == "" {
			output.WriteString(fmt.Sprintf("\t%s *%s\n", pascalCase(name), nodeType(child)))
		}
	}
	output.WriteString("}\n\n")
	output.WriteString(fmt.Sprintf("func new%s(client *Client, bindings map[string]string, referenceErr error) *%s {\n", typeName, typeName))
	output.WriteString(fmt.Sprintf("\tvalue := &%s{client: client, bindings: bindings, referenceErr: referenceErr}\n", typeName))
	for _, name := range sortedKeys(node.Children) {
		child := node.Children[name]
		if child.Scope.Parameter == "" {
			output.WriteString(fmt.Sprintf("\tvalue.%s = new%s(client, bindings, referenceErr)\n", pascalCase(name), nodeType(child)))
		}
	}
	output.WriteString("\treturn value\n}\n\n")
	for _, name := range sortedKeys(node.Children) {
		child := node.Children[name]
		if child.Scope.Parameter == "" {
			continue
		}
		constraint := "ResourceReference"
		if child.Scope.Reference == "profile" {
			constraint = "ProfileReference"
		}
		output.WriteString(fmt.Sprintf("func (r *%s) %s[T %s](reference T) *%s {\n", typeName, pascalCase(name), constraint, nodeType(child)))
		output.WriteString("\tbindings := cloneBindings(r.bindings)\n\treferenceErr := r.referenceErr\n")
		platform := node.Path[0]
		output.WriteString(fmt.Sprintf("\tresolved, err := resolveReference(reference, %q, %q)\n", platform, child.Scope.Reference))
		output.WriteString("\tif referenceErr == nil && err != nil { referenceErr = err }\n")
		output.WriteString(fmt.Sprintf("\tif err == nil { bindings[%q] = resolved }\n", child.Scope.Parameter))
		output.WriteString(fmt.Sprintf("\treturn new%s(r.client, bindings, referenceErr)\n}\n\n", nodeType(child)))
	}
	for _, operationName := range sortedKeys(node.Operations) {
		generateMethod(output, node, operationName, node.Operations[operationName])
	}
}

func generateMethod(output *strings.Builder, node *treeNode, operationName string, definition operationDefinition) {
	base := operationTypeBase(definition.Value.SDK)
	receiverType := "Client"
	receiver := "c"
	core := "c.core"
	bindings := "nil"
	referenceErr := "nil"
	if len(node.Path) > 0 {
		receiverType = nodeType(node)
		receiver = "r"
		core = "r.client.core"
		bindings = "r.bindings"
		referenceErr = "r.referenceErr"
	}
	methodName := pascalCase(operationName)
	if operationName == "fetch" {
		output.WriteString(fmt.Sprintf("func (%s *%s) Fetch(ctx context.Context, socialURL string, options *FetchOptions) (*FetchResponse, error) {\n", receiver, receiverType))
		output.WriteString("\tif socialURL == \"\" { return nil, errors.New(\"openhandle: fetch URL must not be empty\") }\n")
		output.WriteString("\tcontrols := RequestOptions{}\n\tfreshness := Freshness(\"\")\n\tif options != nil { controls = options.RequestOptions; freshness = options.Freshness }\n")
		output.WriteString("\tbody := struct { URL string `json:\"url\"`; Freshness Freshness `json:\"freshness,omitempty\"` }{URL: socialURL, Freshness: freshness}\n")
		output.WriteString("\tresult := &FetchResponse{}\n")
		output.WriteString(fmt.Sprintf("\tif err := %s.do(ctx, %q, %q, nil, body, controls, result); err != nil { return nil, err }\n", core, strings.ToUpper(definition.Method), definition.APIPath))
		output.WriteString("\treturn result, nil\n}\n\n")
		return
	}
	optionsType := base + "Options"
	resultType := base + "Response"
	dataType := operationDataType(definition)
	if definition.Value.SDK.Paginated {
		resultType = base + "Page"
	}
	output.WriteString(fmt.Sprintf("func (%s *%s) %s(ctx context.Context, options *%s) (*%s, error) {\n", receiver, receiverType, methodName, optionsType, resultType))
	if operationRequiresOptions(definition.Value) {
		output.WriteString("\tif options == nil { return nil, errors.New(\"openhandle: operation options are required\") }\n")
	}
	output.WriteString(fmt.Sprintf("\tpath, err := bindPath(%q, %s, %s)\n\tif err != nil { return nil, err }\n", definition.APIPath, bindings, referenceErr))
	output.WriteString("\tquery, controls, err := encodeOptions(options)\n\tif err != nil { return nil, err }\n")
	output.WriteString(fmt.Sprintf("\tresult := &%s{}\n", resultType))
	output.WriteString(fmt.Sprintf("\tif err := %s.do(ctx, %q, path, query, nil, controls, result); err != nil { return nil, err }\n", core, strings.ToUpper(definition.Method)))
	if definition.Value.SDK.Paginated {
		output.WriteString("\tif result.HasNextPage() {\n\t\tcursor := result.NextCursor()\n")
		output.WriteString(fmt.Sprintf("\t\tnextOptions := &%s{}\n\t\tif options != nil { *nextOptions = *options }\n\t\tnextOptions.Cursor = &cursor\n", optionsType))
		output.WriteString("\t\tresult.next = func(nextContext context.Context) (*Page[" + dataType + "], error) {\n")
		output.WriteString(fmt.Sprintf("\t\t\trequestOptions := *nextOptions\n\t\t\treturn %s.%s(nextContext, &requestOptions)\n\t\t}\n\t}\n", receiver, methodName))
	}
	output.WriteString("\treturn result, nil\n}\n\n")
	if definition.Value.SDK.Paginated {
		iteratorName := "Items"
		if operationName != "list" {
			iteratorName = methodName + "Items"
		}
		output.WriteString(fmt.Sprintf("func (%s *%s) %s(options *%s) *Iterator[%s] {\n", receiver, receiverType, iteratorName, optionsType, dataType))
		output.WriteString(fmt.Sprintf("\titeratorOptions := &%s{}\n\tif options != nil { *iteratorOptions = *options }\n", optionsType))
		output.WriteString(fmt.Sprintf("\treturn newIterator(func(ctx context.Context) (*Page[%s], error) { return %s.%s(ctx, iteratorOptions) })\n}\n\n", dataType, receiver, methodName))
	}
}

func generateOperationTypes(output *strings.Builder, definition operationDefinition) {
	if definition.Value.SDK.Operation == "fetch" {
		return
	}
	base := operationTypeBase(definition.Value.SDK)
	output.WriteString(fmt.Sprintf("// %sOptions configures %s.\n", base, definition.Value.SDK.Path))
	output.WriteString(fmt.Sprintf("type %sOptions struct {\n\tRequestOptions\n", base))
	for _, parameter := range definition.Value.Parameters {
		if parameter.In != "query" {
			continue
		}
		fieldType := optionType(parameter)
		tag := fmt.Sprintf("`query:%s", strconv.Quote(parameter.Name))
		if parameter.Required {
			tag += " required:\"true\""
		}
		tag += "`"
		output.WriteString(fmt.Sprintf("\t%s %s %s\n", pascalCase(parameter.Name), fieldType, tag))
	}
	output.WriteString("}\n\n")
	dataType := operationDataType(definition)
	if definition.Value.SDK.Paginated {
		output.WriteString(fmt.Sprintf("type %sPage = Page[%s]\n\n", base, dataType))
	} else {
		output.WriteString(fmt.Sprintf("type %sResponse struct {\n\tResponseMetadata\n\tData %s `json:\"data\"`\n}\n\n", base, dataType))
	}
}

func generateFetchTypes(output *strings.Builder) {
	variants := []string{"InstagramProfile", "InstagramPost", "InstagramStory", "InstagramHighlight", "TikTokProfile", "TikTokPost", "TwitterProfile", "TwitterPost"}
	output.WriteString("// FetchResource is one of the concrete resources returned by Client.Fetch.\n")
	output.WriteString("type FetchResource interface { isFetchResource() }\n\n")
	for _, variant := range variants {
		output.WriteString(fmt.Sprintf("func (*%s) isFetchResource() {}\n", variant))
	}
	output.WriteString("\n// FetchResponse is the polymorphic response returned by Client.Fetch.\n")
	output.WriteString("type FetchResponse struct {\n\tResponseMetadata\n\tData FetchResource `json:\"-\"`\n}\n\n")
	output.WriteString("func (r *FetchResponse) UnmarshalJSON(data []byte) error {\n")
	output.WriteString("\tvar wire struct { ResponseMetadata; Data json.RawMessage `json:\"data\"` }\n")
	output.WriteString("\tif err := json.Unmarshal(data, &wire); err != nil { return err }\n\tr.ResponseMetadata = wire.ResponseMetadata\n")
	output.WriteString("\tvar target FetchResource\n\tswitch {\n")
	output.WriteString("\tcase wire.Platform == PlatformInstagram && wire.Resource == Resource(\"profile\"):\n\t\ttarget = &InstagramProfile{}\n")
	output.WriteString("\tcase wire.Platform == PlatformInstagram && wire.Resource == Resource(\"post\"):\n\t\ttarget = &InstagramPost{}\n")
	output.WriteString("\tcase wire.Platform == PlatformInstagram && wire.Resource == Resource(\"entity\"):\n\t\tvar probe map[string]json.RawMessage\n\t\tif err := json.Unmarshal(wire.Data, &probe); err != nil { return err }\n\t\tif _, ok := probe[\"items\"]; ok { target = &InstagramHighlight{} } else { target = &InstagramStory{} }\n")
	output.WriteString("\tcase wire.Platform == PlatformTikTok && wire.Resource == Resource(\"profile\"):\n\t\ttarget = &TikTokProfile{}\n")
	output.WriteString("\tcase wire.Platform == PlatformTikTok && wire.Resource == Resource(\"post\"):\n\t\ttarget = &TikTokPost{}\n")
	output.WriteString("\tcase wire.Platform == PlatformTwitter && wire.Resource == Resource(\"profile\"):\n\t\ttarget = &TwitterProfile{}\n")
	output.WriteString("\tcase wire.Platform == PlatformTwitter && wire.Resource == Resource(\"post\"):\n\t\ttarget = &TwitterPost{}\n")
	output.WriteString("\tdefault:\n\t\treturn errors.New(\"openhandle: unsupported fetch response variant\")\n\t}\n")
	output.WriteString("\tif err := json.Unmarshal(wire.Data, target); err != nil { return err }\n\tr.Data = target\n\treturn nil\n}\n\n")
}

func operationDataType(definition operationDefinition) string {
	responseValue, ok := definition.Value.Responses["200"]
	if !ok {
		panic("operation has no 200 response: " + definition.Value.SDK.Path)
	}
	media, ok := responseValue.Content["application/json"]
	if !ok {
		panic("operation has no JSON response: " + definition.Value.SDK.Path)
	}
	data, ok := findProperty(media.Schema, "data")
	if !ok {
		panic("operation response has no data schema: " + definition.Value.SDK.Path)
	}
	if definition.Value.SDK.Paginated {
		if data.Items == nil {
			panic("paginated operation data is not an array: " + definition.Value.SDK.Path)
		}
		return strings.TrimPrefix(goType(*data.Items, true), "*")
	}
	return strings.TrimPrefix(goType(data, true), "*")
}

func findProperty(value schema, name string) (schema, bool) {
	if value.Ref != "" {
		if resolved, ok := componentSchemas[referenceName(value.Ref)]; ok {
			return findProperty(resolved, name)
		}
	}
	if property, ok := value.Properties[name]; ok {
		return property, true
	}
	for index := len(value.AllOf) - 1; index >= 0; index-- {
		if property, ok := findProperty(value.AllOf[index], name); ok {
			return property, true
		}
	}
	return schema{}, false
}

func optionType(value parameter) string {
	typeName := goType(value.Schema, value.Required)
	if value.Name == "freshness" {
		typeName = "Freshness"
	} else if value.Name == "platform" {
		typeName = "Platform"
	} else if value.Name == "resource" {
		typeName = "Resource"
	} else if value.Name == "sort" {
		typeName = "SortOrder"
	}
	if value.Required {
		return strings.TrimPrefix(typeName, "*")
	}
	return typeName
}

func operationRequiresOptions(value operation) bool {
	for _, parameter := range value.Parameters {
		if parameter.In == "query" && parameter.Required {
			return true
		}
	}
	return false
}

func operationTypeBase(value sdkOperation) string {
	var parts []string
	for _, scope := range value.Scope {
		parts = append(parts, pascalCase(scope.Name))
	}
	terminal := ""
	if value.Operation == "search" {
		terminal = "Search"
	}
	return strings.Join(parts, "") + terminal
}

func nodeType(node *treeNode) string {
	var parts []string
	for _, value := range node.Path {
		parts = append(parts, pascalCase(value))
	}
	return strings.Join(parts, "") + "Resource"
}

func requiredSet(value schema) map[string]bool {
	result := map[string]bool{}
	for _, name := range value.Required {
		result[name] = true
	}
	return result
}

func hasType(value schema, expected string) bool {
	for _, candidate := range value.Type {
		if candidate == expected {
			return true
		}
	}
	return false
}

func additionalPropertiesAllowed(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("true")) || len(bytes.TrimSpace(value)) > 2 && !bytes.Equal(bytes.TrimSpace(value), []byte("false"))
}

func referenceName(ref string) string {
	return pascalCase(ref[strings.LastIndex(ref, "/")+1:])
}

func pascalCase(value string) string {
	initialisms := map[string]string{
		"id": "ID", "ids": "IDs", "oembed": "OEmbed", "qrCode": "QRCode", "tiktok": "TikTok", "url": "URL",
	}
	if result := initialisms[value]; result != "" {
		return result
	}
	var result strings.Builder
	uppercaseNext := true
	for _, character := range value {
		if character == '_' || character == '-' || character == ' ' {
			uppercaseNext = true
			continue
		}
		if uppercaseNext {
			result.WriteString(strings.ToUpper(string(character)))
			uppercaseNext = false
		} else {
			result.WriteRune(character)
		}
	}
	name := result.String()
	for _, replacement := range [][2]string{{"Id", "ID"}, {"Url", "URL"}, {"QrCode", "QRCode"}, {"Oembed", "OEmbed"}, {"Tiktok", "TikTok"}} {
		name = strings.ReplaceAll(name, replacement[0], replacement[1])
	}
	return name
}

func sortedKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneBindings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values)+1)
	for key, value := range values {
		result[key] = value
	}
	return result
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func generatedHeader() string {
	return "// Code generated by internal/cmd/generate. DO NOT EDIT.\n\n"
}

func writeGenerated(root, relativePath string, contents []byte) {
	formatted, err := format.Source(contents)
	if err != nil {
		_, _ = os.Stderr.Write(contents)
		fatal(fmt.Errorf("format %s: %w", relativePath, err))
	}
	target := filepath.Join(root, relativePath)
	if *check {
		current, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(current, formatted) {
			fatal(fmt.Errorf("%s is stale; run go generate ./...", relativePath))
		}
		return
	}
	if err := os.WriteFile(target, formatted, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
