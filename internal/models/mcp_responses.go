package models

// InitializeResponse defines the structure for the JSON response of the "initialize" method.
// As defined in SPECIFICATION.MD section 6.2.
type InitializeResponse struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

// ServerInfo provides information about the server.
// As defined in SPECIFICATION.MD section 6.2.
type ServerInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// Capabilities defines the server's capabilities.
// As defined in SPECIFICATION.MD section 6.2.
type Capabilities struct {
	Tools ToolsCapabilities `json:"tools"`
}

// ToolsCapabilities is currently an empty struct, can be expanded later.
// As defined in SPECIFICATION.MD section 6.2, it's an empty object: "tools": {}
type ToolsCapabilities struct {
	// In the future, this could list specific tools or their versions.
}

// ToolsListResponse defines the structure for the JSON response of the "tools/list" method.
// As defined in SPECIFICATION.MD section 6.4.
type ToolsListResponse struct {
	Tools []ToolDefinition `json:"tools"`
}

// ToolDefinition describes a single tool available through the server.
// As defined in SPECIFICATION.MD section 6.4.
type ToolDefinition struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ArgumentsSchema Schema          `json:"arguments_schema"`
	ResponseSchema  Schema          `json:"response_schema"`
	Annotations     ToolAnnotations `json:"annotations"`
}

// Schema represents a JSON schema, using map[string]interface{} for flexibility.
// This is used for ArgumentsSchema and ResponseSchema in ToolDefinition.
type Schema map[string]interface{}

// ToolAnnotations provides hints about the tool's behavior.
// As defined in SPECIFICATION.MD section 6.4.
type ToolAnnotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
}
