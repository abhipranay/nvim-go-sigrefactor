package analyzer

// Signature represents a function or method signature
type Signature struct {
	// Name of the function/method
	Name string `json:"name"`
	// Receiver for methods (empty for functions)
	Receiver *Receiver `json:"receiver,omitempty"`
	// Parameters list
	Params []Parameter `json:"params"`
	// Return values
	Returns []Parameter `json:"returns"`
	// Position information
	Position Position `json:"position"`
	// IsInterface indicates if this is an interface method
	IsInterface bool `json:"isInterface"`
	// InterfaceName if this is an interface method
	InterfaceName string `json:"interfaceName,omitempty"`
	// Doc comment
	Doc string `json:"doc,omitempty"`
}

// Receiver represents a method receiver
type Receiver struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Pointer bool   `json:"pointer"`
}

// Parameter represents a function parameter or return value
type Parameter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Variadic bool   `json:"variadic,omitempty"`
}

// Position represents a location in source code
type Position struct {
	Filename string `json:"filename"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Offset   int    `json:"offset"`
	EndLine  int    `json:"endLine"`
	EndCol   int    `json:"endColumn"`
}

// Usage represents a call site or reference
type Usage struct {
	Position  Position `json:"position"`
	Kind      string   `json:"kind"` // "call", "reference", "implementation"
	InFunc    string   `json:"inFunc,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
}

// Implementation represents an interface implementation
type Implementation struct {
	TypeName  string    `json:"typeName"`
	Method    Signature `json:"method"`
	IsPointer bool      `json:"isPointer"`
}

// AnalysisResult contains complete analysis of a signature
type AnalysisResult struct {
	Signature       Signature        `json:"signature"`
	Usages          []Usage          `json:"usages"`
	Implementations []Implementation `json:"implementations,omitempty"`
}

// RefactorSpec specifies the desired signature changes
type RefactorSpec struct {
	// NewParams is the new parameter list (in desired order)
	NewParams []Parameter `json:"params"`
	// NewReturns is the new return list
	NewReturns []Parameter `json:"returns"`
	// DefaultValues for new parameters at call sites
	DefaultValues map[string]string `json:"defaultValues,omitempty"`
	// RenameReceiver to change receiver name
	RenameReceiver string `json:"renameReceiver,omitempty"`
}

// TextEdit represents a text edit
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// Range represents a range in source code (LSP-compatible)
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// WorkspaceEdit contains all edits to apply
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"`
}
